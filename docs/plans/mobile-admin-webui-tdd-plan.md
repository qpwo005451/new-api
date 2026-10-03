# new-api 手机端运维 WebUI（`/m`）TDD 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `prod/251` 上新增一个独立的手机端只读运维台 `/m`：查看 token 用量（我的/全站）、自建的模型可用性（model-monitor）与路由统计（routing-stats），并支持单个渠道的启用/手动禁用。

**Architecture:** 后端新增一路 embed（`web/mobile-dist`）与 `/m` 静态服务 + SPA 回落，并在根路径按 UA 302 到 `/m`；前端是 `web/` 项目内的第二个构建入口（`web/src/mobile/` → `web/mobile-dist/`），复用 `web/src` 的 shadcn 组件、格式化工具、i18n 与 Tailwind 主题 token，但使用独立 rsbuild 配置以保持手机端首屏体积最小。手机端只使用 PAT（`Authorization: Bearer`）认证，不做登录/刷新/会话逻辑，不新增任何后端业务接口。

**Tech Stack:** Go 1.25.1 + Gin（`gin-contrib/static`、`gin-contrib/gzip`）；React 19 + TypeScript + Rsbuild 2 + Tailwind CSS 4 + Base UI（shadcn `base-nova`）+ TanStack Query + i18next；Vitest + React Testing Library（jsdom）；Bun。

## Global Constraints

- 实施工作树：`/home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui`（Orca 创建，父工作树 `per-model-channel-weight`），分支 `codex/mobile-admin-webui`，基线 `qpwo005451/per-model-channel-weight` @ `7d4ac5fc0`（即 `prod/251` 的当前 head）。所有命令都在实施工作树内执行，不得提交到 `prod/251`。
- 不新增后端业务接口、不改数据库、不改 relay/计费路径。后端与构建装配改动只允许出现在 `main.go`、`router/web-router.go`、`router/web_router_test.go`、`router/testdata/**`、`makefile`、`Dockerfile`、`Dockerfile.dev`、`.dockerignore`、`.github/workflows/*.yml`、`electron/build.sh`、`scripts/build_release_candidate.sh`、`scripts/test_release_helpers.sh`、`.gitignore`。
- **embed sweep 规则**：仓库里任何执行 `go build`/`go vet` 的路径，都必须在它之前让 `web/dist` 与 `web/mobile-dist` **同时**存在（真实构建或桩化）。`main.go` 的两个 `//go:embed` 都是无条件的，只准备 `web/dist` 的路径会直接编译失败。
- 实施工作树是新建 checkout，没有任何 `node_modules`：首次运行前端命令前先执行 `cd web && bun install --frozen-lockfile`。
- 不在生产主机 `10.0.0.251` 上执行 `bun install`、前端构建、`go build` 或测试；全部在本地工作站执行。发布候选使用 `scripts/build_release_candidate_local.ps1`。
- 所有新增前端文件必须带 AGPL 版权头（用 `bun run copyright` 生成，它会重写命中文件；不要让它改写无关文件），文件名 kebab-case；测试必须放在被测模块的 `__tests__/` 目录，命名为 `<职责>.test.ts(x)`。
- 基线 `bun run copyright:check` **本来就是红的**（`prod/251` 有 25 个既有文件缺版权头）。验收标准是「新增的 `src/mobile/**` + `rsbuild.mobile.config.ts` 不在失败名单里」，不是整条命令退出码为 0；不要为了让它变绿去改既有文件。
- 用户可见文案必须走 i18n：`useTranslation()` + `t('English source key')`，key 写入 `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`。
- **手机端代码禁止 `import '@/i18n/config'`**：该模块静态 import 七个语言包（合计 ~4 MB 源码 / ~1.1 MB gzip），静态引入它会直接击穿首屏预算。手机端固定用 `web/src/mobile/lib/i18n.ts` 的 `initializeMobileI18n()`，语言包按需 `import()` 成独立异步 chunk。
- 数字、紧凑数字、金额必须复用 `@/lib/format`（`formatNumber` / `formatCompactNumber` / `formatTokens` / `formatUseTime` / `formatLogQuota`）与 `@/lib/currency`；任何传给 `Intl.*` 的语言码必须先经 `@/i18n/languages` 的 `toIntlLocale()`。禁止自己写 `zhCN` 映射。
- 组件优先复用 `@/components/ui/*`（button/input/label/card/switch 等只依赖 `@base-ui/react` + `cva` + `cn`，可用）与 `@/components/confirm-dialog.tsx`。**但不得复用** `@/components/{loading-state,error-state,empty-state}.tsx`、`@/components/page-transition.tsx`、`@/lib/{api,http-client,status-query}.ts`：它们（直接或传递性）引入 `@tanstack/react-router`、`motion/react`、`axios`、`lucide-react`，既违反禁用清单，也会把首屏 JS gzip 从 96 KB 抬到 ~138 KB。手机端用自己的等价物：`@/mobile/components/{mobile-loading,mobile-empty,mobile-error}.tsx`（props 与桌面同名 `title`/`description`/`className`）与 `@/mobile/lib/status.ts`。本项目没有底部 Tab 栏组件，这是唯一需要新增的通用交互控件；手机端展示基元同属新增能力，需在变更说明中记录该能力缺口。
- 手机端**不引入**：TanStack Router、recharts/vchart、axios、lucide-react、CodeMirror、shiki、katex、auto-skeleton。
- **首屏体积口径与预算（控制器按实测调整过）**：首屏 = `index.html` 直接引用的 chunk + 当前激活页签实际加载的 chunk（含动态 import 的 chunk），全部按 gzip 计。
  - 硬不变量：首屏 chunk 里 `react-router` / `motion/react` / `lucide-react` / `axios` 的出现次数必须为 0。
  - 预算：**< 150 KB（153600 B）**。原定 120 KB 在「强制复用 `@/components/ui/*`（base-ui）+ `@tanstack/react-query` + `i18next`」的前提下不可达：Task 8 实测 `lib-react` 59,664 + vendor 54,808 + index 9,659 = **124,131 B**，其中 vendor 已经把 react-query/base-ui/i18next/zustand/dayjs 计入；120 KB 只剩约 114 KB 给这两块基础依赖，装不下。
  - 每个任务都要报告实测数字与 chunk 明细；若将来需要回到 120 KB，已知的杠杆是把手机端组件里的 base-ui 换成纯 Tailwind 标记（预计可回收约 18 KB），不要在未获裁决时自行做这个替换。
- 图表一律自绘 SVG（迷你折线 + 堆叠占比条），不引入图表库。
- 后端测试只用 `github.com/stretchr/testify/require`（致命断言/setup）与 `assert`（非致命断言）；本次后端改动只允许新增一个测试文件 `router/web_router_test.go`。
- Go 代码遵循本仓库现代写法：`any`、`for i := range n`、`strings.Cut/TrimPrefix/CutPrefix`、`slices.Contains`、`min/max`、`strings.Builder`、`reflect.TypeFor[T]()`；改完执行 `gofmt`。
- 本轮不新增 `docs/` 下除本文件以外的任何文档。

---

## 1. 范围与基线

### 1.1 目标（v1）

| 页签 | 能力 | 数据来源 |
| --- | --- | --- |
| 用量 | 我的/全站切换；今天/7天/30天；花费、token、请求数、当前 rpm/tpm；按模型、按用户排行；最近请求列表 | `/api/log/stat`、`/api/log/self/stat`、`/api/data/`、`/api/data/users`、`/api/data/self`、`/api/log`、`/api/log/self` |
| 模型可用性 | 站点健康分布、异常模型置顶、模型状态与最近失败原因、数据新鲜度 | `/api/model-monitor/summary`、`/api/model-monitor/sites/:id` |
| 路由统计 | 模型 × 渠道请求/错误/平均耗时；实测流量占比 与 生效 priority/weight 并排；切换次数与原因；亲和绑定数 | `/api/log/routing_stats`、`/api/log/channel_affinity_bindings` |
| 渠道 | 列表（名称/类型/分组/状态/余额）、名称搜索、状态筛选、单个渠道启用·手动禁用 | `/api/channel/`、`/api/channel/:id/status` |

### 1.2 明确不做（v1）

- 不做登录页、密码、Turnstile、2FA、token 刷新；只支持 PAT。
- 不做渠道新增/编辑/删除/批量启停/标签启停、不做 `/api/channel/:id/key`（需安全验证）。
- 不做 model-monitor 配置与告警编辑、不做探活触发。
- 不做用量按分组维度排行（`quota_data` 聚合不含 group 维度；`/api/data/` 只按 model+日 与 user+日 聚合）。
- 不做 Service Worker / 离线壳（只做 manifest + iOS standalone meta）。v1 不注入 Umami/GA 到手机壳（`main.go` 的注入函数只处理桌面 `indexPage`；后续如需，单点扩展）。
- 不做 iPadOS 13+ 自动跳转（该平台 UA 与桌面 Safari 相同，会落到桌面版；如需可在手机壳内提供入口）。

### 1.3 数据契约（已按 HEAD `7d4ac5fc0` 源码核实）

- `GET /api/log/stat`（`AdminAuth`）/ `GET /api/log/self/stat`（`UserAuth`）→ `{success, data:{quota, rpm, tpm}}`。
  - `quota` = 所选窗口内 `SUM(quota)`，仅 `type = LogTypeConsume`（`model/log.go` `SumUsedQuota`）。
  - `rpm` / `tpm` = **仅最近 60 秒**（`model/log.go:990`）。UI 必须分别标注窗口与"最近 1 分钟"。
- `GET /api/data/`（`AdminAuth`，参数 `start_timestamp`、`end_timestamp`、可选 `username`）→ `[]QuotaData`，每行 `{model_name, created_at, count, quota, token_used}`，按 `model_name, created_at` 聚合。
- `GET /api/data/users`（`AdminAuth`）→ 同上按 `username, created_at` 聚合。
- `GET /api/data/self`（`UserAuth`）→ 按 `model_name, created_at` 聚合，且时间跨度 > 1 个月会被拒绝（`260000` 秒校验在 handler 内）。
- 上述 `/api/data/*` 来自 `quota_data` 汇总表，由数据导出任务按 `DataExportInterval` 写入 → 分钟级延迟。UI 必须显示"汇总数据每 N 分钟更新"。
- `GET /api/log` / `GET /api/log/self` → `{data:{items, total, page, page_size}}`；参数 `p`、`page_size`、`type`、`start_timestamp`、`end_timestamp`、`model_name`、`token_name`、`group`、`request_id`（admin 另支持 `username`、`channel`）。
- `GET /api/model-monitor/summary` → `{data:{enabled, sites:[{site, summary:{score, health, models:[{model_name, status, latest_status, latest_failure_type, latest_error_summary, weight, stale}]}, channel_ids, latest_observed_at, freshness_seconds}]}}`。
- `GET /api/model-monitor/sites/:site_id` → 单站点，含 `observations`（近 24 小时）。
- `GET /api/log/routing_stats?start_timestamp&end_timestamp&model_name&group&channel_id` → `{data:{by_model_channel:[{model_name, channel_id, channel_name, requests, errors, avg_use_time, priority?, weight?}], switches:[{from,to,count}], reasons:[{reason,count}], affinity:[{rule_name, sticky_requests, distinct_keys}], window:{start,end,bucket_seconds}, trend:[{timestamp,requests,switched}]}}`；`priority`/`weight` 在渠道被删除时缺省。
- `GET /api/log/channel_affinity_bindings?limit=` → 当前粘滞会话。
- `GET /api/channel/?p&page_size&id&name&status&group&type`（`ChannelRead`）；`POST /api/channel/:id/status` body `{"status": n}`（`ChannelOperate`）。
  - 渠道状态：`1` 启用、`2` 手动禁用、`3` 自动禁用。`isManageableChannelStatus` 只接受 `1` 和 `2`：对 `status = 3` 的渠道，写操作只能发 `1`。UI 必须把 `3` 单独显示为"自动禁用"，不得与开关的 off 状态混淆。
- 认证：`middleware/auth.go` 的 `classifyDashboardCredential` 接受 `Authorization: Bearer <PAT>`（`model.ValidateAccessToken`）。PAT 请求 `SessionID` 为空，本计划的接口全部只依赖角色/权限，均可使用。

---

## 2. 架构与文件结构

### 2.1 为什么手机端放在 `web/src/mobile/` 而不是新建 `web-mobile/`

- 仓库强制"组件与格式化工具先复用"（`AGENTS.md` 前端规则、`web/AGENTS.md` 3.3）。独立目录无法复用 `@/components/ui/*`、`@/lib/format`、`@/lib/currency`、`@/i18n/languages`，还会复制一份依赖版本与 locale 文件。
- `web/vitest.config.ts` 的 `include` 是 `src/**/*.{test,spec}.{ts,tsx}`，`tsconfig.app.json` 的 `include` 是 `["src"]`，`scripts/add-copyright.mjs` 的 `TARGET_DIRS` 是 `['src','scripts']`。把代码放进 `src/mobile/` 时这三套工具**零改动**即可生效。
- 桌面构建仍由 `web/rsbuild.config.ts` 独立输出到 `web/dist`，手机端由新的 `web/rsbuild.mobile.config.ts` 输出到 `web/mobile-dist`，两者互不包含。

### 2.2 新增/修改文件清单

**后端**

| 文件 | 责任 |
| --- | --- |
| `router/web-router.go`（改） | UA 判定、根路径重定向、`/m` 静态服务与 SPA 回落、`WebAssets` 新字段 |
| `router/web_router_test.go`（新增，唯一后端测试文件） | 上述行为的表驱动测试 + `/m` 静态与回落测试 |
| `router/testdata/web-dist/index.html`（新增，测试夹具） | 桌面壳夹具 |
| `router/testdata/mobile-dist/index.html`、`router/testdata/mobile-dist/static/app.js`（新增，测试夹具） | 手机壳夹具 |
| `main.go`（改） | `//go:embed web/mobile-dist`、`//go:embed web/mobile-dist/index.html`、传参 |
| `makefile`（改） | `build-mobile-web` 目标并入 `build-all-web` |
| `Dockerfile`（改） | builder 阶段产出并拷贝 `web/mobile-dist` |
| `.github/workflows/ci.yml`、`release.yml`（改） | 构建或桩目录补齐（`go build` 要求 embed 目标存在） |
| `.gitignore`（改） | 忽略 `web/mobile-dist` |

**前端（新增，全部在 `web/` 内）**

| 文件 | 责任 |
| --- | --- |
| `web/rsbuild.mobile.config.ts` | 手机端独立构建：入口 `src/mobile/main.tsx`、输出 `mobile-dist`、`assetPrefix '/m/'` |
| `web/src/mobile/index.html` | 手机壳 HTML 模板（无 umami/GA 占位符） |
| `web/src/mobile/main.tsx` | 挂载 React 根、全局样式、Toaster 与 `ThemeProvider`（Task 5 抽到 `providers.tsx`） |
| `web/src/mobile/app.tsx` | 认证门 + TabBar + 当前页签渲染 |
| `web/src/mobile/styles/mobile.css` | 复用 `@/styles/index.css` + 手机端专属（safe-area、dvh、overscroll、tabular-nums） |
| `web/src/mobile/types.ts` | 共享类型（页签、时间范围、API 响应、行类型） |
| `web/src/mobile/lib/pat-store.ts` | PAT 持久化与校验 |
| `web/src/mobile/lib/api-client.ts` | fetch 封装、`ApiError`、401/403 处理 |
| `web/src/mobile/lib/query-client.ts` | QueryClient 与 `staleTime` 常量 |
| `web/src/mobile/lib/router.ts` | hash 路由（`#/usage` 等）与 `useActiveTab` |
| `web/src/mobile/components/mobile-tab-bar.tsx` | 底部 4 页签（唯一新增通用控件） |
| `web/src/mobile/components/kpi-card.tsx` | KPI 卡 |
| `web/src/mobile/components/value-row.tsx` | 标签 + 值的行（列表项基元） |
| `web/src/mobile/components/pull-to-refresh.tsx` | 下拉刷新容器 |
| `web/src/mobile/components/pat-gate.tsx` | 未配置 PAT / 令牌失效时的门 |
| `web/src/mobile/components/mini-bar.tsx` | 自绘 SVG 占比条与迷你折线 |
| `web/src/mobile/features/usage/{api.ts,lib/usage-summary.ts,components/usage-page.tsx,components/recent-requests.tsx}` | 用量页 |
| `web/src/mobile/features/models/{api.ts,lib/availability.ts,components/models-page.tsx}` | 模型可用性页 |
| `web/src/mobile/features/routing/{api.ts,lib/routing-share.ts,components/routing-page.tsx}` | 路由统计页 |
| `web/src/mobile/features/channels/{api.ts,lib/channel-status.ts,components/channels-page.tsx}` | 渠道页 |

测试文件（每个模块自己的 `__tests__/`）：`web/src/mobile/lib/__tests__/{pat-store,api-client,router}.test.ts`、`web/src/mobile/features/usage/lib/__tests__/usage-summary.test.ts`、`web/src/mobile/features/models/lib/__tests__/availability.test.ts`、`web/src/mobile/features/routing/lib/__tests__/routing-share.test.ts`、`web/src/mobile/features/channels/lib/__tests__/channel-status.test.ts`、`web/src/mobile/components/__tests__/{mobile-tab-bar,kpi-card,pull-to-refresh,pat-gate,mini-bar}.test.tsx`、`web/src/mobile/features/{usage,models,routing,channels}/components/__tests__/*-page.test.tsx`、`web/src/mobile/__tests__/app.test.tsx`。

---

## Task 1：后端 UA 判定与根路径重定向

**Files:**
- Modify: `router/web-router.go`
- Create: `router/web_router_test.go`
- Create: `router/testdata/web-dist/index.html`

**Interfaces:**
- Consumes: 现有 `common.EmbedFolder`、`middleware.RouteTag`、`middleware.AccessTokenAudit`、`controller.RelayNotFound`。
- Produces（后续 Task 2/4 依赖这些确切签名）:
  - `const desktopPreferenceCookie = "newapi_prefer_desktop"`
  - `func isMobileUserAgent(userAgent string) bool`
  - `func prefersDesktopShell(c *gin.Context) bool`
  - `func desktopShellHandler(indexPage []byte) gin.HandlerFunc`

- [ ] **Step 1: 写失败测试**

创建 `router/testdata/web-dist/index.html`（内容仅一行，用作夹具）：

```html
<!doctype html><title>desktop shell</title>
```

创建 `router/web_router_test.go`：

```go
/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package router

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/web-dist
var testDesktopFS embed.FS

const testDesktopIndex = "<!doctype html><title>desktop shell</title>"

func newTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.NoRoute(desktopShellHandler([]byte(testDesktopIndex)))
	return engine
}

func TestIsMobileUserAgent(t *testing.T) {
	cases := []struct {
		name      string
		userAgent string
		want      bool
	}{
		{name: "iPhone Safari", userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1", want: true},
		{name: "Android Chrome", userAgent: "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36", want: true},
		{name: "Opera Mini", userAgent: "Opera/9.80 (J2ME/MIDP; Opera Mini/9.80) Presto/2.12.423 Version/12.16", want: true},
		{name: "desktop Chrome", userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36", want: false},
		{name: "desktop Safari on macOS", userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15", want: false},
		{name: "iPadOS 13 desktop UA", userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15", want: false},
		{name: "empty", userAgent: "", want: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, isMobileUserAgent(testCase.userAgent))
		})
	}
}

func TestDesktopShellRedirectsMobileRootToMobileConsole(t *testing.T) {
	engine := newTestEngine(t)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148 Safari/604.1")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusFound, recorder.Code)
	assert.Equal(t, "/m", recorder.Header().Get("Location"))
}

func TestDesktopShellCases(t *testing.T) {
	cases := []struct {
		name          string
		target        string
		userAgent     string
		cookie        string
		wantStatus    int
		wantLocation  string
		wantBody      string
	}{
		{name: "desktop root renders desktop shell", target: "/", userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0.0.0 Safari/537.36", wantStatus: http.StatusOK, wantBody: testDesktopIndex},
		{name: "mobile root with desktop=1 stays on desktop", target: "/?desktop=1", userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148", wantStatus: http.StatusOK, wantBody: testDesktopIndex},
		{name: "mobile root with preference cookie stays on desktop", target: "/", userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148", cookie: desktopPreferenceCookie + "=1", wantStatus: http.StatusOK, wantBody: testDesktopIndex},
		{name: "mobile deep link is not redirected", target: "/usage-logs", userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148", wantStatus: http.StatusOK, wantBody: testDesktopIndex},
		{name: "api path returns relay not found", target: "/api/unknown", userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148", wantStatus: http.StatusNotFound},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine := newTestEngine(t)
			request := httptest.NewRequest(http.MethodGet, testCase.target, nil)
			request.Header.Set("User-Agent", testCase.userAgent)
			if testCase.cookie != "" {
				request.Header.Set("Cookie", testCase.cookie)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)

			require.Equal(t, testCase.wantStatus, recorder.Code)
			if testCase.wantLocation != "" {
				assert.Equal(t, testCase.wantLocation, recorder.Header().Get("Location"))
			}
			if testCase.wantBody != "" {
				assert.Equal(t, testCase.wantBody, recorder.Body.String())
			}
			if testCase.wantStatus == http.StatusOK {
				assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui && go test ./router/ -run 'TestIsMobileUserAgent|TestDesktopShell' -v`
Expected: FAIL — 编译错误 `undefined: isMobileUserAgent`、`undefined: desktopShellHandler`、`undefined: desktopPreferenceCookie`。

- [ ] **Step 3: 写最小实现**

把 `router/web-router.go` 改为：

```go
package router

import (
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

// desktopPreferenceCookie is set by the mobile console when the operator taps
// "Desktop version", so a mobile user agent stops being redirected to /m.
const desktopPreferenceCookie = "newapi_prefer_desktop"

// mobileUserAgentMarkers is a small allowlist of tokens that phone browsers
// ship. iPadOS 13+ reports a desktop Safari user agent and is deliberately not
// matched; such devices stay on the desktop dashboard.
var mobileUserAgentMarkers = []string{
	"Mobile", "Android", "iPhone", "iPod", "Windows Phone", "IEMobile", "BlackBerry", "Opera Mini", "webOS",
}

func isMobileUserAgent(userAgent string) bool {
	for _, marker := range mobileUserAgentMarkers {
		if strings.Contains(userAgent, marker) {
			return true
		}
	}
	return false
}

// prefersDesktopShell reports whether a mobile-looking browser explicitly asked
// for the desktop dashboard, with ?desktop=1 or with the preference cookie.
func prefersDesktopShell(c *gin.Context) bool {
	if c.Query("desktop") == "1" {
		return true
	}
	cookie, err := c.Cookie(desktopPreferenceCookie)
	return err == nil && cookie == "1"
}

// desktopShellHandler serves the desktop SPA entry and redirects the root path
// of mobile browsers to the mobile console. It runs after static.Serve, so only
// paths with no matching embedded file reach it.
func desktopShellHandler(indexPage []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		if c.Request.Method == http.MethodGet && c.Request.URL.Path == "/" &&
			isMobileUserAgent(c.GetHeader("User-Agent")) && !prefersDesktopShell(c) {
			c.Redirect(http.StatusFound, "/m")
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	}
}

// SetWebRouter serves the desktop shell. gzip, rate limiting and caching stay
// inside the web chain instead of being registered with router.Use, so the set
// and order of middleware running for other routes on the engine is unchanged.
func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")

	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		gzip.Gzip(gzip.DefaultCompression),
		middleware.AccessTokenAudit(),
		middleware.GlobalWebRateLimit(),
		middleware.Cache(),
		static.Serve("/", frontendFS),
		desktopShellHandler(assets.IndexPage),
	)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui && gofmt -l router/web-router.go router/web_router_test.go && go test ./router/ -run 'TestIsMobileUserAgent|TestDesktopShell' -v`
Expected: PASS（`gofmt -l` 无输出）。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add router/web-router.go router/web_router_test.go router/testdata/web-dist/index.html
git commit -m "feat(web): detect phone user agents and redirect the root path to /m"
```

---

## Task 2：`/m` 静态服务与 SPA 回落

**Files:**
- Modify: `router/web-router.go`
- Modify: `router/web_router_test.go`
- Create: `router/testdata/mobile-dist/index.html`、`router/testdata/mobile-dist/static/app.js`

**Interfaces:**
- Consumes: Task 1 的 `desktopShellHandler`、`WebAssets`。
- Produces:
  - `WebAssets.MobileBuildFS embed.FS`、`WebAssets.MobileIndexPage []byte`
  - `func withWebChain(pluginDispatcher gin.HandlerFunc, extra ...gin.HandlerFunc) []gin.HandlerFunc`
  - `func buildMobileShell(fs static.ServeFileSystem, indexPage []byte) gin.HandlerFunc`
  - 路由：`GET /m`、`GET /m/*path`

- [ ] **Step 1: 写失败测试**

创建夹具 `router/testdata/mobile-dist/index.html`：

```html
<!doctype html><title>mobile shell</title>
```

创建夹具 `router/testdata/mobile-dist/static/app.js`：

```js
console.log('mobile shell asset')
```

在 `router/web_router_test.go` 末尾追加（并把 `testMobileFS` 的 embed 指令加在 `testDesktopFS` 旁边，另在 import 中加入 `"github.com/QuantumNous/new-api/common"`）：

```go
//go:embed testdata/mobile-dist
var testMobileFS embed.FS

const testMobileIndex = "<!doctype html><title>mobile shell</title>"

func newTestMobileEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	shell := buildMobileShell(common.EmbedFolder(testMobileFS, "testdata/mobile-dist"), []byte(testMobileIndex))
	engine.GET("/m", shell)
	engine.GET("/m/*path", shell)
	return engine
}

func TestMobileShellServesIndexAndFallsBackForDeepLinks(t *testing.T) {
	cases := []struct {
		name         string
		target       string
		wantStatus   int
		wantBody     string
		wantNoCache  bool
	}{
		{name: "root of mobile console", target: "/m", wantStatus: http.StatusOK, wantBody: testMobileIndex, wantNoCache: true},
		{name: "deep link falls back to mobile index", target: "/m/routing", wantStatus: http.StatusOK, wantBody: testMobileIndex, wantNoCache: true},
		{name: "hashed asset is served from the mobile build", target: "/m/static/app.js", wantStatus: http.StatusOK, wantBody: "console.log('mobile shell asset')\n", wantNoCache: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine := newTestMobileEngine(t)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testCase.target, nil))

			require.Equal(t, testCase.wantStatus, recorder.Code)
			assert.Equal(t, testCase.wantBody, recorder.Body.String())
			if testCase.wantNoCache {
				assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
			} else {
				assert.Empty(t, recorder.Header().Get("Cache-Control"))
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui && go test ./router/ -run TestMobileShell -v`
Expected: FAIL — `undefined: buildMobileShell`。

- [ ] **Step 3: 写最小实现**

在 `router/web-router.go` 中扩展 `WebAssets` 并新增两个函数，同时把 `SetWebRouter` 更新为：

```go
// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS         embed.FS
	IndexPage       []byte
	MobileBuildFS   embed.FS
	MobileIndexPage []byte
}

// withWebChain builds the per-request middleware chain shared by the desktop
// fallback and the mobile console. The order matches the pre-existing desktop
// chain exactly, and the chain is never registered with router.Use, so adding
// the mobile routes cannot widen gzip, caching or rate limiting to other
// routes on the engine.
func withWebChain(pluginDispatcher gin.HandlerFunc, extra ...gin.HandlerFunc) []gin.HandlerFunc {
	chain := []gin.HandlerFunc{
		pluginDispatcher,
		middleware.RouteTag("web"),
		gzip.Gzip(gzip.DefaultCompression),
		middleware.AccessTokenAudit(),
		middleware.GlobalWebRateLimit(),
		middleware.Cache(),
	}
	return append(chain, extra...)
}

// buildMobileShell serves the mobile console: real files under /m come from the
// mobile build, everything else returns the mobile index so client-side deep
// links keep working. Cache-Control for the index is forced to no-cache because
// middleware.Cache sets a one-week max-age for every non-root path.
func buildMobileShell(fs static.ServeFileSystem, indexPage []byte) gin.HandlerFunc {
	fileServer := http.StripPrefix("/m", http.FileServer(fs))
	return func(c *gin.Context) {
		relativePath := strings.TrimPrefix(c.Request.URL.Path, "/m")
		if relativePath != "" {
			if file, err := fs.Open(relativePath); err == nil {
				_ = file.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	}
}

func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")
	mobileFS := common.EmbedFolder(assets.MobileBuildFS, "web/mobile-dist")
	mobileShell := buildMobileShell(mobileFS, assets.MobileIndexPage)

	router.GET("/m", withWebChain(pluginDispatcher, mobileShell)...)
	router.GET("/m/*path", withWebChain(pluginDispatcher, mobileShell)...)
	router.NoRoute(
		withWebChain(pluginDispatcher,
			static.Serve("/", frontendFS),
			desktopShellHandler(assets.IndexPage),
		)...,
	)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui && gofmt -l router/ && go test ./router/ -run 'TestIsMobileUserAgent|TestDesktopShell|TestMobileShell' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add router/web-router.go router/web_router_test.go router/testdata/mobile-dist
git commit -m "feat(web): serve the mobile console at /m with an SPA fallback"
```

---

## Task 3：手机端构建入口与最小可运行壳

**Files:**
- Create: `web/rsbuild.mobile.config.ts`、`web/src/mobile/index.html`、`web/src/mobile/main.tsx`、`web/src/mobile/app.tsx`、`web/src/mobile/styles/mobile.css`、`web/src/mobile/types.ts`、`web/src/mobile/lib/i18n.ts`
- Create: `web/src/mobile/__tests__/app.test.tsx`、`web/src/mobile/lib/__tests__/i18n.test.ts`
- Modify: `web/package.json`、`.gitignore`

**Interfaces:**
- Produces: `MobileTab`、`MOBILE_TABS`（后续 Task 7 使用）；`initializeMobileI18n()`、`resolveMobileLocale()`（Task 5 的 `providers.tsx` 与后续所有页面使用）；`web/mobile-dist/` 构建产物（Task 4 的 embed 目标）。

- [ ] **Step 1: 写失败测试**

`web/src/mobile/types.ts` 先定义（正式代码，Step 3 会补齐其余类型）：

```ts
export type MobileTab = 'usage' | 'models' | 'routing' | 'channels'
```

`web/src/mobile/__tests__/app.test.tsx`：

```tsx
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { MobileApp } from '@/mobile/app'

describe('MobileApp', () => {
  it('renders the mobile console heading', () => {
    render(<MobileApp />)

    expect(screen.getByRole('heading', { name: 'Mobile console' })).toBeInTheDocument()
  })
})
```

`web/src/mobile/lib/__tests__/i18n.test.ts`（语言包必须按需加载：首屏预算与 i18n 同时成立的唯一方式）：

```ts
import i18n from 'i18next'
import { beforeEach, describe, expect, it } from 'vitest'

import { initializeMobileI18n, resolveMobileLocale } from '@/mobile/lib/i18n'

describe('resolveMobileLocale', () => {
  it('maps browser tags onto the interface language codes', () => {
    expect(resolveMobileLocale('zh')).toBe('zhCN')
    expect(resolveMobileLocale('zh-CN')).toBe('zhCN')
    expect(resolveMobileLocale('zh-Hant-TW')).toBe('zhTW')
    expect(resolveMobileLocale('fr-FR')).toBe('fr')
    expect(resolveMobileLocale('de-DE')).toBe('en')
  })

  it('keeps interface language codes the desktop already cached', () => {
    expect(resolveMobileLocale('zhTW')).toBe('zhTW')
    expect(resolveMobileLocale('zhCN')).toBe('zhCN')
    expect(resolveMobileLocale('ja')).toBe('ja')
  })
})

describe('initializeMobileI18n', () => {
  beforeEach(async () => {
    localStorage.clear()
    await i18n.changeLanguage('en')
  })

  it('loads only the active locale bundle', async () => {
    localStorage.setItem('i18nextLng', 'zh')

    await initializeMobileI18n()

    expect(i18n.resolvedLanguage).toBe('zhCN')
    expect(i18n.hasResourceBundle('zhCN', 'translation')).toBe(true)
    expect(i18n.hasResourceBundle('en', 'translation')).toBe(true)
    expect(i18n.hasResourceBundle('ja', 'translation')).toBe(false)
    expect(i18n.t('Usage')).not.toBe('Usage')
  })

  it('keeps a cached Traditional Chinese preference across reloads', async () => {
    localStorage.setItem('i18nextLng', 'zhTW')

    await initializeMobileI18n()
    await initializeMobileI18n()

    expect(i18n.resolvedLanguage).toBe('zhTW')
    expect(localStorage.getItem('i18nextLng')).toBe('zhTW')
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile/__tests__/app.test.tsx`
Expected: FAIL — 无法解析 `@/mobile/app`。

- [ ] **Step 3: 写最小实现**

`web/src/mobile/app.tsx`（版权头 + 最小壳）：

```tsx
import { useTranslation } from 'react-i18next'

export function MobileApp() {
  const { t } = useTranslation()

  return (
    <main className='min-h-dvh bg-background text-foreground'>
      <header className='px-4 pt-[max(1rem,env(safe-area-inset-top))] pb-2'>
        <h1 className='text-lg font-semibold'>{t('Mobile console')}</h1>
      </header>
    </main>
  )
}
```

`web/src/mobile/lib/i18n.ts`：手机端自己的 i18n 入口。桌面 `@/i18n/config` 一次性静态 import 七个语言包（~4 MB 源码 / ~1.1 MB gzip），静态引入它会直接击穿首屏预算。这里先以空 resources 初始化并立即渲染（未加载时 `t()` 返回英文源 key，本身就是可读文案），再只把当前语言一个 chunk 拉回来 `addResourceBundle`。locale 文件仍与桌面共用，语言码归一仍走 `@/i18n/languages`。

```ts
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import { convertDetectedLanguage } from '@/i18n/languages'

// Each locale file holds every desktop string, so locales are only ever loaded
// as separate async chunks: the console ships no locale data in its first screen.
const localeLoaders = {
  en: () => import('@/i18n/locales/en.json'),
  zhCN: () => import('@/i18n/locales/zh.json'),
  zhTW: () => import('@/i18n/locales/zh-TW.json'),
  fr: () => import('@/i18n/locales/fr.json'),
  ru: () => import('@/i18n/locales/ru.json'),
  ja: () => import('@/i18n/locales/ja.json'),
  vi: () => import('@/i18n/locales/vi.json'),
} as const

type MobileLocaleCode = keyof typeof localeLoaders

const mobileLocaleCodes = Object.keys(localeLoaders) as MobileLocaleCode[]

function isMobileLocaleCode(value: string): value is MobileLocaleCode {
  return Object.hasOwn(localeLoaders, value)
}

// The shared `i18nextLng` cache the desktop writes already holds interface codes
// (`zhTW`), so an exact match has to win before the BCP-47 mapping runs:
// `convertDetectedLanguage('zhTW')` reads that code as a plain `zh` tag and
// would downgrade it to `zhCN`. Only then reuse the project's browser-tag
// mapping (`zh` -> `zhCN`, `zh-Hant-TW` -> `zhTW`) and narrow region tags onto
// their primary code (`fr-FR` -> `fr`).
export function resolveMobileLocale(value: string): MobileLocaleCode {
  const trimmed = value.trim()
  if (isMobileLocaleCode(trimmed)) return trimmed

  const converted = convertDetectedLanguage(trimmed.toLowerCase())
  if (isMobileLocaleCode(converted)) return converted

  const primary = converted.split('-')[0]
  return isMobileLocaleCode(primary) ? primary : 'en'
}

function detectedLocaleValue(): string {
  const stored = localStorage.getItem('i18nextLng')
  return stored ?? navigator.language
}

async function loadLocale(code: MobileLocaleCode): Promise<void> {
  const [bundle, fallback] = await Promise.all([
    localeLoaders[code](),
    code === 'en' ? undefined : localeLoaders.en(),
  ])

  // A locale file is its own namespace map (`{ translation: { ... } }`), the same
  // shape `@/i18n/config` feeds to i18next's `resources` option.
  for (const [namespace, strings] of Object.entries(bundle.default)) {
    i18n.addResourceBundle(code, namespace, strings, true, true)
  }
  if (fallback) {
    for (const [namespace, strings] of Object.entries(fallback.default)) {
      i18n.addResourceBundle('en', namespace, strings, true, true)
    }
  }
  await i18n.changeLanguage(code)
}

export async function initializeMobileI18n(): Promise<void> {
  const code = resolveMobileLocale(detectedLocaleValue())

  await i18n.use(initReactI18next).init({
    resources: {},
    lng: code,
    fallbackLng: 'en',
    supportedLngs: mobileLocaleCodes,
    load: 'currentOnly',
    nsSeparator: false, // Allow literal colons in keys (e.g. URLs, labels)
    debug: import.meta.env.DEV,
    interpolation: { escapeValue: false },
  })

  localStorage.setItem('i18nextLng', code)
  await loadLocale(code)
}
```

`web/src/mobile/styles/mobile.css`：先复用桌面全部 token，再附加手机端基线（Tailwind v4 用 `@import`）：

```css
@import '../../styles/index.css';

html {
  /* Avoid the double-tap zoom delay on Android; the console has no pinch-zoom content. */
  touch-action: manipulation;
  overscroll-behavior-y: contain;
}

body {
  /* Numbers must not jitter while polling; every metric column uses digits of equal width. */
  font-variant-numeric: tabular-nums;
  padding-bottom: env(safe-area-inset-bottom, 0px);
}

@media (prefers-reduced-motion: reduce) {
  * {
    animation-duration: 0.01ms !important;
    transition-duration: 0.01ms !important;
  }
}
```

`web/src/mobile/main.tsx`（本任务只挂载最小壳；Task 5 把 Provider 抽到 `src/mobile/providers.tsx`）：

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Toaster } from '@/components/ui/sonner'
import { ThemeProvider } from '@/context/theme-provider'
import '@/mobile/styles/mobile.css'

import { MobileApp } from '@/mobile/app'
import { initializeMobileI18n } from '@/mobile/lib/i18n'

const container = document.getElementById('root')
if (container) {
  // Not awaited: the shell renders with the English source keys first and
  // re-renders once the active locale chunk arrives.
  void initializeMobileI18n()

  createRoot(container).render(
    <StrictMode>
      <ThemeProvider>
        <MobileApp />
        <Toaster />
      </ThemeProvider>
    </StrictMode>
  )
}
```

`ThemeProvider`（`@/context/theme-provider`）直接复用桌面主题实现：默认 `system`、写入 `html.light` / `html.dark`、与桌面共用同一个 localStorage 键，因此手机端自动跟随系统深浅色，并继承操作员在桌面上已选的主题。

Task 5 再插入 `QueryClientProvider` 包裹（该步骤的 diff 在 Task 5 Step 3 中给出）。

`web/src/mobile/index.html`（无 umami/GA 占位符，见 1.2）：

```html
<!doctype html>
<html lang='en'>
  <head>
    <meta charset='UTF-8' />
    <meta
      name='viewport'
      content='width=device-width, initial-scale=1, viewport-fit=cover, maximum-scale=5'
    />
    <meta name='color-scheme' content='light dark' />
    <meta name='theme-color' content='#ffffff' media='(prefers-color-scheme: light)' />
    <meta name='theme-color' content='#0a0a0a' media='(prefers-color-scheme: dark)' />
    <title>new-api</title>
  </head>
  <body>
    <div id='root'></div>
  </body>
</html>
```

`web/rsbuild.mobile.config.ts`（对照 `rsbuild.config.ts` 裁剪，只保留必要项）：

```ts
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { defineConfig, loadEnv } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig(({ envMode }) => {
  const env = loadEnv({ mode: envMode, prefixes: ['VITE_'] })
  const serverUrl =
    process.env.VITE_REACT_APP_SERVER_URL ||
    env.rawPublicVars.VITE_REACT_APP_SERVER_URL ||
    'http://localhost:3000'
  const isProd = envMode === 'production'

  return {
    plugins: [pluginReact(), pluginTailwindcss({ optimize: false })],
    source: { entry: { index: './src/mobile/main.tsx' } },
    resolve: { alias: { '@': path.resolve(__dirname, './src') } },
    html: { template: './src/mobile/index.html' },
    server: {
      host: '0.0.0.0',
      strictPort: false,
      base: '/m',
      proxy: { '/api': { target: serverUrl, changeOrigin: true } },
    },
    output: {
      minify: isProd,
      target: 'web',
      // Assets live under /m so buildMobileShell can strip the prefix and serve
      // them straight from web/mobile-dist.
      assetPrefix: '/m',
      distPath: { root: 'mobile-dist' },
    },
  }
})
```

`web/package.json` 的 `scripts` 增加两行（保持字母序位置与现有一致即可）：

```json
    "dev:mobile": "rsbuild dev -c rsbuild.mobile.config.ts",
    "build:mobile": "tsgo -b && rsbuild build -c rsbuild.mobile.config.ts",
```

`.gitignore` 在 `web/dist` 下一行增加：

```
web/mobile-dist
```

- [ ] **Step 4: 运行测试与构建确认通过**

Run:
```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web
bun run test -- src/mobile
bun run typecheck
bunx oxlint -c .oxlintrc.json src/mobile
# The check is red at the baseline (25 pre-existing files); only the new mobile
# files matter here.
bun run copyright:check 2>&1 | grep -E 'src/mobile|rsbuild\.mobile' && exit 1 || true
bun run build:mobile
ls mobile-dist/index.html
grep -o '/m/[^"]*\.js' mobile-dist/index.html | head -3
# First-screen JS budget (< 120 KB gzip): sum the gzip size of every script the
# built index.html loads eagerly. Locale files must be async chunks instead.
for f in $(grep -o '/m/static/js/[^"]*\.js' mobile-dist/index.html); do
  gzip -c "mobile-dist${f#/m}" | wc -c
done
```
Expected: 测试 PASS；typecheck 无错误；lint 无 error；`mobile-dist/index.html` 存在；`grep` 输出至少一行以 `/m/` 开头的 JS 资源路径；上面 `gzip -c | wc -c` 各值之和 < 122880（120 KB）。超标时先确认语言包没被打进首屏 chunk：`grep -l 'locales/' -r mobile-dist/static/js` 应当只命中异步 chunk（`index.html` 里没有引用它们）。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add .gitignore web/package.json web/rsbuild.mobile.config.ts web/src/mobile
git commit -m "feat(web-mobile): add the /m build entry and an empty mobile shell"
```

---

## Task 4：后端装配与构建流水线接入

**Files:**
- Modify: `main.go`、`makefile`、`Dockerfile`、`Dockerfile.dev`、`.dockerignore`、`.github/workflows/ci.yml`、`.github/workflows/release.yml`、`.github/workflows/electron-build.yml`、`electron/build.sh`、`scripts/build_release_candidate.sh`

**Interfaces:**
- Consumes: Task 2 的 `WebAssets.MobileBuildFS` / `WebAssets.MobileIndexPage`；Task 3 的 `web/mobile-dist`。
- Produces: 生产二进制在 `FRONTEND_BASE_URL` 为空时提供 `/m`。

- [ ] **Step 1: 让构建失败（先证伪装配）**

先只加入 `main.go` 里的两个 `//go:embed web/mobile-dist` 声明（Step 2 的其余装配与流水线改动留到 Step 3 之前一起做）。

工作树里 `web/dist` 是 gitignored 且此时还不存在，先给它一个桩，否则失败信息会是 `pattern web/dist`，掩盖你要验证的信号：

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
mkdir -p web/dist && printf '<!doctype html><title>desktop stub</title>\n' > web/dist/index.html
rm -rf web/mobile-dist && go build ./...
```
Expected: FAIL — `pattern web/mobile-dist: no matching files found`（说明 embed 目标确实被需要）。

- [ ] **Step 2: 写出装配实现**

`main.go` 的 embed 区（紧接现有 `//go:embed web/dist/index.html` 之后）增加：

```go
//go:embed web/mobile-dist
var mobileBuildFS embed.FS

//go:embed web/mobile-dist/index.html
var mobileIndexPage []byte
```

`main.go` 的 `router.SetRouter` 调用改为：

```go
	router.SetRouter(server, router.WebAssets{
		BuildFS:         buildFS,
		IndexPage:       indexPage,
		MobileBuildFS:   mobileBuildFS,
		MobileIndexPage: mobileIndexPage,
	})
```

`makefile`：在 `build-web` 之后新增目标，并让 `build-all-web` 依赖两个 Web 目标：

```make
build-mobile-web:
	@echo "Building mobile web frontend..."
	@cd $(WEB_DIR) && bun install --frozen-lockfile
	@cd $(WEB_DIR) && bun run build:mobile

build-all-web: build-web build-mobile-web
```

`Dockerfile`：builder 阶段改为同时产出两套产物，并在 builder2 阶段拷贝：

```dockerfile
RUN DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat /build/VERSION) bun run build
RUN DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat /build/VERSION) bun run build:mobile
```

```dockerfile
COPY --from=builder /build/web/dist ./web/dist
COPY --from=builder /build/web/mobile-dist ./web/mobile-dist
```

`.github/workflows/ci.yml`：把现有 `web/dist` 桩目录步骤扩展为同时创建 `web/mobile-dist` 桩：

```yaml
          mkdir -p web/dist web/mobile-dist
          touch web/dist/index.html web/mobile-dist/index.html
```

`.github/workflows/release.yml`：三处 `bun run build` 之后各加一行（共三处，行号按文件当前内容就近插入）：

```yaml
          DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$VERSION bun run build:mobile
```

- [ ] **Step 2b: 补齐其余会 `go build` 的路径（embed sweep）**

`//go:embed web/mobile-dist` 是无条件的，只准备 `web/dist` 的路径现在会编译失败。逐个补齐（每一处都照该文件既有风格就近插入）：

| 路径 | 需要做的事 |
| --- | --- |
| `.github/workflows/electron-build.yml` | 在 `bun run build` 那一行之后加 `DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(git describe --tags) bun run build:mobile` |
| `electron/build.sh` | 在 `bun run build` 之后、`cd ../electron` 之前加同一行（`electron/build.sh` 里版本号来自 `git describe --tags --always`，用该文件现有写法） |
| `Dockerfile.dev` | 桩命令同时建两个目录与两个 `index.html`（它本来就不构建真实前端） |
| `.dockerignore` | `/web/dist` 下一行加 `/web/mobile-dist`，否则本地陈旧的 mobile 产物会被 `COPY . .` 带进 builder2 并被 `//go:embed` 一并嵌进二进制（`COPY --from` 是合并语义，不清理目标多余文件） |
| `scripts/build_release_candidate.sh` | `build_embed_assets` 增加 `bun run build:mobile`；`validate_embed_assets`、`restore_embed_assets_from_cache`、`store_embed_assets_in_cache` 都要把 `mobile-dist` 与 `dist` 同等对待（缓存条目里同时存 `dist/` 与 `mobile-dist/`），否则本地发布候选流水线只会产出桌面产物 |
| `makefile` | 第 47 行那条注释同步说明两个 embed 目标由 `build-all-web` 覆盖 |

已知但仍不处理：`scripts/post-rebuild-patches.sh` 在运行容器里 `go build`，容器最终阶段既无 `web/dist` 也无 `web/mobile-dist` —— 本次改动前它就已经失败，保持现状，只在报告里登记。

Run:
```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
grep -n "build:mobile" .github/workflows/electron-build.yml electron/build.sh Dockerfile Dockerfile.dev scripts/build_release_candidate.sh makefile
grep -n "web/mobile-dist" .github/workflows/ci.yml Dockerfile Dockerfile.dev .dockerignore scripts/build_release_candidate.sh
bash scripts/test_release_helpers.sh 2>&1 | tail -5
```
Expected: 每条路径都能在 grep 结果里看到；`scripts/test_release_helpers.sh` 通过（若它断言了 `build_release_candidate.sh` 的措辞而失败，就同步更新该测试脚本，它也在允许改动清单里）。

- [ ] **Step 3: 构建并运行后端测试确认通过**

Run:
```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run build:mobile
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui && go build ./... && go test ./router/ -run 'TestIsMobileUserAgent|TestDesktopShell|TestMobileShell' -v
git diff --stat makefile Dockerfile .github/workflows
```
Expected: `go build ./...` 成功（无输出）；router 测试 PASS；`git diff --stat` 显示 makefile/Dockerfile/两个 workflow 均有改动。

- [ ] **Step 4: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add main.go makefile Dockerfile .github/workflows/ci.yml .github/workflows/release.yml
git commit -m "feat(web): embed and build the mobile console in every pipeline"
```

---

## Task 5：PAT 存储、API 客户端与查询客户端

**Files:**
- Create: `web/src/mobile/lib/pat-store.ts`、`web/src/mobile/lib/api-client.ts`、`web/src/mobile/lib/query-client.ts`、`web/src/mobile/providers.tsx`
- Create: `web/src/mobile/lib/__tests__/pat-store.test.ts`、`web/src/mobile/lib/__tests__/api-client.test.ts`
- Modify: `web/src/mobile/main.tsx`

**Interfaces:**
- Produces:
  - `readPat(): string`、`writePat(pat: string): void`、`clearPat(): void`、`isPlausiblePat(value: string): boolean`
  - `class ApiError extends Error { code: 'unauthorized' | 'forbidden' | 'http' | 'network' | 'business'; status: number; apiCode?: string }`
  - `mobileApiGet<T>(path: string, params?: Record<string, string | number | undefined>): Promise<T>`
  - `mobileApiPost<T>(path: string, body?: unknown): Promise<T>`
  - `MOBILE_STALE_TIME: { usage: number; availability: number; routing: number; channels: number }`
  - `mobileQueryClient: QueryClient`
  - `<MobileProviders children />`（Task 13 在其中加入 `ThemeProvider`）

- [ ] **Step 1: 写失败测试**

`web/src/mobile/lib/__tests__/pat-store.test.ts`：

```ts
import { beforeEach, describe, expect, it } from 'vitest'

import { clearPat, isPlausiblePat, readPat, writePat } from '@/mobile/lib/pat-store'

describe('pat-store', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('returns an empty string when no token was stored', () => {
    expect(readPat()).toBe('')
  })

  it('round-trips a token and trims surrounding whitespace', () => {
    writePat('  abcdefghijklmnopqrstuvwxyz012  ')

    expect(readPat()).toBe('abcdefghijklmnopqrstuvwxyz012')
  })

  it('clears a stored token', () => {
    writePat('abcdefghijklmnopqrstuvwxyz012')
    clearPat()

    expect(readPat()).toBe('')
  })

  it('accepts the 28 and 32 character tokens the server produces', () => {
    expect(isPlausiblePat('a'.repeat(28))).toBe(true)
    expect(isPlausiblePat('a'.repeat(32))).toBe(true)
  })

  it('rejects tokens that are too short, too long, or contain spaces', () => {
    expect(isPlausiblePat('')).toBe(false)
    expect(isPlausiblePat('a'.repeat(27))).toBe(false)
    expect(isPlausiblePat('a'.repeat(33))).toBe(false)
    expect(isPlausiblePat('abcdefghijklmnopqrstuvwxy z012')).toBe(false)
  })

  it('falls back to an empty string when storage is unavailable', () => {
    const original = window.localStorage.getItem
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: {
        getItem: () => {
          throw new Error('storage disabled')
        },
      },
    })

    expect(readPat()).toBe('')
    Object.defineProperty(window, 'localStorage', { configurable: true, value: { getItem: original } })
  })
})
```

`web/src/mobile/lib/__tests__/api-client.test.ts`：

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError, mobileApiGet, mobileApiPost } from '@/mobile/lib/api-client'
import { readPat, writePat } from '@/mobile/lib/pat-store'

const VALID_PAT = 'a'.repeat(29)

function mockJsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('api-client', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.unstubAllGlobals()
  })

  it('sends the stored token as a bearer credential and unwraps data', async () => {
    writePat(VALID_PAT)
    const fetchMock = vi.fn().mockResolvedValue(mockJsonResponse({ success: true, data: { quota: 7 } }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(mobileApiGet<{ quota: number }>('/api/log/stat', { start_timestamp: 1, end_timestamp: 2, empty: undefined })).resolves.toEqual({ quota: 7 })

    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/log/stat?start_timestamp=1&end_timestamp=2')
    expect(init.headers.Authorization).toBe(`Bearer ${VALID_PAT}`)
    expect(init.method).toBe('GET')
  })

  it('posts a json body', async () => {
    writePat(VALID_PAT)
    const fetchMock = vi.fn().mockResolvedValue(mockJsonResponse({ success: true, data: true }))
    vi.stubGlobal('fetch', fetchMock)

    await mobileApiPost('/api/channel/3/status', { status: 2 })

    const [, init] = fetchMock.mock.calls[0]
    expect(init.method).toBe('POST')
    expect(init.body).toBe('{"status":2}')
    expect(init.headers['Content-Type']).toBe('application/json')
  })

  it('throws unauthorized and clears the token on 401', async () => {
    writePat(VALID_PAT)
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockJsonResponse({ success: false, message: 'invalid' }, 401)))

    await expect(mobileApiGet('/api/log/stat')).rejects.toMatchObject({ code: 'unauthorized', status: 401 })
    expect(readPat()).toBe('')
  })

  it('throws forbidden on 403 without clearing the token', async () => {
    writePat(VALID_PAT)
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockJsonResponse({ success: false, message: 'no permission' }, 403)))

    await expect(mobileApiGet('/api/channel/')).rejects.toMatchObject({ code: 'forbidden', status: 403 })
    expect(readPat()).toBe(VALID_PAT)
  })

  it('throws business with the server message when success is false', async () => {
    writePat(VALID_PAT)
    // A Response body can only be read once, so each call needs its own response.
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(() => Promise.resolve(mockJsonResponse({ success: false, message: 'time range too wide' })))
    )

    await expect(mobileApiGet('/api/data/self')).rejects.toMatchObject({ code: 'business', apiCode: undefined })
    await expect(mobileApiGet('/api/data/self')).rejects.toThrow('time range too wide')
  })

  it('throws network when fetch rejects', async () => {
    writePat(VALID_PAT)
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))

    const error = await mobileApiGet('/api/log/stat').catch((cause: unknown) => cause)
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code: 'network' })
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile/lib/__tests__`
Expected: FAIL — 无法解析 `@/mobile/lib/pat-store`。

- [ ] **Step 3: 写最小实现**

`web/src/mobile/lib/pat-store.ts`：

```ts
const PAT_STORAGE_KEY = 'newapi_mobile_pat'

// The server generates access tokens with common.GenerateRandomKey(29 + rand(4)),
// so anything outside 29..32 characters is a truncated paste.
// controller.GenerateAccessToken stores base64(29..32 requested chars), which is
// 28 or 32 characters: 29..32 requested chars become 21..24 bytes, and base64
// encodes 21 bytes as 28 characters. Roughly a quarter of issued tokens are 28
// characters long, so the floor has to be 28 or valid tokens get rejected.
const PAT_LENGTH_MIN = 28
const PAT_LENGTH_MAX = 32

export function isPlausiblePat(value: string): boolean {
  const trimmed = value.trim()
  return trimmed.length >= PAT_LENGTH_MIN && trimmed.length <= PAT_LENGTH_MAX && !/\s/.test(trimmed)
}

export function readPat(): string {
  try {
    return window.localStorage.getItem(PAT_STORAGE_KEY)?.trim() ?? ''
  } catch {
    return ''
  }
}

export function writePat(pat: string): void {
  try {
    window.localStorage.setItem(PAT_STORAGE_KEY, pat.trim())
  } catch {
    // A browser that blocks storage still works for the current tab; the
    // operator only has to paste the token again after a reload.
  }
}

export function clearPat(): void {
  try {
    window.localStorage.removeItem(PAT_STORAGE_KEY)
  } catch {
    // See writePat.
  }
}
```

`web/src/mobile/lib/api-client.ts`：

```ts
import { clearPat, readPat } from '@/mobile/lib/pat-store'

export type ApiErrorCode = 'unauthorized' | 'forbidden' | 'http' | 'network' | 'business'

export class ApiError extends Error {
  readonly code: ApiErrorCode
  readonly status: number
  readonly apiCode?: string

  constructor(code: ApiErrorCode, message: string, status: number, apiCode?: string) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.apiCode = apiCode
  }
}

interface ApiEnvelope<T> {
  success: boolean
  message?: string
  code?: string
  data?: T
}

function buildUrl(path: string, params?: Record<string, string | number | undefined>): string {
  if (!params) {
    return path
  }
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined) {
      continue
    }
    query.set(key, String(value))
  }
  const serialized = query.toString()
  return serialized === '' ? path : `${path}?${serialized}`
}

async function request<T>(path: string, init: RequestInit, params?: Record<string, string | number | undefined>): Promise<T> {
  const token = readPat()
  let response: Response
  try {
    response = await fetch(buildUrl(path, params), {
      ...init,
      credentials: 'omit',
      headers: {
        Accept: 'application/json',
        ...(token === '' ? {} : { Authorization: `Bearer ${token}` }),
        ...init.headers,
      },
    })
  } catch {
    throw new ApiError('network', 'Network request failed', 0)
  }

  if (response.status === 401) {
    clearPat()
    throw new ApiError('unauthorized', 'Access token rejected', 401)
  }
  if (response.status === 403) {
    throw new ApiError('forbidden', 'Access token has no permission for this action', 403)
  }

  let envelope: ApiEnvelope<T> | null = null
  try {
    envelope = (await response.json()) as ApiEnvelope<T>
  } catch {
    envelope = null
  }

  if (!response.ok) {
    throw new ApiError('http', envelope?.message ?? `Request failed with status ${response.status}`, response.status, envelope?.code)
  }
  if (!envelope || envelope.success !== true) {
    throw new ApiError('business', envelope?.message ?? 'Request failed', response.status, envelope?.code)
  }
  return envelope.data as T
}

export function mobileApiGet<T>(path: string, params?: Record<string, string | number | undefined>): Promise<T> {
  return request<T>(path, { method: 'GET' }, params)
}

export function mobileApiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}
```

`web/src/mobile/lib/query-client.ts`：

```ts
import { QueryClient } from '@tanstack/react-query'

import { ApiError } from '@/mobile/lib/api-client'

// Aggregated usage comes from quota_data, which the export task refreshes on
// DataExportInterval (5 minutes by default); live rate figures are cheap to
// refetch, and channel state is what the operator acts on.
export const MOBILE_STALE_TIME = {
  usage: 60_000,
  availability: 60_000,
  routing: 120_000,
  channels: 30_000,
} as const

export const mobileQueryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: (failureCount, error) => {
        if (error instanceof ApiError && error.code !== 'network') {
          return false
        }
        return failureCount < 2
      },
    },
  },
})
```

`web/src/mobile/providers.tsx`（Task 13 会在这里加入 `ThemeProvider`）：

```tsx
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

import { mobileQueryClient } from '@/mobile/lib/query-client'

interface MobileProvidersProps {
  children: ReactNode
}

export function MobileProviders(props: MobileProvidersProps) {
  return <QueryClientProvider client={mobileQueryClient}>{props.children}</QueryClientProvider>
}
```

`web/src/mobile/main.tsx` 改用该 Provider（取代 Task 3 里直接包裹的 `ThemeProvider`）：

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Toaster } from '@/components/ui/sonner'
import '@/mobile/styles/mobile.css'

import { MobileApp } from '@/mobile/app'
import { initializeMobileI18n } from '@/mobile/lib/i18n'
import { MobileProviders } from '@/mobile/providers'

const container = document.getElementById('root')
if (container) {
  // Not awaited: the shell renders with the English source keys first and
  // re-renders once the active locale chunk arrives.
  void initializeMobileI18n()

  createRoot(container).render(
    <StrictMode>
      <MobileProviders>
        <MobileApp />
        <Toaster />
      </MobileProviders>
    </StrictMode>
  )
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web
bun run test -- src/mobile/lib/__tests__
bun run typecheck && bunx oxlint -c .oxlintrc.json src/mobile
```
Expected: 全部 PASS，typecheck/lint 无 error。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add web/src/mobile/lib web/src/mobile/main.tsx web/src/mobile/providers.tsx
git commit -m "feat(web-mobile): authenticate the mobile console with a personal access token"
```

---

## Task 6：PAT 门与展示基元

**Files:**
- Create: `web/src/mobile/components/pat-gate.tsx`、`web/src/mobile/components/kpi-card.tsx`、`web/src/mobile/components/value-row.tsx`、`web/src/mobile/components/mobile-loading.tsx`、`web/src/mobile/components/mobile-empty.tsx`、`web/src/mobile/components/mobile-error.tsx`
- Create: `web/src/mobile/components/__tests__/pat-gate.test.tsx`、`web/src/mobile/components/__tests__/kpi-card.test.tsx`、`web/src/mobile/components/__tests__/mobile-states.test.tsx`
- Create: `web/src/mobile/lib/status.ts`、`web/src/mobile/lib/__tests__/status.test.ts`
- Create: `web/src/lib/status-config.ts`（把 `web/src/lib/status-query.ts` 里与 axios 无关的纯逻辑搬出来）
- Modify: `web/src/mobile/app.tsx`、`web/src/mobile/lib/pat-store.ts`、`web/src/lib/status-query.ts`

**Interfaces:**
- Consumes: `readPat` / `writePat` / `isPlausiblePat`（Task 5）；`@/lib/format` 的 `formatNumber`。
- Produces:
  - `<PatGate onReady={(pat: string) => void} children />`——无有效 PAT 时渲染输入页，否则渲染 children
  - `<KpiCard label value hint />`
  - `<ValueRow label value hint />`

- [ ] **Step 1: 写失败测试**

`web/src/mobile/components/__tests__/pat-gate.test.tsx`：

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { PatGate } from '@/mobile/components/pat-gate'
import { readPat } from '@/mobile/lib/pat-store'

describe('PatGate', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('asks for a token when none is stored', () => {
    render(
      <PatGate onReady={vi.fn()}>
        <p>console</p>
      </PatGate>
    )

    expect(screen.getByLabelText('Access token')).toBeInTheDocument()
    expect(screen.queryByText('console')).not.toBeInTheDocument()
  })

  it('rejects a token that is too short without storing it', async () => {
    const user = userEvent.setup()
    render(
      <PatGate onReady={vi.fn()}>
        <p>console</p>
      </PatGate>
    )

    await user.type(screen.getByLabelText('Access token'), 'short')
    await user.click(screen.getByRole('button', { name: 'Verify and continue' }))

    expect(screen.getByText('The token looks too short. Check that you copied all of it.')).toBeInTheDocument()
    expect(readPat()).toBe('')
  })

  it('stores a plausible token and reveals the console', async () => {
    const user = userEvent.setup()
    const onReady = vi.fn()
    render(
      <PatGate onReady={onReady}>
        <p>console</p>
      </PatGate>
    )

    await user.type(screen.getByLabelText('Access token'), 'a'.repeat(29))
    await user.click(screen.getByRole('button', { name: 'Verify and continue' }))

    expect(readPat()).toBe('a'.repeat(29))
    expect(onReady).toHaveBeenCalledWith('a'.repeat(29))
  })

  it('skips the gate when a token is already stored', () => {
    window.localStorage.setItem('newapi_mobile_pat', 'b'.repeat(29))

    render(
      <PatGate onReady={vi.fn()}>
        <p>console</p>
      </PatGate>
    )

    expect(screen.getByText('console')).toBeInTheDocument()
  })
})
```

`web/src/mobile/components/__tests__/kpi-card.test.tsx`：

```tsx
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { KpiCard } from '@/mobile/components/kpi-card'

describe('KpiCard', () => {
  it('shows the label, the formatted value and the hint', () => {
    render(<KpiCard label='Requests' value={12345} hint='last 7 days' />)

    expect(screen.getByText('Requests')).toBeInTheDocument()
    expect(screen.getByText('12,345')).toBeInTheDocument()
    expect(screen.getByText('last 7 days')).toBeInTheDocument()
  })

  it('accepts a pre-formatted string value', () => {
    render(<KpiCard label='Cost' value='$1.23' />)

    expect(screen.getByText('$1.23')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile/components/__tests__`
Expected: FAIL — 模块不存在。

- [ ] **Step 3: 写最小实现**

`web/src/mobile/components/kpi-card.tsx`：用 `@/components/ui/card` + `@/lib/format`：

```tsx
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'
import { formatNumber } from '@/lib/format'
import { toIntlLocale } from '@/i18n/languages'

interface KpiCardProps {
  label: string
  value: number | string
  hint?: string
}

export function KpiCard(props: KpiCardProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const displayValue = typeof props.value === 'number' ? formatNumber(props.value, locale) : props.value

  return (
    <Card className='gap-1 py-3'>
      <CardContent className='px-3'>
        <p className='text-muted-foreground text-xs'>{t(props.label)}</p>
        <p className='text-lg font-semibold tabular-nums'>{displayValue}</p>
        {props.hint ? <p className='text-muted-foreground text-[11px]'>{t(props.hint)}</p> : null}
      </CardContent>
    </Card>
  )
}
```

`web/src/mobile/components/value-row.tsx`：

```tsx
interface ValueRowProps {
  label: string
  value: string
  secondary?: string
}

export function ValueRow(props: ValueRowProps) {
  return (
    <div className='flex min-h-11 items-center justify-between gap-3 px-3 py-2'>
      <div className='min-w-0'>
        <p className='truncate text-sm'>{props.label}</p>
        {props.secondary ? <p className='text-muted-foreground truncate text-xs'>{props.secondary}</p> : null}
      </div>
      <p className='shrink-0 text-sm font-medium tabular-nums'>{props.value}</p>
    </div>
  )
}
```

`web/src/mobile/components/pat-gate.tsx`：使用 `@/components/ui/{button,input,label}`：

```tsx
import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { isPlausiblePat, readPat, writePat } from '@/mobile/lib/pat-store'

interface PatGateProps {
  onReady: (pat: string) => void
  children: ReactNode
}

export function PatGate(props: PatGateProps) {
  const { t } = useTranslation()
  const [storedPat, setStoredPat] = useState(() => readPat())
  const [draft, setDraft] = useState('')
  const [errorKey, setErrorKey] = useState<string | null>(null)

  if (storedPat !== '') {
    return <>{props.children}</>
  }

  const submit = () => {
    if (!isPlausiblePat(draft)) {
      setErrorKey('The token looks too short. Check that you copied all of it.')
      return
    }
    const token = draft.trim()
    writePat(token)
    setErrorKey(null)
    setStoredPat(token)
    props.onReady(token)
  }

  return (
    <main className='mx-auto flex min-h-dvh max-w-md flex-col justify-center gap-4 px-5 pb-[max(1rem,env(safe-area-inset-bottom))]'>
      <div className='space-y-1'>
        <h1 className='text-lg font-semibold'>{t('Access token required')}</h1>
        <p className='text-muted-foreground text-sm'>{t('Paste the access token you generated on the desktop console.')}</p>
      </div>
      <div className='space-y-2'>
        <Label htmlFor='mobile-pat'>{t('Access token')}</Label>
        <Input
          id='mobile-pat'
          autoComplete='off'
          inputMode='text'
          spellCheck={false}
          type='password'
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          aria-invalid={errorKey !== null}
          aria-describedby={errorKey ? 'mobile-pat-error' : undefined}
        />
        {errorKey ? (
          <p id='mobile-pat-error' role='alert' className='text-destructive text-xs'>
            {t(errorKey)}
          </p>
        ) : null}
      </div>
      <Button className='h-11' onClick={submit}>
        {t('Verify and continue')}
      </Button>
    </main>
  )
}
```

`web/src/mobile/app.tsx` 改为用 `PatGate` 包裹：

```tsx
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { MobileLoading } from '@/mobile/components/mobile-loading'
import { PatGate } from '@/mobile/components/pat-gate'
import { mobileStatusQueryOptions } from '@/mobile/lib/status'

export function MobileApp() {
  const { t } = useTranslation()
  // formatQuotaWithCurrency and the other money helpers read the currency
  // settings from the system-config store, which /api/status hydrates. Without
  // this query every amount would be rendered with the USD defaults.
  const status = useQuery(mobileStatusQueryOptions)

  if (status.isPending) {
    return <MobileLoading />
  }

  return (
    <PatGate onReady={() => {}}>
      <main className='min-h-dvh bg-background text-foreground'>
        <header className='px-4 pt-[max(1rem,env(safe-area-inset-top))] pb-2'>
          <h1 className='text-lg font-semibold'>{t('Mobile console')}</h1>
        </header>
      </main>
    </PatGate>
  )
}
```

注意：Task 3 的 `app.test.tsx` 此时会失败两处——未存 PAT 时看不到 heading，且 `useQuery` 需要 QueryClientProvider。在 Step 4 之前把它改为：

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'

import { MobileApp } from '@/mobile/app'
import { STATUS_QUERY_KEY } from '@/mobile/lib/status'

const testQueryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

function renderApp() {
  return render(
    <QueryClientProvider client={testQueryClient}>
      <MobileApp />
    </QueryClientProvider>
  )
}

describe('MobileApp', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    testQueryClient.clear()
    // Seeding the shared status query keeps the shell from calling /api/status
    // while the currency helpers fall back to their documented USD defaults.
    testQueryClient.setQueryData(STATUS_QUERY_KEY, {})
  })

  it('renders the mobile console heading', () => {
    renderApp()

    expect(screen.getByRole('heading', { name: 'Mobile console' })).toBeInTheDocument()
  })
})
```

- [ ] **Step 3b: 手机端展示基元与状态查询（不得复用桌面那三个状态组件）**

桌面 `@/components/{loading-state,error-state,empty-state}.tsx` 会经 `@/components/page-transition.tsx` 传递性引入 `@tanstack/react-router` 与 `motion/react`，`@/lib/status-query.ts` 会经 `@/lib/api` → `@/lib/http-client` 引入 `axios`，三者还都带 `lucide-react`。它们全在手机端禁用清单里：实测静态引入后首屏 JS gzip 从 96,508 B 涨到 137,898 B（超预算 15 KB）。所以手机端要自己的等价物（**props 与桌面同名，Task 8–11 直接替换即可**）：

1. `web/src/mobile/components/mobile-loading.tsx` 导出 `MobileLoading({ className }: { className?: string })`：外层 `role='status'` + `aria-live='polite'`，一个纯 Tailwind 的旋转圆环（`size-5 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-foreground`），`sr-only` 文案 `t('Loading...')`；**不使用 `lucide-react`**。
2. `web/src/mobile/components/mobile-empty.tsx` 导出 `MobileEmpty({ title, description, className })`：居中标题（`text-sm font-medium`）与说明（`text-xs text-muted-foreground`），**不渲染图标、不接受 icon 参数**。
3. `web/src/mobile/components/mobile-error.tsx` 导出 `MobileError({ title, description, onRetry, className })`：标题 + 说明；传 `onRetry` 时额外渲染 `@/components/ui/button` 的 `<Button className='mt-3 h-10' onClick={onRetry}>{t('Retry')}</Button>`，不传则不渲染按钮。
4. `web/src/lib/status-config.ts`（新文件，纯搬迁）：把 `status-query.ts` 中与 axios 无关的部分搬过来——`StatusData`、`mapStatusDataToConfig`、`STATUS_QUERY_KEY`、`STATUS_STORAGE_KEY`、`readCachedStatus`、`writeCachedStatus`，再加 `syncStatusToSystemConfig(status: StatusData): void`（`useSystemConfigStore.getState().setConfig(mapStatusDataToConfig(status))` + `writeCachedStatus(status)`，保留原实现里的 try/catch 与 `import.meta.env.DEV` 日志）。该文件只允许 import `@tanstack/react-query`、`@/lib/constants`、`@/stores/system-config-store`。
   `web/src/lib/status-query.ts` 改为从 `status-config.ts` import 这些名字并**原样 re-export**（含 `export type { StatusData }`），保证既有消费者（`@/hooks/*`、`@/lib/nav-modules`、`@/main.tsx` 等）与 `src/lib/__tests__/status-query.test.tsx` 不改一行仍通过；内部 `fetchStatus` 改为调用 `syncStatusToSystemConfig`。这是纯搬迁，桌面行为必须零变化。
5. `web/src/mobile/lib/status.ts`：`export const mobileStatusQueryOptions = queryOptions({ queryKey: STATUS_QUERY_KEY, queryFn: async (): Promise<StatusData> => { const status = await mobileApiGet<StatusData>('/api/status'); syncStatusToSystemConfig(status); return status }, staleTime: 5 * 60 * 1000, gcTime: 30 * 60 * 1000 })`，并 re-export `STATUS_QUERY_KEY`。`/api/status` 无鉴权中间件，而 `mobileApiGet` 在未存 PAT 时不会带 `Authorization`，所以它在 PAT 门之前也能成功。

先写失败测试，再写实现：

- `web/src/mobile/components/__tests__/mobile-states.test.tsx`：`MobileLoading` 有 `role='status'`；`MobileEmpty` 渲染 `title` 与 `description` 且容器内**没有任何 `svg`**（`container.querySelector('svg')` 为 null）；`MobileError` 传 `onRetry` 时点击 `t('Retry')` 按钮会调用它、不传时没有任何按钮。
- `web/src/mobile/lib/__tests__/status.test.ts`：`vi.stubGlobal('fetch', ...)` 返回 `{ success: true, data: { quota_per_unit: 500000, quota_display_type: 'USD' } }`；断言 `mobileStatusQueryOptions.queryFn`（用 `await mobileStatusQueryOptions.queryFn({} as never)` 或经 `QueryClient.fetchQuery` 调用）请求的是 `/api/status`、把 `quotaPerUnit` 写进了 `useSystemConfigStore.getState().currency.quotaPerUnit`、返回 `data`；并断言该请求的 `Authorization` 头**不存在**（未存 PAT 时不得带）。
- 回归：`web/src/lib/__tests__/status-query.test.tsx` 必须仍然全绿。

- [ ] **Step 3c: PAT 失效时门必须重新出现（订阅 pat-store）**

`PatGate` 现在的实现只在挂载时读一次 `readPat()`，而 `api-client` 在任何 401 上都会 `clearPat()`。计划对 `pat-gate.tsx` 的职责是「未配置 PAT / **令牌失效**时的门」，所以令牌被吊销或过期后手机端必须自动回到输入页，而不是把用户锁在一堆错误页里直到手动刷新（Task 8 接入受保护接口后这是必然路径）。

实现方式（订阅式，不加全局状态库）：

1. `web/src/mobile/lib/pat-store.ts` 增加一个极小的订阅面：
   - `const patListeners = new Set<() => void>()`
   - `export function subscribePat(listener: () => void): () => void`（加入集合并返回 unsubscribe）
   - `writePat` 与 `clearPat` 在写入成功后通知所有 listener（`readPat` 保持不变，仍是同步读取当前值——`useSyncExternalStore` 的 `getSnapshot` 就是它）。
2. `web/src/mobile/components/pat-gate.tsx` 把 `const [storedPat] = useState(() => readPat())` 改为 `const storedPat = useSyncExternalStore(subscribePat, readPat)`，其余交互（`onReady`、错误提示、ARIA）不变。这样 `clearPat()` 会触发重渲染并把输入页放回去。

先写失败测试，再写实现：

- `web/src/mobile/lib/__tests__/pat-store.test.ts` 追加：`subscribePat` 在 `writePat` 与 `clearPat` 后各被通知一次；调用返回的 unsubscribe 之后不再收到通知。
- `web/src/mobile/components/__tests__/pat-gate.test.tsx` 追加：存好一个合法 PAT 渲染 `PatGate`（children 可见）→ 调用 `clearPat()` → 断言 `t('Access token')` 输入页重新出现、children 消失。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile src/lib/__tests__/status-query.test.tsx && bun run typecheck`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add web/src/mobile/components web/src/mobile/lib web/src/mobile/app.tsx web/src/mobile/__tests__/app.test.tsx web/src/lib/status-config.ts web/src/lib/status-query.ts
git commit -m "feat(web-mobile): gate the console behind a personal access token"
```

---

## Task 7：hash 路由与底部 Tab 栏

**Files:**
- Create: `web/src/mobile/lib/router.ts`、`web/src/mobile/components/mobile-tab-bar.tsx`
- Create: `web/src/mobile/lib/__tests__/router.test.ts`、`web/src/mobile/components/__tests__/mobile-tab-bar.test.tsx`
- Modify: `web/src/mobile/app.tsx`

**Interfaces:**
- Produces:
  - `MOBILE_TABS: readonly MobileTab[]` = `['usage', 'models', 'routing', 'channels']`
  - `parseTab(hash: string): MobileTab`、`tabHash(tab: MobileTab): string`、`useActiveTab(): [MobileTab, (tab: MobileTab) => void]`
  - `<MobileTabBar active onChange />`，每个页签是 `role='tab'`，容器是 `role='tablist'`，`aria-selected` 与激活状态一致

- [ ] **Step 1: 写失败测试**

`web/src/mobile/lib/__tests__/router.test.ts`：

```ts
import { describe, expect, it } from 'vitest'

import { parseTab, tabHash } from '@/mobile/lib/router'

describe('parseTab', () => {
  it('returns usage for an empty hash', () => {
    expect(parseTab('')).toBe('usage')
  })

  it('parses each known tab', () => {
    expect(parseTab('#/models')).toBe('models')
    expect(parseTab('#/routing')).toBe('routing')
    expect(parseTab('#/channels')).toBe('channels')
  })

  it('falls back to usage for an unknown hash', () => {
    expect(parseTab('#/nope')).toBe('usage')
    expect(parseTab('#/usage/extra')).toBe('usage')
  })

  it('ignores a trailing slash', () => {
    expect(parseTab('#/routing/')).toBe('routing')
  })
})

describe('tabHash', () => {
  it('builds the hash for a tab', () => {
    expect(tabHash('channels')).toBe('#/channels')
    expect(tabHash('usage')).toBe('#/usage')
  })
})
```

`web/src/mobile/components/__tests__/mobile-tab-bar.test.tsx`：

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { MobileTabBar } from '@/mobile/components/mobile-tab-bar'

describe('MobileTabBar', () => {
  it('marks the active tab as selected', () => {
    render(<MobileTabBar active='routing' onChange={vi.fn()} />)

    expect(screen.getByRole('tab', { name: 'Routing statistics' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'Usage' })).toHaveAttribute('aria-selected', 'false')
  })

  it('reports the tapped tab', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<MobileTabBar active='usage' onChange={onChange} />)

    await user.click(screen.getByRole('tab', { name: 'Channels' }))

    expect(onChange).toHaveBeenCalledWith('channels')
  })

  it('exposes four tabs inside a tablist', () => {
    render(<MobileTabBar active='usage' onChange={vi.fn()} />)

    expect(screen.getByRole('tablist')).toBeInTheDocument()
    expect(screen.getAllByRole('tab')).toHaveLength(4)
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

`useActiveTab` 的三条主干行为也必须进 `router.test.ts`（`renderHook` + `act`，这是本任务真正的回归网——Task 8–11 会反复改写 `app.tsx`）：

1. 深链接初始化：`window.location.hash = '#/channels'` 后 `renderHook(() => useActiveTab())`，`result.current[0]` 必须是 `'channels'`。
2. `hashchange` 同步：hook 挂载后把 `window.location.hash` 设为 `'#/routing'` 并 `window.dispatchEvent(new HashChangeEvent('hashchange'))`，`act` 之后 `result.current[0]` 必须变成 `'routing'`。
3. 卸载清理：`unmount()` 之后 `hashchange` 不得再更新状态（用 `vi.spyOn(window, 'removeEventListener')` 断言 `'hashchange'` 监听被移除，或断言卸载后再派发事件不抛错且不更新）。

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile/lib/__tests__/router.test.ts src/mobile/components/__tests__/mobile-tab-bar.test.tsx`
Expected: FAIL — 模块不存在。

- [ ] **Step 3: 写最小实现**

`web/src/mobile/lib/router.ts`：

```ts
import { useCallback, useEffect, useState } from 'react'

import type { MobileTab } from '@/mobile/types'

export const MOBILE_TABS: readonly MobileTab[] = ['usage', 'models', 'routing', 'channels']

export function parseTab(hash: string): MobileTab {
  const normalized = hash.replace(/^#\/?/, '').replace(/\/+$/, '')
  const candidate = MOBILE_TABS.find((tab) => tab === normalized)
  return candidate ?? 'usage'
}

export function tabHash(tab: MobileTab): string {
  return `#/${tab}`
}

export function useActiveTab(): [MobileTab, (tab: MobileTab) => void] {
  const [active, setActive] = useState<MobileTab>(() => parseTab(window.location.hash))

  useEffect(() => {
    const onHashChange = () => setActive(parseTab(window.location.hash))
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [])

  const select = useCallback((tab: MobileTab) => {
    window.location.hash = tabHash(tab)
  }, [])

  return [active, select]
}
```

`web/src/mobile/components/mobile-tab-bar.tsx`：图标用内联 SVG（不引入图标库）：

```tsx
import { useTranslation } from 'react-i18next'

import { MOBILE_TABS } from '@/mobile/lib/router'
import type { MobileTab } from '@/mobile/types'
import { cn } from '@/lib/utils'

const TAB_LABEL_KEY: Record<MobileTab, string> = {
  usage: 'Usage',
  models: 'Model availability',
  routing: 'Routing statistics',
  channels: 'Channels',
}

interface MobileTabBarProps {
  active: MobileTab
  onChange: (tab: MobileTab) => void
}

export function MobileTabBar(props: MobileTabBarProps) {
  const { t } = useTranslation()

  return (
    <nav
      role='tablist'
      aria-label={t('Mobile console')}
      className='bg-background/95 fixed inset-x-0 bottom-0 flex border-t pb-[env(safe-area-inset-bottom)] backdrop-blur'
    >
      {MOBILE_TABS.map((tab) => {
        const selected = tab === props.active
        return (
          <button
            key={tab}
            type='button'
            role='tab'
            aria-selected={selected}
            className={cn(
              'flex min-h-12 flex-1 flex-col items-center justify-center gap-0.5 text-[11px]',
              selected ? 'text-foreground font-medium' : 'text-muted-foreground'
            )}
            onClick={() => props.onChange(tab)}
          >
            {t(TAB_LABEL_KEY[tab])}
          </button>
        )
      })}
    </nav>
  )
}
```

`web/src/mobile/app.tsx` 接入路由与页签（四个页签先用占位组件，Task 8–11 逐个替换）：

```tsx
import { useTranslation } from 'react-i18next'

import { MobileTabBar } from '@/mobile/components/mobile-tab-bar'
import { PatGate } from '@/mobile/components/pat-gate'
import { useActiveTab } from '@/mobile/lib/router'

export function MobileApp() {
  const { t } = useTranslation()
  const [activeTab, selectTab] = useActiveTab()

  return (
    <PatGate onReady={() => {}}>
      <main className='min-h-dvh bg-background pb-16 text-foreground'>
        <header className='px-4 pt-[max(1rem,env(safe-area-inset-top))] pb-2'>
          <h1 className='text-lg font-semibold'>{t('Mobile console')}</h1>
        </header>
        <section data-testid='mobile-panel' data-tab={activeTab} />
      </main>
      <MobileTabBar active={activeTab} onChange={selectTab} />
    </PatGate>
  )
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile && bun run typecheck && bunx oxlint -c .oxlintrc.json src/mobile`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add web/src/mobile/lib/router.ts web/src/mobile/lib/__tests__/router.test.ts web/src/mobile/components/mobile-tab-bar.tsx web/src/mobile/components/__tests__/mobile-tab-bar.test.tsx web/src/mobile/app.tsx
git commit -m "feat(web-mobile): navigate the console with a four-tab hash router"
```

---

## Task 8：用量页

**Files:**
- Modify: `web/src/mobile/types.ts`
- Create: `web/src/mobile/features/usage/lib/usage-summary.ts`、`web/src/mobile/features/usage/api.ts`、`web/src/mobile/features/usage/components/usage-page.tsx`、`web/src/mobile/features/usage/components/recent-requests.tsx`
- Create: `web/src/mobile/features/usage/lib/__tests__/usage-summary.test.ts`、`web/src/mobile/features/usage/components/__tests__/usage-page.test.tsx`
- Modify: `web/src/mobile/app.tsx`

**Interfaces:**
- Consumes: `mobileApiGet`（Task 5）、`MOBILE_STALE_TIME`、`KpiCard`、`ValueRow`、`MobileEmpty`、`MobileLoading`、`MobileError`。
- Produces:
  - `resolveTimeRange(preset: TimeRangePreset, now: number): TimeRange`
  - `aggregateTotals(rows: QuotaDataRow[]): UsageTotals`
  - `rankRows(rows: QuotaDataRow[], key: 'model_name' | 'username', limit: number): RankRow[]`
  - `useUsageTotals`、`useUsageRanking`、`useLiveRate`、`useRecentLogs`
  - `<UsagePage />`

- [ ] **Step 1: 写失败测试**

`web/src/mobile/features/usage/lib/__tests__/usage-summary.test.ts`：

```ts
import { describe, expect, it } from 'vitest'

import { aggregateTotals, rankRows, resolveTimeRange } from '@/mobile/features/usage/lib/usage-summary'
import type { QuotaDataRow } from '@/mobile/types'

const rows: QuotaDataRow[] = [
  { model_name: 'gpt-5', created_at: 1, count: 10, quota: 500, token_used: 1000 },
  { model_name: 'gpt-5', created_at: 2, count: 5, quota: 250, token_used: 500 },
  { model_name: 'claude-5', created_at: 1, count: 1, quota: 900, token_used: 30 },
]

describe('resolveTimeRange', () => {
  // 2026-10-05T12:00:00Z
  const now = 1775476800

  it('starts today at the local midnight of the current day', () => {
    const range = resolveTimeRange('today', now)

    expect(range.end).toBe(now)
    expect(new Date(range.start * 1000).getHours()).toBe(0)
    expect(range.start).toBeLessThanOrEqual(now)
  })

  it('covers seven days for the 7d preset', () => {
    const range = resolveTimeRange('7d', now)

    expect(range.end - range.start).toBe(7 * 24 * 60 * 60)
  })

  it('covers thirty days for the 30d preset', () => {
    const range = resolveTimeRange('30d', now)

    expect(range.end - range.start).toBe(30 * 24 * 60 * 60)
  })
})

describe('aggregateTotals', () => {
  it('sums requests, tokens and quota', () => {
    expect(aggregateTotals(rows)).toEqual({ requests: 16, tokens: 1530, quota: 1650 })
  })

  it('returns zeros for an empty range', () => {
    expect(aggregateTotals([])).toEqual({ requests: 0, tokens: 0, quota: 0 })
  })
})

describe('rankRows', () => {
  it('groups by model, sums, sorts by quota and truncates', () => {
    expect(rankRows(rows, 'model_name', 2)).toEqual([
      { key: 'claude-5', requests: 1, tokens: 30, quota: 900 },
      { key: 'gpt-5', requests: 15, tokens: 1500, quota: 750 },
    ])
  })

  it('returns an empty list when there are no rows', () => {
    expect(rankRows([], 'model_name', 5)).toEqual([])
  })

  it('keeps a single row unchanged', () => {
    expect(rankRows([rows[2]], 'model_name', 5)).toEqual([
      { key: 'claude-5', requests: 1, tokens: 30, quota: 900 },
    ])
  })
})
```

`web/src/mobile/features/usage/components/__tests__/usage-page.test.tsx`：

```tsx
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { UsagePage } from '@/mobile/features/usage/components/usage-page'
import { mobileQueryClient } from '@/mobile/lib/query-client'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

function renderPage() {
  render(
    <QueryClientProvider client={mobileQueryClient}>
      <UsagePage />
    </QueryClientProvider>
  )
}

describe('UsagePage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    mobileQueryClient.clear()
    vi.unstubAllGlobals()
  })

  it('shows totals from the quota data aggregate', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.startsWith('/api/log/self/stat')) {
          return jsonResponse({ success: true, data: { quota: 1650, rpm: 3, tpm: 400 } })
        }
        if (url.startsWith('/api/data/self')) {
          return jsonResponse({
            success: true,
            data: [{ model_name: 'gpt-5', created_at: 1, count: 16, quota: 1650, token_used: 1530 }],
          })
        }
        return jsonResponse({ success: true, data: { items: [], total: 0 } })
      })
    )

    renderPage()

    await waitFor(() => expect(screen.getByText('16')).toBeInTheDocument())
    // formatTokens shortens thousands, and both the KPI card and the model row
    // render 1530 tokens, so more than one matching node is expected.
    expect(screen.getAllByText('1.5K').length).toBeGreaterThan(0)
    expect(screen.getByText('gpt-5')).toBeInTheDocument()
    expect(screen.getByText('Summary data updates every few minutes.')).toBeInTheDocument()
  })

  it('switches the scope to the whole instance', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ success: true, data: { quota: 0, rpm: 0, tpm: 0 } }))
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    renderPage()
    await user.click(screen.getByRole('button', { name: 'All' }))

    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).startsWith('/api/log/stat'))).toBe(true))
  })

  it('shows the empty state when the range has no data', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ success: true, data: [] })))

    renderPage()

    await waitFor(() => expect(screen.getByText('Nothing to show for this range.')).toBeInTheDocument())
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile/features/usage`
Expected: FAIL — 模块不存在。

- [ ] **Step 3: 写最小实现**

`web/src/mobile/types.ts` 追加（保留 Task 3 已定义的 `MobileTab`）：

```ts
export type UsageScope = 'self' | 'all'

export type TimeRangePreset = 'today' | '7d' | '30d'

export interface TimeRange {
  start: number
  end: number
}

export interface UsageTotals {
  requests: number
  tokens: number
  quota: number
}

export interface RankRow {
  key: string
  requests: number
  tokens: number
  quota: number
}

export interface QuotaDataRow {
  model_name?: string
  username?: string
  created_at: number
  count: number
  quota: number
  token_used: number
}

export interface LogStat {
  quota: number
  rpm: number
  tpm: number
}

export interface UsageLogRow {
  id: number
  created_at: number
  model_name: string
  token_name?: string
  prompt_tokens: number
  completion_tokens: number
  quota: number
  use_time: number
  is_stream: boolean
  type: number
}

export interface PagedResult<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}
```

`web/src/mobile/features/usage/lib/usage-summary.ts`（用 `@/lib/dayjs`）：

```ts
import dayjs from '@/lib/dayjs'
import type { QuotaDataRow, RankRow, TimeRange, TimeRangePreset, UsageTotals } from '@/mobile/types'

const PRESET_DAYS: Record<Exclude<TimeRangePreset, 'today'>, number> = {
  '7d': 7,
  '30d': 30,
}

export function resolveTimeRange(preset: TimeRangePreset, now: number): TimeRange {
  if (preset === 'today') {
    return { start: dayjs.unix(now).startOf('day').unix(), end: now }
  }
  return { start: now - PRESET_DAYS[preset] * 24 * 60 * 60, end: now }
}

export function aggregateTotals(rows: QuotaDataRow[]): UsageTotals {
  return rows.reduce<UsageTotals>(
    (totals, row) => ({
      requests: totals.requests + row.count,
      tokens: totals.tokens + row.token_used,
      quota: totals.quota + row.quota,
    }),
    { requests: 0, tokens: 0, quota: 0 }
  )
}

export function rankRows(rows: QuotaDataRow[], key: 'model_name' | 'username', limit: number): RankRow[] {
  const grouped = new Map<string, RankRow>()
  for (const row of rows) {
    const name = (key === 'model_name' ? row.model_name : row.username) ?? ''
    if (name === '') {
      continue
    }
    const current = grouped.get(name) ?? { key: name, requests: 0, tokens: 0, quota: 0 }
    current.requests += row.count
    current.tokens += row.token_used
    current.quota += row.quota
    grouped.set(name, current)
  }
  return [...grouped.values()].sort((left, right) => right.quota - left.quota).slice(0, limit)
}
```

`web/src/mobile/features/usage/api.ts`：

```ts
import { keepPreviousData, useQuery } from '@tanstack/react-query'

import { mobileApiGet } from '@/mobile/lib/api-client'
import { MOBILE_STALE_TIME } from '@/mobile/lib/query-client'
import type { LogStat, PagedResult, QuotaDataRow, TimeRange, UsageLogRow, UsageScope } from '@/mobile/types'

export function useLiveRate(scope: UsageScope, range: TimeRange) {
  return useQuery({
    queryKey: ['mobile', 'usage', 'stat', scope, range.start, range.end],
    queryFn: () =>
      mobileApiGet<LogStat>(scope === 'all' ? '/api/log/stat' : '/api/log/self/stat', {
        start_timestamp: range.start,
        end_timestamp: range.end,
      }),
    staleTime: MOBILE_STALE_TIME.usage,
  })
}

export function useUsageAggregate(scope: UsageScope, range: TimeRange) {
  return useQuery({
    queryKey: ['mobile', 'usage', 'aggregate', scope, range.start, range.end],
    queryFn: () =>
      mobileApiGet<QuotaDataRow[]>(scope === 'all' ? '/api/data/' : '/api/data/self', {
        start_timestamp: range.start,
        end_timestamp: range.end,
      }),
    staleTime: MOBILE_STALE_TIME.usage,
    placeholderData: keepPreviousData,
  })
}

export function useUsageRanking(scope: UsageScope, range: TimeRange, enabled: boolean) {
  return useQuery({
    queryKey: ['mobile', 'usage', 'ranking', range.start, range.end],
    queryFn: () => mobileApiGet<QuotaDataRow[]>('/api/data/users', { start_timestamp: range.start, end_timestamp: range.end }),
    staleTime: MOBILE_STALE_TIME.usage,
    placeholderData: keepPreviousData,
    enabled: scope === 'all' && enabled,
  })
}

export function useRecentLogs(scope: UsageScope, range: TimeRange, type: number) {
  return useQuery({
    queryKey: ['mobile', 'usage', 'logs', scope, range.start, range.end, type],
    queryFn: () =>
      mobileApiGet<PagedResult<UsageLogRow>>(scope === 'all' ? '/api/log' : '/api/log/self', {
        p: 1,
        page_size: 20,
        type,
        start_timestamp: range.start,
        end_timestamp: range.end,
      }),
    staleTime: MOBILE_STALE_TIME.usage,
  })
}
```

`web/src/mobile/features/usage/components/recent-requests.tsx`：列表用 `ValueRow` + `@/lib/format` 的 `formatTokens` / `formatUseTime` / `formatLogQuota` / `formatTimestampRelative`：

```tsx
import { useTranslation } from 'react-i18next'

import { MobileEmpty } from '@/mobile/components/mobile-empty'
import { formatLogQuota, formatTimestampRelative, formatTokens, formatUseTime } from '@/lib/format'
import { ValueRow } from '@/mobile/components/value-row'
import { useRecentLogs } from '@/mobile/features/usage/api'
import type { TimeRange, UsageScope } from '@/mobile/types'

interface RecentRequestsProps {
  scope: UsageScope
  range: TimeRange
}

export function RecentRequests(props: RecentRequestsProps) {
  const { t } = useTranslation()
  const logs = useRecentLogs(props.scope, props.range, 2)

  if (logs.isPending) {
    return <MobileLoading />
  }
  if (logs.isError) {
    return <MobileError title={t('Load failed')} description={t('Retry later.')} />
  }
  if (logs.data.items.length === 0) {
    return <MobileEmpty title={t('No recent requests')} description={t('Nothing to show for this range.')} />
  }

  return (
    <div className='divide-y'>
      {logs.data.items.map((row) => (
        <ValueRow
          key={row.id}
          label={row.model_name}
          secondary={`${formatTimestampRelative(row.created_at)} · ${formatUseTime(row.use_time)}`}
          value={`${formatTokens(row.prompt_tokens + row.completion_tokens)} · ${formatLogQuota(row.quota)}`}
        />
      ))}
    </div>
  )
}
```

`web/src/mobile/features/usage/components/usage-page.tsx`：

```tsx
import { useState } from 'react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'
import { formatTokens, formatNumber } from '@/lib/format'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { KpiCard } from '@/mobile/components/kpi-card'
import { ValueRow } from '@/mobile/components/value-row'
import { useLiveRate, useUsageAggregate, useUsageRanking } from '@/mobile/features/usage/api'
import { RecentRequests } from '@/mobile/features/usage/components/recent-requests'
import { aggregateTotals, rankRows, resolveTimeRange } from '@/mobile/features/usage/lib/usage-summary'
import type { TimeRangePreset, UsageScope } from '@/mobile/types'

const PRESETS: readonly TimeRangePreset[] = ['today', '7d', '30d']
const PRESET_LABEL_KEY: Record<TimeRangePreset, string> = {
  today: 'Today',
  '7d': 'Last 7 days',
  '30d': 'Last 30 days',
}

export function UsagePage() {
  const { t, i18n } = useTranslation()
  const locale = i18n.resolvedLanguage || i18n.language
  const [scope, setScope] = useState<UsageScope>('self')
  const [preset, setPreset] = useState<TimeRangePreset>('today')
  // 时间窗只在挂载与切换预设时冻结：range 会进 queryKey，渲染期取 Date.now() 会让
  // 每次响应到达都产生新 key（新缓存条目 → 再次 pending → 再次请求），在真实移动网络上
  // 表现为「加载—闪数据—加载」并持续打后端。Task 8 review Important-1。
  const [anchor, setAnchor] = useState(() => Math.floor(Date.now() / 1000))
  const range = useMemo(() => resolveTimeRange(preset, anchor), [preset, anchor])
  const selectPreset = (next: TimeRangePreset) => {
    setPreset(next)
    setAnchor(Math.floor(Date.now() / 1000))
  }

  const aggregate = useUsageAggregate(scope, range)
  const rate = useLiveRate(scope, range)
  const ranking = useUsageRanking(scope, range, scope === 'all')

  if (aggregate.isPending) {
    return <MobileLoading />
  }
  if (aggregate.isError) {
    return <MobileError title={t('Load failed')} description={t('Retry later.')} />
  }

  const totals = aggregateTotals(aggregate.data)
  const modelRanking = rankRows(aggregate.data, 'model_name', 5)
  const userRanking = ranking.data ? rankRows(ranking.data, 'username', 5) : []

  return (
    <div className='space-y-4 px-3 pb-4'>
      <div role='group' aria-label={t('Usage scope')} className='flex gap-2'>
        {(['self', 'all'] as const).map((option) => (
          <button
            key={option}
            type='button'
            aria-pressed={scope === option}
            className={scope === option ? 'text-foreground text-sm font-medium' : 'text-muted-foreground text-sm'}
            onClick={() => setScope(option)}
          >
            {t(option === 'self' ? 'Mine' : 'All')}
          </button>
        ))}
      </div>

      <div role='group' aria-label={t('Time range')} className='flex gap-2'>
        {PRESETS.map((option) => (
          <button
            key={option}
            type='button'
            aria-pressed={preset === option}
            onClick={() => selectPreset(option)}
            className={preset === option ? 'text-foreground text-sm font-medium' : 'text-muted-foreground text-sm'}
          >
            {t(PRESET_LABEL_KEY[option])}
          </button>
        ))}
      </div>

      <div className='grid grid-cols-2 gap-2'>
        <KpiCard label='Requests' value={totals.requests} hint={PRESET_LABEL_KEY[preset]} />
        <KpiCard label='Tokens' value={formatTokens(totals.tokens)} hint={PRESET_LABEL_KEY[preset]} />
        <KpiCard label='Cost' value={formatQuotaWithCurrency(totals.quota, locale)} hint={PRESET_LABEL_KEY[preset]} />
        <KpiCard
          label='Current rate'
          value={typeof rate.data?.rpm === 'number' ? `${formatNumber(rate.data.rpm, locale)} / ${formatTokens(rate.data.tpm)}` : '—'}
          hint='Last 60 seconds'
        />
      </div>

      <p className='text-muted-foreground text-[11px]'>{t('Summary data updates every few minutes.')}</p>

      <section>
        <h2 className='px-3 py-1 text-sm font-medium'>{t('By model')}</h2>
        <div className='divide-y'>
          {modelRanking.map((row) => (
            <ValueRow key={row.key} label={row.key} value={formatTokens(row.tokens)} secondary={formatQuotaWithCurrency(row.quota, locale)} />
          ))}
        </div>
      </section>

      {scope === 'all' ? (
        <section>
          <h2 className='px-3 py-1 text-sm font-medium'>{t('By user')}</h2>
          <div className='divide-y'>
            {userRanking.map((row) => (
              <ValueRow key={row.key} label={row.key} value={formatTokens(row.tokens)} secondary={formatQuotaWithCurrency(row.quota, locale)} />
            ))}
          </div>
        </section>
      ) : null}

      <section>
        <h2 className='px-3 py-1 text-sm font-medium'>{t('Recent requests')}</h2>
        <RecentRequests scope={scope} range={range} />
      </section>
    </div>
  )
}
```

`web/src/mobile/app.tsx` 把 `data-tab` 面板替换为按页签渲染（`usage` 用真实页面，其余先保留空面板）：

```tsx
        {activeTab === 'usage' ? <UsagePage /> : <section data-testid='mobile-panel' data-tab={activeTab} />}
```

并在文件头部加入 `import { UsagePage } from '@/mobile/features/usage/components/usage-page'`。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile && bun run typecheck && bunx oxlint -c .oxlintrc.json src/mobile`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add web/src/mobile/types.ts web/src/mobile/features/usage web/src/mobile/app.tsx
git commit -m "feat(web-mobile): add the usage tab with mine/all scopes and rankings"
```

---

## Task 9：模型可用性页

**Files:**
- Modify: `web/src/mobile/types.ts`
- Create: `web/src/mobile/features/models/lib/availability.ts`、`web/src/mobile/features/models/api.ts`、`web/src/mobile/features/models/components/models-page.tsx`
- Create: `web/src/mobile/features/models/lib/__tests__/availability.test.ts`、`web/src/mobile/features/models/components/__tests__/models-page.test.tsx`
- Modify: `web/src/mobile/app.tsx`

**Interfaces:**
- Produces:
  - `flattenAvailability(sites: MonitorSiteResponse[]): AvailabilityRow[]`
  - `countByHealth(sites: MonitorSiteResponse[]): Record<AvailabilityHealth, number>`
  - `useModelAvailability()`；`<ModelsPage />`
  - 类型 `MonitorSiteModel`、`MonitorSiteSummary`、`MonitorSiteResponse`、`AvailabilityHealth`、`AvailabilityRow`

- [ ] **Step 1: 写失败测试**

`web/src/mobile/features/models/lib/__tests__/availability.test.ts`：

```ts
import { describe, expect, it } from 'vitest'

import { countByHealth, flattenAvailability } from '@/mobile/features/models/lib/availability'
import type { MonitorSiteResponse } from '@/mobile/types'

const sites: MonitorSiteResponse[] = [
  {
    site: { id: 1, name: 'primary', enabled: true },
    summary: {
      score: 80,
      health: 'degraded',
      models: [
        { model_name: 'gpt-5', status: 'available', latest_status: 'success', weight: 100, stale: false },
        {
          model_name: 'claude-5',
          status: 'unavailable',
          latest_status: 'failure',
          latest_failure_type: 'timeout',
          latest_error_summary: 'upstream timeout',
          weight: 50,
          stale: false,
        },
      ],
    },
    channel_ids: [3],
    latest_observed_at: 1000,
    freshness_seconds: 120,
  },
  {
    site: { id: 2, name: 'backup', enabled: true },
    summary: { score: 100, health: 'normal', models: [] },
    channel_ids: [],
    latest_observed_at: 0,
    freshness_seconds: undefined,
  },
]

describe('countByHealth', () => {
  it('counts sites per health bucket including unknown', () => {
    expect(countByHealth(sites)).toEqual({ normal: 1, degraded: 1, unavailable: 0, unknown: 0 })
  })

  it('returns zeros for no sites', () => {
    expect(countByHealth([])).toEqual({ normal: 0, degraded: 0, unavailable: 0, unknown: 0 })
  })
})

describe('flattenAvailability', () => {
  it('lists every model with its site and puts problems first', () => {
    const rows = flattenAvailability(sites)

    expect(rows.map((row) => row.modelName)).toEqual(['claude-5', 'gpt-5'])
    expect(rows[0]).toMatchObject({ siteName: 'primary', status: 'unavailable', latestFailureType: 'timeout' })
    expect(rows[1]).toMatchObject({ siteName: 'primary', status: 'available' })
  })

  it('returns an empty list when no site exposes models', () => {
    expect(flattenAvailability([sites[1]])).toEqual([])
  })
})
```

`web/src/mobile/features/models/components/__tests__/models-page.test.tsx`：

```tsx
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ModelsPage } from '@/mobile/features/models/components/models-page'
import { mobileQueryClient } from '@/mobile/lib/query-client'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

describe('ModelsPage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    mobileQueryClient.clear()
    vi.unstubAllGlobals()
  })

  it('shows the site health counters and the failing model', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        jsonResponse({
          success: true,
          data: {
            enabled: true,
            sites: [
              {
                site: { id: 1, name: 'primary', enabled: true },
                summary: {
                  score: 80,
                  health: 'degraded',
                  models: [
                    {
                      model_name: 'claude-5',
                      status: 'unavailable',
                      latest_status: 'failure',
                      latest_failure_type: 'timeout',
                      latest_error_summary: 'upstream timeout',
                      weight: 50,
                      stale: false,
                    },
                  ],
                },
                channel_ids: [3],
                latest_observed_at: 1000,
                freshness_seconds: 120,
              },
            ],
          },
        })
      )
    )

    render(
      <QueryClientProvider client={mobileQueryClient}>
        <ModelsPage />
      </QueryClientProvider>
    )

    await waitFor(() => expect(screen.getByText('claude-5')).toBeInTheDocument())
    expect(screen.getByText('Degraded')).toBeInTheDocument()
    expect(screen.getByText('upstream timeout')).toBeInTheDocument()
  })

  it('explains that the monitor is disabled', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ success: true, data: { enabled: false, sites: [] } })))

    render(
      <QueryClientProvider client={mobileQueryClient}>
        <ModelsPage />
      </QueryClientProvider>
    )

    await waitFor(() => expect(screen.getByText('Model monitoring is disabled on this instance.')).toBeInTheDocument())
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile/features/models`
Expected: FAIL — 模块不存在。

- [ ] **Step 3: 写最小实现**

`web/src/mobile/types.ts` 追加：

```ts
export type AvailabilityHealth = 'normal' | 'degraded' | 'unavailable' | 'unknown'

export interface MonitorSiteModel {
  model_name: string
  status: string
  latest_status: string
  latest_failure_type?: string
  latest_error_summary?: string
  weight: number
  stale: boolean
}

export interface MonitorSiteSummary {
  score: number
  health: string
  models: MonitorSiteModel[]
}

export interface MonitorSiteResponse {
  site: { id: number; name: string; enabled: boolean }
  summary: MonitorSiteSummary
  channel_ids: number[]
  latest_observed_at: number
  freshness_seconds?: number
}

export interface ModelMonitorSummary {
  enabled: boolean
  sites: MonitorSiteResponse[]
}

export interface AvailabilityRow {
  siteName: string
  modelName: string
  status: string
  latestFailureType?: string
  latestErrorSummary?: string
  stale: boolean
}
```

`web/src/mobile/features/models/lib/availability.ts`：

```ts
import type { AvailabilityHealth, AvailabilityRow, MonitorSiteResponse } from '@/mobile/types'

const HEALTH_BUCKETS: readonly AvailabilityHealth[] = ['normal', 'degraded', 'unavailable', 'unknown']

function normalizeHealth(value: string): AvailabilityHealth {
  return HEALTH_BUCKETS.find((health) => health === value) ?? 'unknown'
}

export function countByHealth(sites: MonitorSiteResponse[]): Record<AvailabilityHealth, number> {
  const counts: Record<AvailabilityHealth, number> = { normal: 0, degraded: 0, unavailable: 0, unknown: 0 }
  for (const site of sites) {
    counts[normalizeHealth(site.summary.health)] += 1
  }
  return counts
}

export function flattenAvailability(sites: MonitorSiteResponse[]): AvailabilityRow[] {
  const rows: AvailabilityRow[] = []
  for (const site of sites) {
    for (const model of site.summary.models) {
      rows.push({
        siteName: site.site.name,
        modelName: model.model_name,
        status: model.status,
        latestFailureType: model.latest_failure_type,
        latestErrorSummary: model.latest_error_summary,
        stale: model.stale,
      })
    }
  }
  const severity = (row: AvailabilityRow): number => {
    if (row.status === 'unavailable') {
      return 0
    }
    if (row.status === 'limited') {
      return 1
    }
    if (row.stale) {
      return 2
    }
    return 3
  }
  return rows.sort((left, right) => severity(left) - severity(right) || left.modelName.localeCompare(right.modelName))
}
```

`web/src/mobile/features/models/api.ts`：

```ts
import { useQuery } from '@tanstack/react-query'

import { mobileApiGet } from '@/mobile/lib/api-client'
import { MOBILE_STALE_TIME } from '@/mobile/lib/query-client'
import type { ModelMonitorSummary } from '@/mobile/types'

export function useModelAvailability() {
  return useQuery({
    queryKey: ['mobile', 'model-monitor', 'summary'],
    queryFn: () => mobileApiGet<ModelMonitorSummary>('/api/model-monitor/summary'),
    staleTime: MOBILE_STALE_TIME.availability,
  })
}
```

`web/src/mobile/features/models/components/models-page.tsx`：

```tsx
import { useTranslation } from 'react-i18next'

import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'
import { formatTimestampRelative } from '@/lib/format'
import { KpiCard } from '@/mobile/components/kpi-card'
import { ValueRow } from '@/mobile/components/value-row'
import { useModelAvailability } from '@/mobile/features/models/api'
import { countByHealth, flattenAvailability } from '@/mobile/features/models/lib/availability'
import type { AvailabilityHealth } from '@/mobile/types'

const HEALTH_LABEL_KEY: Record<AvailabilityHealth, string> = {
  normal: 'Healthy',
  degraded: 'Degraded',
  unavailable: 'Unavailable',
  unknown: 'Unknown',
}

const MODEL_STATUS_LABEL_KEY: Record<string, string> = {
  available: 'Available',
  limited: 'Limited',
  unavailable: 'Unavailable',
  unknown: 'Unknown',
}

export function ModelsPage() {
  const { t } = useTranslation()
  const availability = useModelAvailability()

  if (availability.isPending) {
    return <MobileLoading />
  }
  if (availability.isError) {
    return <MobileError title={t('Load failed')} description={t('Retry later.')} />
  }
  if (!availability.data.enabled) {
    return <p className='text-muted-foreground px-3 py-6 text-sm'>{t('Model monitoring is disabled on this instance.')}</p>
  }

  const counts = countByHealth(availability.data.sites)
  const rows = flattenAvailability(availability.data.sites)
  const newestObservation = availability.data.sites.reduce((latest, site) => Math.max(latest, site.latest_observed_at), 0)

  return (
    <div className='space-y-4 px-3 pb-4'>
      <div className='grid grid-cols-2 gap-2'>
        {(Object.keys(HEALTH_LABEL_KEY) as AvailabilityHealth[]).map((health) => (
          <KpiCard key={health} label={HEALTH_LABEL_KEY[health]} value={counts[health]} hint='Sites' />
        ))}
      </div>

      <p className='text-muted-foreground text-[11px]'>
        {t('Latest probe')} {formatTimestampRelative(newestObservation)}
      </p>

      <section>
        <h2 className='px-3 py-1 text-sm font-medium'>{t('Models by site')}</h2>
        <div className='divide-y'>
          {rows.map((row) => (
            <ValueRow
              key={`${row.siteName}/${row.modelName}`}
              label={row.modelName}
              secondary={`${row.siteName}${row.latestFailureType ? ` · ${row.latestFailureType}` : ''}`}
              value={t(MODEL_STATUS_LABEL_KEY[row.status] ?? 'Unknown')}
            />
          ))}
        </div>
      </section>

      {rows.some((row) => row.latestErrorSummary) ? (
        <section>
          <h2 className='px-3 py-1 text-sm font-medium'>{t('Latest failures')}</h2>
          <ul className='space-y-1 px-3'>
            {rows
              .filter((row) => row.latestErrorSummary)
              .map((row) => (
                <li key={`error-${row.siteName}-${row.modelName}`} className='text-xs'>
                  <span className='font-medium'>{row.modelName}</span> {row.latestErrorSummary}
                </li>
              ))}
          </ul>
        </section>
      ) : null}
    </div>
  )
}
```

`web/src/mobile/app.tsx`：加入 `models` 分支：

```tsx
        {activeTab === 'usage' ? <UsagePage /> : null}
        {activeTab === 'models' ? <ModelsPage /> : null}
        {activeTab !== 'usage' && activeTab !== 'models' ? <section data-testid='mobile-panel' data-tab={activeTab} /> : null}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile && bun run typecheck && bunx oxlint -c .oxlintrc.json src/mobile`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add web/src/mobile/types.ts web/src/mobile/features/models web/src/mobile/app.tsx
git commit -m "feat(web-mobile): add the model availability tab"
```

---

## Task 10：路由统计页

**Files:**
- Modify: `web/src/mobile/types.ts`
- Create: `web/src/mobile/features/routing/lib/routing-share.ts`、`web/src/mobile/features/routing/api.ts`、`web/src/mobile/features/routing/components/routing-page.tsx`、`web/src/mobile/components/mini-bar.tsx`
- Create: `web/src/mobile/features/routing/lib/__tests__/routing-share.test.ts`、`web/src/mobile/components/__tests__/mini-bar.test.tsx`、`web/src/mobile/features/routing/components/__tests__/routing-page.test.tsx`
- Modify: `web/src/mobile/app.tsx`

**Interfaces:**
- Produces:
  - `summarizeRoutingShare(rows: RoutingModelChannelStat[]): RoutingShareRow[]`，其中
    `RoutingShareRow = { modelName, requests, errors, avgUseTime, configuredShare: number | null, channels: RoutingChannelShare[] }`（模型级 `configuredShare` 只表示该模型的渠道权重是否已知：已知为 1，未知为 `null`；真正要展示的权重分配在渠道级 `configuredShare`），
    `RoutingChannelShare = { channelId, channelName, requests, configuredShare: number | null }`
  - `topSwitches(rows: RoutingSwitchStat[], limit: number): RoutingSwitchStat[]`
  - `useRoutingStats(range)`、`<RoutingPage />`、`<MiniBar segments />`

- [ ] **Step 1: 写失败测试**

`web/src/mobile/features/routing/lib/__tests__/routing-share.test.ts`：

```ts
import { describe, expect, it } from 'vitest'

import { summarizeRoutingShare, topSwitches } from '@/mobile/features/routing/lib/routing-share'
import type { RoutingModelChannelStat, RoutingSwitchStat } from '@/mobile/types'

const rows: RoutingModelChannelStat[] = [
  { model_name: 'gpt-5', channel_id: 1, channel_name: 'a', requests: 100, errors: 2, avg_use_time: 1.5, priority: 0, weight: 10 },
  { model_name: 'gpt-5', channel_id: 2, channel_name: 'b', requests: 300, errors: 0, avg_use_time: 1.1, priority: 0, weight: 30 },
  { model_name: 'claude-5', channel_id: 3, channel_name: 'c', requests: 0, errors: 0, avg_use_time: 0 },
]

describe('summarizeRoutingShare', () => {
  it('compares observed share against the configured weight share', () => {
    const [gpt5] = summarizeRoutingShare(rows)

    expect(gpt5.modelName).toBe('gpt-5')
    expect(gpt5.requests).toBe(400)
    expect(gpt5.errors).toBe(2)
    expect(gpt5.configuredShare).toBeCloseTo(1)
    expect(gpt5.channels[0]).toMatchObject({ channelName: 'b', requests: 300 })
    expect(gpt5.channels[0].configuredShare).toBeCloseTo(0.75)
    expect(gpt5.channels[1].configuredShare).toBeCloseTo(0.25)
  })

  it('marks the configured share as unknown when no channel reports a weight', () => {
    const [claude] = summarizeRoutingShare([rows[2]])

    expect(claude.configuredShare).toBeNull()
    expect(claude.channels[0].configuredShare).toBeNull()
  })

  it('returns an empty list for no rows', () => {
    expect(summarizeRoutingShare([])).toEqual([])
  })
})

describe('topSwitches', () => {
  it('sorts by count and truncates', () => {
    const switches: RoutingSwitchStat[] = [
      { from: 1, to: 2, count: 3 },
      { from: 2, to: 3, count: 9 },
    ]

    expect(topSwitches(switches, 1)).toEqual([{ from: 2, to: 3, count: 9 }])
  })
})
```

`web/src/mobile/components/__tests__/mini-bar.test.tsx`：

```tsx
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { MiniBar } from '@/mobile/components/mini-bar'

describe('MiniBar', () => {
  it('renders one segment per entry with proportional width', () => {
    render(<MiniBar segments={[{ key: 'a', share: 0.25 }, { key: 'b', share: 0.75 }]} />)

    const segments = screen.getAllByTestId('mini-bar-segment')
    expect(segments).toHaveLength(2)
    expect(segments[0]).toHaveStyle({ width: '25%' })
    expect(segments[1]).toHaveStyle({ width: '75%' })
  })

  it('renders nothing for an empty segment list', () => {
    render(<MiniBar segments={[]} />)

    expect(screen.queryAllByTestId('mini-bar-segment')).toHaveLength(0)
  })
})
```

`web/src/mobile/features/routing/components/__tests__/routing-page.test.tsx`：

```tsx
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { RoutingPage } from '@/mobile/features/routing/components/routing-page'
import { mobileQueryClient } from '@/mobile/lib/query-client'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

describe('RoutingPage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    mobileQueryClient.clear()
    vi.unstubAllGlobals()
  })

  it('shows per-model request split with configured weights', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (String(url).startsWith('/api/log/routing_stats')) {
          return jsonResponse({
            success: true,
            data: {
              by_model_channel: [
                { model_name: 'gpt-5', channel_id: 1, channel_name: 'a', requests: 100, errors: 0, avg_use_time: 1.5, priority: 0, weight: 10 },
                { model_name: 'gpt-5', channel_id: 2, channel_name: 'b', requests: 300, errors: 0, avg_use_time: 1.1, priority: 0, weight: 30 },
              ],
              switches: [{ from: 1, to: 2, count: 4 }],
              reasons: [{ reason: 'channel_error', count: 4 }],
              affinity: [{ rule_name: 'default', sticky_requests: 12, distinct_keys: 3 }],
              window: { start: 0, end: 3600, bucket_seconds: 600 },
              trend: [{ timestamp: 0, requests: 400, switched: 4 }],
            },
          })
        }
        return jsonResponse({ success: true, data: { entries: [], total: 0 } })
      })
    )

    render(
      <QueryClientProvider client={mobileQueryClient}>
        <RoutingPage />
      </QueryClientProvider>
    )

    await waitFor(() => expect(screen.getByText('gpt-5')).toBeInTheDocument())
    expect(screen.getByText('Channel switches')).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile/features/routing src/mobile/components/__tests__/mini-bar.test.tsx`
Expected: FAIL — 模块不存在。

- [ ] **Step 3: 写最小实现**

`web/src/mobile/types.ts` 追加：

```ts
export interface RoutingModelChannelStat {
  model_name: string
  channel_id: number
  channel_name: string
  requests: number
  errors: number
  avg_use_time: number
  priority?: number
  weight?: number
}

export interface RoutingSwitchStat {
  from: number
  to: number
  count: number
}

export interface RoutingReasonStat {
  reason: string
  count: number
}

export interface RoutingAffinityStat {
  rule_name: string
  sticky_requests: number
  distinct_keys: number
}

export interface RoutingStatsWindow {
  start: number
  end: number
  bucket_seconds: number
}

export interface RoutingTrendPoint {
  timestamp: number
  requests: number
  switched: number
}

export interface RoutingStats {
  requests: number
  errors: number
  by_model_channel: RoutingModelChannelStat[]
  window: RoutingStatsWindow
  trend: RoutingTrendPoint[]
  // trail 扫描会截断，switched/scanned/switches 只覆盖被扫到的部分（Task 10 review Minor-2）
  scanned: number
  truncated: boolean
  switched: number
  switched_success: number
  switched_failed: number
  sticky: number
  switches: RoutingSwitchStat[]
  switch_reasons: RoutingReasonStat[]
  affinity_by_rule: RoutingAffinityStat[]
}

export interface ChannelAffinityBinding {
  channel_id: number
  channel_name: string
  rule_name: string
  key: string
}

export interface ChannelAffinityBindings {
  entries: ChannelAffinityBinding[]
  total: number
}
```

`web/src/mobile/features/routing/lib/routing-share.ts`：

```ts
import type { RoutingModelChannelStat, RoutingSwitchStat } from '@/mobile/types'

export interface RoutingChannelShare {
  channelId: number
  channelName: string
  requests: number
  configuredShare: number | null
}

export interface RoutingShareRow {
  modelName: string
  requests: number
  errors: number
  avgUseTime: number
  configuredShare: number | null
  channels: RoutingChannelShare[]
}

export function summarizeRoutingShare(rows: RoutingModelChannelStat[]): RoutingShareRow[] {
  const byModel = new Map<string, RoutingModelChannelStat[]>()
  for (const row of rows) {
    const bucket = byModel.get(row.model_name)
    if (bucket) {
      bucket.push(row)
    } else {
      byModel.set(row.model_name, [row])
    }
  }

  return [...byModel.entries()].map(([modelName, channels]) => {
    const requests = channels.reduce((total, channel) => total + channel.requests, 0)
    const configuredWeight = channels.reduce((total, channel) => total + (channel.weight ?? 0), 0)
    const hasConfiguredWeight = channels.some((channel) => channel.weight !== undefined && channel.weight > 0)
    return {
      modelName,
      requests,
      errors: channels.reduce((total, channel) => total + channel.errors, 0),
      avgUseTime: channels.reduce((total, channel) => total + channel.avg_use_time * channel.requests, 0) / (requests || 1),
      configuredShare: hasConfiguredWeight ? 1 : null,
      channels: channels
        .map((channel) => ({
          channelId: channel.channel_id,
          channelName: channel.channel_name,
          requests: channel.requests,
          configuredShare: hasConfiguredWeight ? (channel.weight ?? 0) / configuredWeight : null,
        }))
        .sort((left, right) => right.requests - left.requests),
    }
  })
}

export function topSwitches(rows: RoutingSwitchStat[], limit: number): RoutingSwitchStat[] {
  return [...rows].sort((left, right) => right.count - left.count).slice(0, limit)
}
```

`web/src/mobile/components/mini-bar.tsx`：

```tsx
import { cn } from '@/lib/utils'

interface MiniBarSegment {
  key: string
  share: number
  className?: string
}

interface MiniBarProps {
  segments: MiniBarSegment[]
}

export function MiniBar(props: MiniBarProps) {
  if (props.segments.length === 0) {
    return null
  }
  return (
    <div className='bg-muted flex h-2 w-full overflow-hidden rounded-full' role='presentation'>
      {props.segments.map((segment) => (
        <span
          key={segment.key}
          data-testid='mini-bar-segment'
          className={cn('bg-primary h-full', segment.className)}
          style={{ width: `${Math.max(0, Math.min(1, segment.share)) * 100}%` }}
        />
      ))}
    </div>
  )
}
```

`web/src/mobile/features/routing/api.ts`：

```ts
import { keepPreviousData, useQuery } from '@tanstack/react-query'

import { mobileApiGet } from '@/mobile/lib/api-client'
import { MOBILE_STALE_TIME } from '@/mobile/lib/query-client'
import type { ChannelAffinityBindings, RoutingStats, TimeRange } from '@/mobile/types'

export function useRoutingStats(range: TimeRange) {
  return useQuery({
    queryKey: ['mobile', 'routing', 'stats', range.start, range.end],
    queryFn: () =>
      mobileApiGet<RoutingStats>('/api/log/routing_stats', {
        start_timestamp: range.start,
        end_timestamp: range.end,
      }),
    staleTime: MOBILE_STALE_TIME.routing,
    placeholderData: keepPreviousData,
  })
}

export function useAffinityBindings() {
  return useQuery({
    queryKey: ['mobile', 'routing', 'affinity'],
    queryFn: () => mobileApiGet<ChannelAffinityBindings>('/api/log/channel_affinity_bindings', { limit: 20 }),
    staleTime: MOBILE_STALE_TIME.routing,
  })
}
```

`web/src/mobile/features/routing/components/routing-page.tsx`：

```tsx
import { useState } from 'react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'
import { formatNumber, formatPercent, formatUseTime } from '@/lib/format'
import { KpiCard } from '@/mobile/components/kpi-card'
import { MiniBar } from '@/mobile/components/mini-bar'
import { ValueRow } from '@/mobile/components/value-row'
import { useAffinityBindings, useRoutingStats } from '@/mobile/features/routing/api'
import { summarizeRoutingShare, topSwitches } from '@/mobile/features/routing/lib/routing-share'
import { resolveTimeRange } from '@/mobile/features/usage/lib/usage-summary'
import type { TimeRangePreset } from '@/mobile/types'

const PRESETS: readonly TimeRangePreset[] = ['today', '7d', '30d']
const PRESET_LABEL_KEY: Record<TimeRangePreset, string> = {
  today: 'Today',
  '7d': 'Last 7 days',
  '30d': 'Last 30 days',
}

export function RoutingPage() {
  const { t, i18n } = useTranslation()
  const locale = i18n.resolvedLanguage || i18n.language
  const [preset, setPreset] = useState<TimeRangePreset>('today')
  // 时间窗只在挂载与切换预设时冻结：range 会进 queryKey，渲染期取 Date.now() 会让
  // 每次响应到达都产生新 key（新缓存条目 → 再次 pending → 再次请求），在真实移动网络上
  // 表现为「加载—闪数据—加载」并持续打后端。Task 8 review Important-1。
  const [anchor, setAnchor] = useState(() => Math.floor(Date.now() / 1000))
  const range = useMemo(() => resolveTimeRange(preset, anchor), [preset, anchor])
  const selectPreset = (next: TimeRangePreset) => {
    setPreset(next)
    setAnchor(Math.floor(Date.now() / 1000))
  }
  const stats = useRoutingStats(range)
  const affinity = useAffinityBindings()

  if (stats.isPending) {
    return <MobileLoading />
  }
  if (stats.isError) {
    return <MobileError title={t('Load failed')} description={t('Retry later.')} />
  }

  const models = summarizeRoutingShare(stats.data.by_model_channel)
  const switches = topSwitches(stats.data.switches, 5)
  const stickyRequests = stats.data.affinity.reduce((total, row) => total + row.sticky_requests, 0)

  return (
    <div className='space-y-4 px-3 pb-4'>
      <div role='group' aria-label={t('Time range')} className='flex gap-2'>
        {PRESETS.map((option) => (
          <button
            key={option}
            type='button'
            aria-pressed={preset === option}
            onClick={() => selectPreset(option)}
            className={preset === option ? 'text-foreground text-sm font-medium' : 'text-muted-foreground text-sm'}
          >
            {t(PRESET_LABEL_KEY[option])}
          </button>
        ))}
      </div>

      <div className='grid grid-cols-2 gap-2'>
        <KpiCard label='Requests' value={models.reduce((total, model) => total + model.requests, 0)} hint={PRESET_LABEL_KEY[preset]} />
        <KpiCard label='Switched requests' value={formatNumber(stats.data.switched, locale)} hint={PRESET_LABEL_KEY[preset]} />
      </div>

      {stats.data.truncated ? (
        <p role='status' className='text-muted-foreground text-[11px]'>
          {t('Some switches in this window were not scanned.')}
        </p>
      ) : null}

      {models.length === 0 ? (
        <p className='text-muted-foreground text-sm'>{t('Run requests, or widen the time window, to collect routing statistics.')}</p>
      ) : (
        <section className='space-y-3'>
          {models.map((model) => (
            <article key={model.modelName} className='space-y-2'>
              <div className='flex items-baseline justify-between'>
                <h2 className='text-sm font-medium'>{model.modelName}</h2>
                <p className='text-muted-foreground text-xs'>
                  {formatNumber(model.requests, locale)} · {formatNumber(model.errors, locale)} {t('errors')} · {formatUseTime(model.avgUseTime)}
                </p>
              </div>
              <MiniBar
                segments={model.channels.map((channel) => ({
                  key: String(channel.channelId),
                  share: model.requests === 0 ? 0 : channel.requests / model.requests,
                }))}
              />
              <div className='divide-y'>
                {model.channels.map((channel) => (
                  <ValueRow
                    key={channel.channelId}
                    label={channel.channelName || `#${channel.channelId}`}
                    secondary={
                      channel.configuredShare === null
                        ? t('Configured weight unknown')
                        : `${t('Configured weight')} ${formatPercent(channel.configuredShare * 100)}`
                    }
                    value={formatPercent((model.requests === 0 ? 0 : channel.requests / model.requests) * 100)}
                  />
                ))}
              </div>
            </article>
          ))}
        </section>
      )}

      <section>
        <h2 className='px-3 py-1 text-sm font-medium'>{t('Sticky sessions')}</h2>
        <ValueRow label={t('Sticky sessions')} value={formatNumber(stickyRequests, locale)} secondary={affinity.data ? `${affinity.data.total} ${t('keys')}` : undefined} />
      </section>

      <section>
        <h2 className='px-3 py-1 text-sm font-medium'>{t('Channel switches')}</h2>
        <div className='divide-y'>
          {switches.map((row) => (
            <ValueRow key={`${row.from}-${row.to}`} label={`${row.from} → ${row.to}`} value={formatNumber(row.count, locale)} />
          ))}
        </div>
      </section>
    </div>
  )
}
```

`web/src/mobile/app.tsx`：加入 `routing` 分支（`activeTab === 'routing' ? <RoutingPage /> : null`），并把兜底面板条件改为 `activeTab === 'channels'`。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile && bun run typecheck && bunx oxlint -c .oxlintrc.json src/mobile`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add web/src/mobile/types.ts web/src/mobile/components/mini-bar.tsx web/src/mobile/components/__tests__/mini-bar.test.tsx web/src/mobile/features/routing web/src/mobile/app.tsx
git commit -m "feat(web-mobile): add the routing statistics tab"
```

---

## Task 11：渠道页与启用/禁用开关

**Files:**
- Modify: `web/src/mobile/types.ts`
- Create: `web/src/mobile/features/channels/lib/channel-status.ts`、`web/src/mobile/features/channels/api.ts`、`web/src/mobile/features/channels/components/channels-page.tsx`
- Create: `web/src/mobile/features/channels/lib/__tests__/channel-status.test.ts`、`web/src/mobile/features/channels/components/__tests__/channels-page.test.tsx`
- Modify: `web/src/mobile/app.tsx`、`web/src/mobile/providers.tsx`

**Interfaces:**
- Produces:
  - `const CHANNEL_STATUS = { enabled: 1, manuallyDisabled: 2, autoDisabled: 3 } as const`
  - `channelToggleTarget(status: number): number`——`1 → 2`，其余（含 `3`）`→ 1`
  - `channelStatusLabelKey(status: number): string`
  - `useChannels(filters)`、`useToggleChannelStatus()`；`<ChannelsPage />`
  - 类型 `ChannelRow`、`ChannelListResult`、`ChannelFilters`

- [ ] **Step 1: 写失败测试**

`web/src/mobile/features/channels/lib/__tests__/channel-status.test.ts`：

```ts
import { describe, expect, it } from 'vitest'

import { channelStatusLabelKey, channelToggleTarget } from '@/mobile/features/channels/lib/channel-status'

describe('channelToggleTarget', () => {
  it('disables an enabled channel with the manual-disabled status', () => {
    expect(channelToggleTarget(1)).toBe(2)
  })

  it('enables a manually disabled channel', () => {
    expect(channelToggleTarget(2)).toBe(1)
  })

  // The server rejects status 3 for writes (isManageableChannelStatus only
  // accepts 1 and 2), so an auto-disabled channel can only be enabled.
  it('enables an auto disabled channel instead of sending status 3', () => {
    expect(channelToggleTarget(3)).toBe(1)
  })
})

describe('channelStatusLabelKey', () => {
  it('maps every server status to a distinct label key', () => {
    expect(channelStatusLabelKey(1)).toBe('Enabled')
    expect(channelStatusLabelKey(2)).toBe('Manually disabled')
    expect(channelStatusLabelKey(3)).toBe('Auto disabled')
    expect(channelStatusLabelKey(9)).toBe('Unknown status')
  })
})
```

`web/src/mobile/features/channels/components/__tests__/channels-page.test.tsx`：

```tsx
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ChannelsPage } from '@/mobile/features/channels/components/channels-page'
import { mobileQueryClient } from '@/mobile/lib/query-client'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

const CHANNEL = {
  id: 7,
  name: 'primary-openai',
  type: 1,
  status: 1,
  group: 'default',
  balance: 12.5,
  used_quota: 0,
  priority: 0,
  weight: 10,
}

function renderPage() {
  render(
    <QueryClientProvider client={mobileQueryClient}>
      <ChannelsPage />
    </QueryClientProvider>
  )
}

describe('ChannelsPage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    mobileQueryClient.clear()
    vi.unstubAllGlobals()
  })

  it('lists channels with their status label', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ success: true, data: { items: [CHANNEL], total: 1, page: 1, page_size: 20 } })))

    renderPage()

    await waitFor(() => expect(screen.getByText('primary-openai')).toBeInTheDocument())
    expect(screen.getByText('Enabled')).toBeInTheDocument()
  })

  it('asks for confirmation and posts the manual-disabled status', async () => {
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      if (String(url).startsWith('/api/channel/7/status')) {
        return jsonResponse({ success: true, data: true })
      }
      if (init?.method === 'GET' || init?.method === undefined) {
        return jsonResponse({ success: true, data: { items: [CHANNEL], total: 1, page: 1, page_size: 20 } })
      }
      return jsonResponse({ success: true, data: null })
    })
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    renderPage()
    await waitFor(() => expect(screen.getByText('primary-openai')).toBeInTheDocument())

    await user.click(screen.getByRole('switch', { name: 'Enabled primary-openai' }))
    await user.click(await screen.findByRole('button', { name: 'Disable channel' }))

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(([url]) => String(url).startsWith('/api/channel/7/status'))
      expect(call?.[1]?.method).toBe('POST')
      expect(call?.[1]?.body).toBe('{"status":2}')
    })
  })

  it('filters the list by channel name', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ success: true, data: { items: [CHANNEL], total: 1, page: 1, page_size: 20 } }))
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    renderPage()
    await waitFor(() => expect(screen.getByText('primary-openai')).toBeInTheDocument())

    await user.type(screen.getByLabelText('Search channels'), 'openai')

    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).includes('name=openai'))).toBe(true))
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile/features/channels`
Expected: FAIL — 模块不存在。

- [ ] **Step 3: 写最小实现**

`web/src/mobile/types.ts` 追加：

```ts
export interface ChannelRow {
  id: number
  name: string
  type: number
  status: number
  group: string
  balance: number
  used_quota: number
  priority: number
  weight: number
}

export interface ChannelFilters {
  name: string
  statusFilter: number
}

export type ChannelListResult = PagedResult<ChannelRow>
```

`web/src/mobile/features/channels/lib/channel-status.ts`：

```ts
export const CHANNEL_STATUS = {
  enabled: 1,
  manuallyDisabled: 2,
  autoDisabled: 3,
} as const

const STATUS_LABEL_KEY: Record<number, string> = {
  [CHANNEL_STATUS.enabled]: 'Enabled',
  [CHANNEL_STATUS.manuallyDisabled]: 'Manually disabled',
  [CHANNEL_STATUS.autoDisabled]: 'Auto disabled',
}

// The server only accepts 1 and 2 for status writes, so an auto-disabled
// channel can be enabled but never re-disabled through this control.
export function channelToggleTarget(status: number): number {
  return status === CHANNEL_STATUS.enabled ? CHANNEL_STATUS.manuallyDisabled : CHANNEL_STATUS.enabled
}

export function channelStatusLabelKey(status: number): string {
  return STATUS_LABEL_KEY[status] ?? 'Unknown status'
}
```

`web/src/mobile/features/channels/api.ts`：

```ts
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { mobileApiGet, mobileApiPost } from '@/mobile/lib/api-client'
import { MOBILE_STALE_TIME } from '@/mobile/lib/query-client'
import type { ChannelFilters, ChannelListResult } from '@/mobile/types'

export function useChannels(filters: ChannelFilters) {
  return useQuery({
    queryKey: ['mobile', 'channels', filters.name, filters.statusFilter],
    queryFn: () =>
      mobileApiGet<ChannelListResult>('/api/channel/', {
        p: 1,
        page_size: 50,
        name: filters.name === '' ? undefined : filters.name,
        status: filters.statusFilter === 0 ? undefined : filters.statusFilter,
      }),
    staleTime: MOBILE_STALE_TIME.channels,
  })
}

export function useToggleChannelStatus() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: { id: number; status: number }) => mobileApiPost<boolean>(`/api/channel/${input.id}/status`, { status: input.status }),
    onSettled: async () => {
      await queryClient.invalidateQueries({ queryKey: ['mobile', 'channels'] })
    },
  })
}
```

`web/src/mobile/features/channels/components/channels-page.tsx`：用 `@/components/ui/switch`、`@/components/confirm-dialog`、`@/components/ui/input`、`@/mobile/components/mobile-empty`、`sonner`：

```tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { MobileEmpty } from '@/mobile/components/mobile-empty'
import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { ValueRow } from '@/mobile/components/value-row'
import { useChannels, useToggleChannelStatus } from '@/mobile/features/channels/api'
import { CHANNEL_STATUS, channelStatusLabelKey, channelToggleTarget } from '@/mobile/features/channels/lib/channel-status'
import type { ChannelFilters, ChannelRow } from '@/mobile/types'

const STATUS_FILTERS: readonly number[] = [0, CHANNEL_STATUS.enabled, CHANNEL_STATUS.manuallyDisabled, CHANNEL_STATUS.autoDisabled]

export function ChannelsPage() {
  const { t, i18n } = useTranslation()
  const locale = i18n.resolvedLanguage || i18n.language
  const [filters, setFilters] = useState<ChannelFilters>({ name: '', statusFilter: 0 })
  const [pending, setPending] = useState<ChannelRow | null>(null)
  const channels = useChannels(filters)
  const toggle = useToggleChannelStatus()

  if (channels.isPending) {
    return <MobileLoading />
  }
  if (channels.isError) {
    return <MobileError title={t('Load failed')} description={t('Retry later.')} />
  }

  const confirmToggle = async () => {
    if (!pending) {
      return
    }
    const target = channelToggleTarget(pending.status)
    try {
      await toggle.mutateAsync({ id: pending.id, status: target })
      toast.success(target === CHANNEL_STATUS.enabled ? t('Channel enabled') : t('Channel disabled'))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Load failed'))
    } finally {
      setPending(null)
    }
  }

  return (
    <div className='space-y-3 px-3 pb-4'>
      <div className='space-y-1'>
        <Label htmlFor='mobile-channel-search'>{t('Search channels')}</Label>
        <Input id='mobile-channel-search' value={filters.name} onChange={(event) => setFilters({ ...filters, name: event.target.value })} />
      </div>

      <div role='group' aria-label={t('Status')} className='flex flex-wrap gap-2'>
        {STATUS_FILTERS.map((status) => (
          <button
            key={status}
            type='button'
            aria-pressed={filters.statusFilter === status}
            onClick={() => setFilters({ ...filters, statusFilter: status })}
            className={filters.statusFilter === status ? 'text-foreground text-sm font-medium' : 'text-muted-foreground text-sm'}
          >
            {status === 0 ? t('All') : t(channelStatusLabelKey(status))}
          </button>
        ))}
      </div>

      {channels.data.items.length === 0 ? (
        <MobileEmpty title={t('No channels')} description={t('Nothing to show for this range.')} />
      ) : (
        <ul className='divide-y'>
          {channels.data.items.map((channel) => (
            <li key={channel.id} className='flex min-h-14 items-center gap-3'>
              <div className='min-w-0 flex-1'>
                <ValueRow
                  label={channel.name}
                  secondary={`#${channel.id} · ${channel.group} · ${t(channelStatusLabelKey(channel.status))}`}
                  value={formatQuotaWithCurrency(channel.balance, locale)}
                />
              </div>
              <Switch
                aria-label={`${t(channelStatusLabelKey(channel.status))} ${channel.name}`}
                checked={channel.status === CHANNEL_STATUS.enabled}
                onCheckedChange={() => setPending(channel)}
              />
            </li>
          ))}
        </ul>
      )}

      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPending(null)
          }
        }}
        title={pending && channelToggleTarget(pending.status) === CHANNEL_STATUS.enabled ? t('Enable channel') : t('Disable channel')}
        desc={pending && channelToggleTarget(pending.status) === CHANNEL_STATUS.enabled ? t('Enable this channel?') : t('Disable this channel? Traffic will stop routing to it.')}
        confirmText={pending && channelToggleTarget(pending.status) === CHANNEL_STATUS.enabled ? t('Enable channel') : t('Disable channel')}
        handleConfirm={confirmToggle}
      />
    </div>
  )
}
```

`web/src/mobile/app.tsx`：把兜底面板替换为 `{activeTab === 'channels' ? <ChannelsPage /> : null}`。

**Task 11 review 追加要求（控制器裁决）**：

1. `ConfirmDialog` 要传 `isLoading={toggle.isPending}`，避免写入进行中重复点击发出重复 POST（`@/components/confirm-dialog.tsx` 支持该 prop）。
2. 渠道列表查询加 `placeholderData: keepPreviousData`：搜索框每次按键都会换 queryKey，否则列表在输入期间被 `MobileLoading` 顶掉、输入体验碎裂。
3. `ChannelListResult` 的 `page`/`page_size` 设为可选：`/api/channel/search` 只返回 `items/total/type_counts`，没有分页字段。
4. 写入失败时对 401（`ApiError.code === 'unauthorized'`，`api-client.ts` 会 `clearPat()`）给专门文案（例如需要重新粘贴令牌），不要只给通用失败文案。
5. 测试补：状态 chip 的请求参数（`Enabled` 发 `status=1`、`Disabled` 发 `status=0`、`All` 省略 `status`）、写入成功后列表会重新拉取、写入进行中不能重复提交。

**`toast` 需要在手机端挂载 `<Toaster />`（否则 `toast.success`/`toast.error` 是静默空操作）**：在 `web/src/mobile/providers.tsx` 里从 **`sonner` 直接**引入并渲染：

```tsx
import { Toaster } from 'sonner'
// ...
return (
  <QueryClientProvider client={mobileQueryClient}>
    {props.children}
    <Toaster />
  </QueryClientProvider>
)
```

**不要**用 `@/components/ui/sonner`：那个包装层会引入 `@hugeicons/core-free-icons`、`@hugeicons/react` 与 `@/context/theme-provider`（桌面主题），把手机首屏拖大（Global Constraints 的禁用依赖口径）。

**手机端只能挂一个 `<Toaster />`（Task 11 review Important-1/2）**：`web/src/mobile/main.tsx` 从 Task 3 起就挂了 `@/components/ui/sonner` 的桌面包装 Toaster，它把 `@hugeicons/*`（图标 path 数据）与 `@/context/theme-provider` 带进首屏；本次又在 `providers.tsx` 挂了明文 `sonner` 的 Toaster。两个实例都会订阅全局 ToastState 并渲染同一个 toast（默认位置相同），成功/失败反馈会叠成两份、样式还不一致。因此：

- 删掉 `main.tsx` 里的 `import { Toaster } from '@/components/ui/sonner'` 与 `<Toaster />`（`main.tsx` 只保留 `MobileApp`/`MobileProviders`/`initializeMobileI18n` 的接线）；
- 只保留 `providers.tsx` 中从 `sonner` 直接引入的那个；
- 复测首屏时**不能用包名字符串**做判据（内联后 `@hugeicons` 字样会消失），要用符号/路径数据，例如 `grep -c 'HugeiconsIcon\|CheckmarkCircle02Icon\|theme-provider' mobile-dist/static/js/<首屏 chunk>` 必须为 0，并对比删除前后的 gzip 总量。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile && bun run typecheck && bunx oxlint -c .oxlintrc.json src/mobile`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add web/src/mobile/types.ts web/src/mobile/features/channels web/src/mobile/app.tsx web/src/mobile/providers.tsx
git commit -m "feat(web-mobile): add the channels tab with enable and disable controls"
```

---

## Task 12：i18n 补齐与校验

**Files:**
- Modify: `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`

**Interfaces:**
- Consumes: 前序任务代码中出现的全部 `t('...')` 字面量。
- Produces: 七个语言文件包含全部新增 key。

- [ ] **Step 1: 收集新增 key 并确认缺失**

先按项目 i18n 技能（Newapi 项目的 `i18n-translate` 技能）读取并遵循其流程，然后执行：

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web
bun run i18n:sync --help 2>/dev/null || node scripts/sync-i18n.mjs --help
rg -o "t\('([^']+)'\)" -r '$1' src/mobile | sort -u
```

Expected: 输出新增 key 列表，且 `bun run i18n:sync` 报告缺失 key。下表的预期集合必须与 `rg` 输出逐条对齐：若两者不一致，以 `rg` 输出为准修正本清单后再翻译（不得留下清单里有、代码里没有的 key）：

```text
Access token
Access token required
All
Auto disabled
Available
By model
By user
Channel disabled
Channel enabled
Channel switches
Channels
Configured weight
Configured weight unknown
Cost
Current rate
Degraded
Desktop version
Disable channel
Disable this channel? Traffic will stop routing to it.
Enable channel
Enable this channel?
Enabled
Healthy
Last 30 days
Last 60 seconds
Last 7 days
Latest failures
Latest probe
Limited
Load failed
Loading
Manually disabled
Mine
Mobile console
Model availability
Model monitoring is disabled on this instance.
Models by site
No channels
No recent requests
Nothing to show for this range.
Paste the access token you generated on the desktop console.
Recent requests
Requests
Retry later.
Routing statistics
Run requests, or widen the time window, to collect routing statistics.
Search channels
Sites
Status
Sticky sessions
Summary data updates every few minutes.
The token looks too short. Check that you copied all of it.
Time range
Today
Tokens
Unknown
Unknown status
Usage
Usage scope
Verify and continue
errors
keys
```

已在 `web/src/i18n/locales/en.json` 中存在的 key（例如 `Usage`、`Channels`、`Status`、`Enabled`、`All`、`Models by site` 等）保持不动，只补齐缺失项。

- [ ] **Step 2: 运行同步确认仍然失败（缺失项存在）**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run i18n:sync && git diff --stat src/i18n/locales`
Expected: 若 Step 1 已补齐则此步应无 diff；若仍有缺失，`i18n:sync` 会把这些 key 补进 `en.json` 并在输出中列出需要人工翻译的语言。以该输出为准继续补齐。

- [ ] **Step 3: 补齐七种语言的翻译**

按 i18n 技能规则翻译，写入各语言文件对应 key（值是该语言的展示文案，key 保持英文原文）。中文（`zh.json`）示例值：

```json
{
  "Mobile console": "手机控制台",
  "Access token required": "需要访问令牌",
  "Paste the access token you generated on the desktop console.": "粘贴你在桌面控制台生成的访问令牌。",
  "The token looks too short. Check that you copied all of it.": "令牌长度不足，请确认已完整复制。",
  "Verify and continue": "验证并继续",
  "Mine": "我的",
  "By model": "按模型",
  "By user": "按用户",
  "Current rate": "当前速率",
  "Last 60 seconds": "最近 60 秒",
  "Summary data updates every few minutes.": "汇总数据每几分钟更新一次。",
  "Recent requests": "最近请求",
  "No recent requests": "暂无请求",
  "Nothing to show for this range.": "该时间范围内没有数据。",
  "Model availability": "模型可用性",
  "Healthy": "正常",
  "Degraded": "降级",
  "Unavailable": "不可用",
  "Unknown": "未知",
  "Sites": "站点",
  "Latest probe": "最近探活",
  "Models by site": "站点模型",
  "Latest failures": "最近失败",
  "Model monitoring is disabled on this instance.": "本站未启用模型监控。",
  "Routing statistics": "路由统计",
  "Channel switches": "渠道切换",
  "Configured weight": "配置权重",
  "Configured weight unknown": "配置权重未知",
  "Sticky sessions": "粘滞会话",
  "Run requests, or widen the time window, to collect routing statistics.": "发起请求或扩大时间窗以收集路由统计。",
  "Search channels": "搜索渠道",
  "Disable channel": "禁用渠道",
  "Enable channel": "启用渠道",
  "Disable this channel? Traffic will stop routing to it.": "禁用该渠道？流量将不再路由到它。",
  "Enable this channel?": "启用该渠道？",
  "Channel enabled": "渠道已启用",
  "Channel disabled": "渠道已禁用",
  "Manually disabled": "手动禁用",
  "Auto disabled": "自动禁用",
  "Unknown status": "未知状态",
  "Time range": "时间范围",
  "Usage scope": "用量范围",
  "Today": "今天",
  "Last 7 days": "近 7 天",
  "Last 30 days": "近 30 天",
  "Requests": "请求数",
  "Tokens": "Token",
  "Cost": "花费",
  "Load failed": "加载失败",
  "Retry later.": "请稍后重试。",
  "No channels": "暂无渠道",
  "errors": "错误",
  "keys": "个键"
}
```

其余六种语言按同一 key 集合翻译。

- [ ] **Step 4: 运行校验确认通过**

Run:
```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web
bun run i18n:sync && git diff --exit-code src/i18n/locales
bun run test -- src/mobile && bun run typecheck && bunx oxlint -c .oxlintrc.json src/i18n src/mobile
```
Expected: `i18n:sync` 无进一步改动（`git diff --exit-code` 退出码 0），测试/typecheck/lint 全部通过。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add web/src/i18n/locales
git commit -m "i18n(web-mobile): translate the mobile console for all supported locales"
```

---

## Task 13：手机端交互细节（下拉刷新、深色、PWA 元数据）

**Files:**
- Create: `web/src/mobile/components/pull-to-refresh.tsx`、`web/src/mobile/components/__tests__/pull-to-refresh.test.tsx`、`web/public-mobile/manifest.webmanifest`、`web/src/mobile/__tests__/providers.test.tsx`
- Modify: `web/rsbuild.mobile.config.ts`、`web/src/mobile/index.html`、`web/src/mobile/app.tsx`、`web/src/mobile/styles/mobile.css`、`web/src/mobile/providers.tsx`

**Interfaces:**
- Produces: `<PullToRefresh onRefresh={() => Promise<void>} children />`——仅当滚动容器处于顶部且下拉位移超过 64px 时触发；`data-refreshing` 属性反映进行中状态。
- Produces: `MobileProviders` 内部加入 `ThemeProvider`，使手机端跟随系统深浅色并复用桌面的主题偏好；`web/src/mobile/__tests__/providers.test.tsx` 断言系统深色偏好下 `document.documentElement` 带 `dark` 类、浅色偏好下带 `light` 类。

- [ ] **Step 1: 写失败测试**

`web/src/mobile/components/__tests__/pull-to-refresh.test.tsx`：

```tsx
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { PullToRefresh } from '@/mobile/components/pull-to-refresh'

function touchStart(element: HTMLElement, clientY: number) {
  fireEvent.touchStart(element, { touches: [{ clientY }] })
}

describe('PullToRefresh', () => {
  it('refreshes after a long pull from the top', async () => {
    const onRefresh = vi.fn().mockResolvedValue(undefined)
    const { container } = render(
      <PullToRefresh onRefresh={onRefresh}>
        <p>content</p>
      </PullToRefresh>
    )
    const scroller = container.firstElementChild as HTMLElement
    Object.defineProperty(scroller, 'scrollTop', { value: 0, configurable: true })

    touchStart(scroller, 0)
    fireEvent.touchMove(scroller, { touches: [{ clientY: 120 }] })
    fireEvent.touchEnd(scroller)

    await waitFor(() => expect(onRefresh).toHaveBeenCalledTimes(1))
  })

  it('ignores a short pull', async () => {
    const onRefresh = vi.fn().mockResolvedValue(undefined)
    const { container } = render(
      <PullToRefresh onRefresh={onRefresh}>
        <p>content</p>
      </PullToRefresh>
    )
    const scroller = container.firstElementChild as HTMLElement
    Object.defineProperty(scroller, 'scrollTop', { value: 0, configurable: true })

    touchStart(scroller, 0)
    fireEvent.touchMove(scroller, { touches: [{ clientY: 20 }] })
    fireEvent.touchEnd(scroller)

    expect(onRefresh).not.toHaveBeenCalled()
  })

  it('ignores a pull that starts away from the top', () => {
    const onRefresh = vi.fn().mockResolvedValue(undefined)
    const { container } = render(
      <PullToRefresh onRefresh={onRefresh}>
        <p>content</p>
      </PullToRefresh>
    )
    const scroller = container.firstElementChild as HTMLElement
    Object.defineProperty(scroller, 'scrollTop', { value: 200, configurable: true })

    touchStart(scroller, 0)
    fireEvent.touchMove(scroller, { touches: [{ clientY: 120 }] })
    fireEvent.touchEnd(scroller)

    expect(onRefresh).not.toHaveBeenCalled()
  })
})
```

`web/src/mobile/__tests__/providers.test.tsx`：

```tsx
import { render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { mobileQueryClient } from '@/mobile/lib/query-client'
import { MobileProviders } from '@/mobile/providers'

// The shared test setup installs a matchMedia stub that only answers
// prefers-reduced-motion; this suite replaces it to drive the color scheme and
// restores the original descriptor afterwards.
const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')

function stubSystemColorScheme(prefersDark: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: (query: string): MediaQueryList => ({
      matches: query.includes('prefers-color-scheme: dark') ? prefersDark : query.includes('prefers-reduced-motion') && !query.includes('no-preference'),
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    }),
  })
}

describe('MobileProviders theme', () => {
  beforeEach(() => {
    window.localStorage.clear()
    mobileQueryClient.clear()
    mobileQueryClient.setQueryData(STATUS_QUERY_KEY, {})
    document.documentElement.classList.remove('light', 'dark')
  })

  afterEach(() => {
    if (originalMatchMedia) {
      Object.defineProperty(window, 'matchMedia', originalMatchMedia)
    }
    document.documentElement.classList.remove('light', 'dark')
  })

  it('applies the dark class when the system prefers dark', async () => {
    stubSystemColorScheme(true)

    render(<MobileProviders>{null}</MobileProviders>)

    await waitFor(() => expect(document.documentElement.classList.contains('dark')).toBe(true))
  })

  it('applies the light class when the system prefers light', async () => {
    stubSystemColorScheme(false)

    render(<MobileProviders>{null}</MobileProviders>)

    await waitFor(() => expect(document.documentElement.classList.contains('light')).toBe(true))
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web && bun run test -- src/mobile/components/__tests__/pull-to-refresh.test.tsx`
Expected: FAIL — 模块不存在。

- [ ] **Step 3: 写最小实现**

`web/src/mobile/components/pull-to-refresh.tsx`：

```tsx
import { type ReactNode, useRef, useState } from 'react'

const TRIGGER_DISTANCE = 64

interface PullToRefreshProps {
  onRefresh: () => Promise<void> | void
  children: ReactNode
}

export function PullToRefresh(props: PullToRefreshProps) {
  const startY = useRef<number | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  const handleTouchStart = (event: React.TouchEvent<HTMLDivElement>) => {
    const scroller = event.currentTarget
    startY.current = scroller.scrollTop <= 0 ? event.touches[0]?.clientY ?? null : null
  }

  const handleTouchEnd = async () => {
    const start = startY.current
    startY.current = null
    if (start === null || refreshing) {
      return
    }
    setRefreshing(true)
    try {
      await props.onRefresh()
    } finally {
      setRefreshing(false)
    }
  }

  const handleTouchMove = (event: React.TouchEvent<HTMLDivElement>) => {
    const start = startY.current
    if (start === null || refreshing) {
      return
    }
    if (event.touches[0]?.clientY !== undefined && event.touches[0].clientY - start > TRIGGER_DISTANCE) {
      // Mark the gesture as ready; the refresh itself runs on touch end so the
      // request is not started while the finger is still moving.
      startY.current = start
    }
  }

  return (
    <div
      className='min-h-dvh overflow-y-auto overscroll-contain'
      data-refreshing={refreshing}
      onTouchStart={handleTouchStart}
      onTouchMove={handleTouchMove}
      onTouchEnd={handleTouchEnd}
    >
      {props.children}
    </div>
  )
}
```

> `handleTouchEnd` 需要知道最后一次移动是否超过阈值。为避免测试与实现出现两套阈值判断，Step 3 的最终实现把阈值判断放在 `handleTouchEnd` 使用的 ref 上：

```tsx
export function PullToRefresh(props: PullToRefreshProps) {
  const startY = useRef<number | null>(null)
  const pulledFarEnough = useRef(false)
  const [refreshing, setRefreshing] = useState(false)

  const handleTouchStart = (event: React.TouchEvent<HTMLDivElement>) => {
    const atTop = event.currentTarget.scrollTop <= 0
    startY.current = atTop ? (event.touches[0]?.clientY ?? null) : null
    pulledFarEnough.current = false
  }

  const handleTouchMove = (event: React.TouchEvent<HTMLDivElement>) => {
    const start = startY.current
    const currentY = event.touches[0]?.clientY
    if (start === null || currentY === undefined || refreshing) {
      return
    }
    if (currentY - start > TRIGGER_DISTANCE) {
      pulledFarEnough.current = true
    }
  }

  const handleTouchEnd = async () => {
    const shouldRefresh = startY.current !== null && pulledFarEnough.current && !refreshing
    startY.current = null
    pulledFarEnough.current = false
    if (!shouldRefresh) {
      return
    }
    setRefreshing(true)
    try {
      await props.onRefresh()
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <div
      className='min-h-dvh overflow-y-auto overscroll-contain'
      data-refreshing={refreshing}
      onTouchStart={handleTouchStart}
      onTouchMove={handleTouchMove}
      onTouchEnd={handleTouchEnd}
    >
      {props.children}
    </div>
  )
}
```

`web/src/mobile/providers.tsx` 加入主题 Provider：

```tsx
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

import { ThemeProvider } from '@/context/theme-provider'
import { mobileQueryClient } from '@/mobile/lib/query-client'

interface MobileProvidersProps {
  children: ReactNode
}

export function MobileProviders(props: MobileProvidersProps) {
  return (
    <QueryClientProvider client={mobileQueryClient}>
      <ThemeProvider>{props.children}</ThemeProvider>
    </QueryClientProvider>
  )
}
```

`web/public-mobile/manifest.webmanifest`：

```json
{
  "name": "new-api",
  "short_name": "new-api",
  "start_url": "/m",
  "scope": "/m",
  "display": "standalone",
  "background_color": "#ffffff",
  "theme_color": "#ffffff"
}
```

`web/rsbuild.mobile.config.ts` 把公共静态目录指向手机端专用目录（该目录下的文件会在构建时被复制到产物根目录）：

```ts
    server: {
      host: '0.0.0.0',
      strictPort: false,
      base: '/m',
      publicDir: './public-mobile',
      proxy: { '/api': { target: serverUrl, changeOrigin: true } },
    },
    output: {
      minify: isProd,
      target: 'web',
      assetPrefix: '/m',
      distPath: { root: 'mobile-dist' },
    },
```

`web/src/mobile/index.html` 的 `<head>` 增加：

```html
    <link rel='manifest' href='/m/manifest.webmanifest' />
    <meta name='apple-mobile-web-app-capable' content='yes' />
    <meta name='apple-mobile-web-app-status-bar-style' content='default' />
```

`web/src/mobile/app.tsx`：用 `PullToRefresh` 包裹页面内容，`onRefresh` 调用 `mobileQueryClient.invalidateQueries({ queryKey: ['mobile'] })`；同时把"桌面版"入口写入页面底部（设置 cookie 后跳回 `/`）：

```tsx
  const openDesktopConsole = () => {
    document.cookie = 'newapi_prefer_desktop=1; path=/; max-age=31536000; samesite=lax'
    window.location.href = '/'
  }
```

在 `MobileTabBar` 上方渲染一个按钮：

```tsx
      <div className='px-3 pb-2'>
        <button type='button' className='text-muted-foreground text-xs underline' onClick={openDesktopConsole}>
          {t('Desktop version')}
        </button>
      </div>
```

`web/src/mobile/styles/mobile.css` 追加：

```css
@media (display-mode: standalone) {
  /* Installed consoles have no browser chrome, so the top inset must come from the header itself. */
  header {
    padding-top: max(1rem, env(safe-area-inset-top, 0px));
  }
}
```

- [ ] **Step 4: 运行测试与构建确认通过**

Run:
```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web
bun run test -- src/mobile
bun run typecheck && bunx oxlint -c .oxlintrc.json src/mobile && bun run format:check
bun run build:mobile && ls mobile-dist/manifest.webmanifest
```
Expected: 测试 PASS；`manifest.webmanifest` 出现在 `mobile-dist` 根下；format 检查通过。若 `manifest.webmanifest` 缺失，说明 `server.publicDir` 未按预期复制该目录，此时改为在 `output.copy` 中声明 `{ from: './public-mobile', to: '.' }` 并把结果重新记录到本任务。

- [ ] **Step 5: 提交**

```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
git add web/public-mobile web/rsbuild.mobile.config.ts web/src/mobile
git commit -m "feat(web-mobile): add pull to refresh, standalone metadata and a desktop escape hatch"
```

---

## Task 14：全量验证与人工冒烟

**Files:**
- 无代码改动；产出验证记录（写入本计划文件的新增小节，见 Step 4）。

**Interfaces:**
- Consumes: 前序全部任务。
- Produces: 可交给发布流程的验证证据。

- [ ] **Step 1: 运行前端全量检查**

Run:
```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui/web
bun run test
bun run typecheck
bun run lint
bun run format:check
bun run copyright:check 2>&1 | grep -E 'src/mobile|rsbuild\.mobile' && exit 1 || true
bun run i18n:sync && git diff --exit-code src/i18n/locales
bun run build:mobile
du -sh mobile-dist && find mobile-dist -name '*.js' -exec du -h {} + | sort -h | tail -5
# No forbidden dependency may reach the first-screen chunks.
for f in $(grep -o '/m/static/js/[^"]*\.js' mobile-dist/index.html); do
  if grep -qE 'react-router|motion/react|lucide-react|axios' "mobile-dist${f#/m}"; then echo "FORBIDDEN DEP IN FIRST SCREEN: $f"; fi
done
```
Expected: 全部通过；把 `du` 输出记录进 Step 4。

- [ ] **Step 2: 运行后端全量检查**

Run:
```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
# go vet/go build need both embed targets to exist; web/dist is gitignored.
mkdir -p web/dist && [ -f web/dist/index.html ] || cp web/mobile-dist/index.html web/dist/index.html
gofmt -l router main.go
go vet ./router/ ./...
go build ./...
go test ./router/ -v
```
Expected: `gofmt -l` 无输出；`go vet`/`go build` 无错误；`go test ./router/` 全部 PASS。

- [ ] **Step 3: 本地人工冒烟**

Run（在本地启动后端，桌面与手机 UA 各测一遍）：
```bash
cd /home/ra/orca/workspaces/Newapi/codex-mobile-admin-webui
mkdir -p web/dist && [ -f web/dist/index.html ] || cp web/mobile-dist/index.html web/dist/index.html
go run . &
sleep 5
curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' -H 'User-Agent: Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148' http://localhost:3000/
curl -s -o /dev/null -w '%{http_code}\n' -H 'User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0.0.0 Safari/537.36' http://localhost:3000/
curl -s -o /dev/null -w '%{http_code}\n' -H 'User-Agent: Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Mobile/15E148' http://localhost:3000/m/routing
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:3000/m/static/../index.html
```
Expected: 第一条 `302 /m`（或 `Found` + `/m`）；第二条 `200`；第三条 `200`（SPA 回落）；第四条 `200` 且返回手机 index（不得泄露 `web/dist` 内容）。

浏览器（Chrome DevTools 切换设备模拟）逐项确认：
1. 首次打开 `/m` 显示令牌输入页；粘贴一个真实 PAT 后进入控制台，刷新页面仍保持登录。
2. 吊销该 PAT（桌面端撤销）后下拉刷新，出现"令牌失效"提示并回到令牌输入页。
3. 用量页「我的/全站」与三个时间范围切换都能返回数据；汇总区显示"每几分钟更新"提示。
4. 模型可用性页显示站点健康计数与异常模型。
5. 路由统计页显示每个模型的实测占比条，与配置权重并排。
6. 渠道页搜索到目标渠道，点击开关弹出确认框，确认后渠道状态在桌面端一致变化。
7. 点击底部"桌面版"链接回到桌面 dashboard，刷新不再被跳回 `/m`。

- [ ] **Step 4: 记录验证结果并提交**

在本文件末尾追加小节（记录实际命令与实际输出，不得写"已通过"而无输出）：

```markdown
## 附录 A. 验证记录（追加到本文件末尾）

- 前端：`bun run test` → <粘贴摘要>；`bun run build:mobile` → `mobile-dist` 体积 <粘贴 du 输出>
- 后端：`go test ./router/ -v` → <粘贴 PASS 摘要>；`go build ./...` → <粘贴结果>
- 冒烟：<粘贴 curl 输出与浏览器逐项结论>
- 未完成/阻塞：<逐条列出，无则写"无">
```

```bash
git add docs/plans/mobile-admin-webui-tdd-plan.md
git commit -m "docs(web-mobile): record the mobile console verification evidence"
```

---

## 17. 测试矩阵与运行命令

| 层 | 命令 | 覆盖 |
| --- | --- | --- |
| Go 路由 | `go test ./router/ -run 'TestIsMobileUserAgent|TestDesktopShell|TestMobileShell' -v` | UA 判定表、根路径重定向、`?desktop=1`/cookie 例外、深链不跳转、`/api` 404、`/m` 静态与回落、index 的 no-cache |
| 前端纯逻辑 | `bun run test -- src/mobile/lib src/mobile/features` | PAT 校验与持久化、fetch 封装与错误码、时间范围、聚合与排行、可用性排序、实测/配置占比、渠道状态映射 |
| 前端组件 | `bun run test -- src/mobile/components` | Tab 栏 ARIA 与选中态、KPI 卡格式化、PAT 门校验与错误提示、MiniBar 比例、下拉刷新阈值与顶部判定 |
| 前端页面 | `bun run test -- "src/mobile/features/*/components/__tests__"` | 四个页签的加载/空/错误/成功路径与请求参数 |
| 静态检查 | `bun run typecheck && bun run lint && bun run format:check && bun run knip`；`bun run copyright:check 2>&1 \| grep -E 'src/mobile\|rsbuild\.mobile'` 必须为空 | 类型、lint（含 `project/intl-locale`）、格式、未使用导出；版权头只需覆盖本次新增文件（基线有 25 个既有文件失败，与本计划无关） |
| i18n | `bun run i18n:sync && git diff --exit-code src/i18n/locales` | 七个语言文件 key 完整且无待同步项 |
| 构建 | `bun run build:mobile && go build ./...` | 手机产物体积与 embed 装配 |

## 18. 风险与未决项

1. **`/api/data/*` 有分钟级延迟**：KPI 与排行来自 `quota_data` 汇总表。UI 已用固定文案说明；如果实际延迟影响使用，后续可改为直接聚合 `logs` 表（需要新的后端聚合接口，超出 v1 范围）。
2. **分组维度缺失**：`quota_data` 聚合不含 `use_group`，用量页不提供按分组排行。
3. **iPadOS 13+ 不跳转**：UA 与桌面 Safari 相同，需要手动访问 `/m`。
4. **PAT 无 SessionID**：本 UI 不提供会话管理、安全验证类操作；PAT 轮换后需在手机上重新粘贴。
5. **手机端不注入 Umami/GA**：`main.go` 的注入函数只改写桌面 `indexPage`；如需统计手机端访问，需要单点扩展 `InjectUmamiAnalytics`/`InjectGoogleAnalytics` 同时处理 `mobileIndexPage`。
6. **`FRONTEND_BASE_URL` 不为空时**：`SetWebRouter` 不会被调用，`/m` 随整个前端一起重定向到外部地址；这是现有部署语义，v1 不改变。
7. **首屏体积目标**：若 `bun run build:mobile` 后的首屏 JS（gzip）超过 120 KB，优先检查是否误引入了 `@/components/ui` 中带重依赖的组件（如 `chart.tsx`、`markdown.tsx`）。
