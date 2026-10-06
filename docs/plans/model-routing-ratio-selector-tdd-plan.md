# 模型路由按模型比例切换（预设作用域 + 自定义模式）TDD 计划

状态：**本文件只是计划**。本轮未实施代码、未提交、未推送、未部署、未接触生产、未调用真实模型 API。

文件位置说明：用户明确要求先写 TDD 计划；仓库既有 TDD 计划位于 `docs/plans/`（`virtual-pool-sticky-session-tdd-plan.md`、`mobile-admin-webui-tdd-plan.md`），本文件沿用该约定。`docs/plans` 在本工作树**未被** `.gitignore` 忽略（已用 `git check-ignore` 核实），两个既有计划均为已跟踪文件。

## 1. 目标

在「系统设置 → 请求策略 → 模型路由」（`/system-settings/request-policies/model-weights`）里，把路由比例切换精确到**单个模型**：

1. 每个模型一个「比例」选择器，选项 = 该模型可用的预设 + `自定义`。
2. 选预设 → **只替换该模型的覆盖行**，其他模型的行不动 → 一次 `PATCH /api/option/request_policy` 写 `model_weight_setting.weights` → 热生效（无需重启）。
3. 选 `自定义` → 打开该模型的比例输入（按有效优先级分层，按百分比填写），确认后归一化成整数 weight 后写库。
4. 每个模型组按有效优先级分层显示占比（%），让「7:3」这种比例可见。
5. 移动端控制台（`web/src/mobile`）新增同一套「按模型切换比例」的**写**能力（切换预设 + 自定义比例），不再只有只读统计。

## 2. 非目标（本版不做）

- 不改后端候选构造与加权算法（`service/channel_select.go`、`model/channel_cache.go`）。
- 不改计费、不改虚拟池路由（`model_retry_policy_setting.virtual_model_routes`）、不改渠道级 priority/weight。
- 不新增数据库表、字段或迁移；`model_weight_setting` 仍是一个 JSON 字符串 option。
- 不引入新的前端依赖；不新增 `docs/` 之外的文件。
- 不改移动端既有的只读统计页（`web/src/mobile/features/routing`）；移动端新增的是独立设置页。
- 不做预设的审计日志、版本历史、并发锁、审批流。
- 不做「按用户/分组作用域」的预设。

## 3. 基线

- 工作树：`/home/ra/orca/workspaces/Newapi/per-model-channel-weight`
- 分支与 HEAD：`prod/251`，`12d8ad4e0`（工作区干净）
- 相关功能来自 `9368fe5aa feat(routing): add one-click per-model routing presets`
- 工具链：系统 `go 1.27.1`（`go.mod` 要求 1.25.1）、`bun 1.4.2`、`node 24.19.0`。本工作树**没有** `.local-tools/`，使用系统工具链。
- 本轮已执行的基线（均通过，详见第 9 节）：
  - `go test -count=1 ./setting/operation_setting ./model`
  - `cd web && bun install`（1228 packages）
  - `cd web && bunx vitest run src/features/system-settings/models`

## 4. 现状证据

行号对应本工作树 HEAD。

| 位置 | 当前行为 | 对本计划的影响 |
| --- | --- | --- |
| `web/src/features/system-settings/request-policies/section-registry.tsx:96-107` | 模型路由是 `id: 'model-weights'` 的 section，渲染 `ModelWeightSection` | 改动集中在这一节 |
| `web/src/features/system-settings/models/model-weight-section.tsx:162-178` | 行状态是**平铺数组** `ModelWeightRow[]`（channel_id, model, weight, priority），用 `props.defaultValues` 的 `useEffect` 重置 | 需要按模型分组渲染；行状态仍是同一个数组，分组只是视图 |
| `model-weight-section.tsx:238-246` | 保存 = 序列化整表 → `savePolicy.mutateAsync({'model_weight_setting.weights': ...})`，与 `baselineRef` 比较去重 | 单模型应用复用同一条写路径，只是先做合并 |
| `model-weight-section.tsx:266-269` | 把 `presets` 与 `serializeModelWeights(rows)`（**未保存的编辑行**）传给预设栏 | 选中态基于编辑行而非生效行，改数字就会跳到「自定义」；需要修正 |
| `web/src/features/system-settings/models/model-weight-preset-bar.tsx:58-66` | `activeName = findActivePresetName(presets, currentWeights)`；应用预设 = 写 `serializePresetWeights(preset)`（整表替换） | 全局语义保留；新增按模型的作用域与合并 |
| `model-weight-preset-bar.tsx:68-96` | 保存当前为预设 = 快照**整表**行；删除按 `name` 匹配 | 需要引入作用域，保存/删除按 (scope, name) |
| `web/src/features/system-settings/models/model-weight-presets.ts:35-38,92-105,158-167` | `ModelWeightPreset = {name, weights}`；`canonicalKey` 做顺序/大小写无关比较；`findActivePresetName` 整表比较 | 作用域字段与按模型比较的落点 |
| `setting/operation_setting/model_weight_setting.go:38-42` | `ModelWeightPreset{Name, Weights}` | 新增可选 `Model` 字段 |
| `model_weight_setting.go:167-200` | `ValidateModelWeightPresets`：数组、≤50 个、名字非空且 ≤64 字节、名字大小写不敏感唯一、条目复用 `validateModelWeightEntries` | 需要新增作用域校验与 (scope, name) 唯一性 |
| `model_weight_setting.go:202-243` | `validateModelWeightEntries`：channel_id>0、model 非空 ≤255、weight ≤1e6、priority ≥0 ≤1e9、(channel, model) 去重、每行必须有 weight 或 priority | 单模型预设的条目必须全部属于该模型，用同一套条目规则 |
| `model_weight_setting.go:66-100` | `modelWeightIndex()` 按 `modelWeightSetting.Weights` 原始字符串缓存，写 option 后自动失效 | 热生效的机制来源，不需要额外失效动作 |
| `controller/option.go:404-419` | `model_weight_setting.weights` / `.presets` 的写入前校验 | 单模型合并后的整表写仍走同一校验，不需要新端点 |
| `model/channel_cache.go:355-367` | 候选按 priority 降序、weight 降序、id 升序排列 | 比例只在同一 priority 内分摊 |
| `model/channel_cache.go:380-434` | `SelectSatisfiedChannelFromCandidates`：先取 `targetPriority`（priority 档），**只在同档内**按 weight 做加权随机；`sumWeight==0` 时退化为均分 | 占比必须按「模型 × 优先级档」分层计算；weight=0 表示不参与 |
| `setting/operation_setting/model_weight_setting_test.go` | 已有 `TestValidateModelWeightPresets`、`TestMaxModelWeightPresetNameLength` | 后端测试扩这里，不新建文件 |
| `model/request_policy_test.go:95-100,159-167` | 已有预设往返与非法条目用例 | 加一条作用域往返用例 |
| `web/src/features/system-settings/models/__tests__/model-weight-presets.test.ts`、`model-weight-section.test.tsx` | 纯函数与组件测试已存在（184 / 317 行） | 前端测试扩这两个文件，不新建文件 |
| `web/src/features/channels/types.ts:89` | 渠道 schema 有 `priority: z.number().nullish()` | 计算有效优先级/有效权重时可取渠道值 |
| `web/src/lib/format.ts:53-59` | `formatPercent(value)` 接受 0–100 的数值 | 占比展示直接复用 |

## 5. 契约

### 5.1 预设作用域（后端）

```go
type ModelWeightPreset struct {
	Name    string                `json:"name"`
	Model   string                `json:"model,omitempty"` // 新增：空 = 全局预设
	Weights []ModelWeightOverride `json:"weights"`
}
```

- `model` 缺省或仅空白 = **全局预设**：语义与现在完全一致（整表替换），旧数据无需迁移。
- `model` 非空 = **单模型预设**：只能作用于该模型的覆盖行，且 `weights` 里每一行的 `model` 必须与该作用域**大小写不敏感**相等。
- 规范化：作用域与条目 model 都按 `strings.TrimSpace` 去空白；比较时小写。
- 唯一性键从 `lower(name)` 改为 `lower(scope) + "|" + lower(name)`，`scope` 为规范化后的 `model`（全局为 `""`）。因此「均衡@全局」与「均衡@deepseek-v4.1-flash」可以共存。
- 上限不变：≤50 个预设、名字 ≤64 字节、作用域 model ≤255 字节、条目规则复用 `validateModelWeightEntries`。
- 错误信息必须点名作用域，例如：
  - `model weight preset %q entry %d targets model %q outside its scope`
  - `duplicate model weight preset name %q for scope %q`
  - `model weight preset %d scope is longer than 255 bytes`

### 5.2 前端类型与解析

```ts
export type ModelWeightPreset = {
  name: string
  /** 空/缺省 = 全局预设（整表）；非空 = 只作用于该模型。 */
  model?: string
  weights: ModelWeightOverrideEntry[]
}
```

- `parseModelWeightPresets`：读取 `model`，trim；空白视为全局；`model` 非字符串时按缺失处理（不丢整条预设）。
- `serializeModelWeightPresets`：`model` 为空时**不输出 `model` 键**，保证旧全局预设序列化后字节不变（避免无意义 diff 与测试抖动）。

### 5.3 合并语义

```ts
applyPresetToModel(currentWeightsRaw: string, preset: ModelWeightPreset): string
```

- 全局预设：结果 = `preset.weights`（整表替换，与现状一致）。
- 单模型预设：
  1. 取 `others` = 当前行中 model 不等于作用域的行，保持原相对顺序；
  2. 取该模型在 `others` **之前**的原始索引 `firstIndex`；
  3. 若 `firstIndex` 存在：把预设行按顺序插回该位置；否则追加到末尾。
- 结果必须是合法 `model_weight_setting.weights`：不产生重复 `(channel_id, model)`，每行有 weight 或 priority。若预设本身不合法（历史脏数据），合并前先丢弃该预设并在 UI 给出提示，不允许写出被后端拒绝的整表。

### 5.4 选中态

- 全局栏：`findActivePresetName` 保持不变，但**只传全局预设**（`model` 为空）。
- 单模型：新增 `findActiveScopedPresetName(presets, model, weightsRaw)`：
  - 只在 `preset.model` 与该模型大小写不敏感相等的预设里找；
  - 只比较 `weightsRaw` 中属于该模型的行（顺序与大小写无关，复用 `canonicalKey`）；
  - 无匹配返回 `null` → 选择器显示 `自定义`。
- 选择器显示的是**生效行**推导出的状态，不再使用编辑行；编辑行与生效行不一致时另给「未保存」标记（沿用现有 `baselineRef` 比较）。

### 5.5 自定义比例归一化

```ts
normalizeRatioWeights(percents: number[]): number[]
```

- 输入为该模型**同一优先级档内**各行的百分比（0–100）。
- 输出整数 weight：`percent <= 0 → 0`（该渠道不参与）；`percent > 0 → max(1, Math.round(percent))`（避免 0.4% 被四舍五入成「排除」）。
- 只做分层内的归一化；不同优先级档各自独立，不跨档摊分。
- 上下限：结果必须 ≤ `MAX_MODEL_WEIGHT_VALUE`（1e6）且为整数。
- 确认写入时：只改 weight，不动 priority；未列入该档的行不产生新覆盖。
- 取整会带来 <1% 的偏差，必须在 UI 文案与文档里写明「按整数权重近似」。

### 5.6 占比展示

- 分层键 = `(model, effectivePriority)`，`effectivePriority = row.priority ?? channel.priority ?? 0`（渠道值来自已有的 `getChannels` 查询）。
- `effectiveWeight = row.weight ?? channel.weight ?? 未知`。
- 该档内若所有行都能确定 `effectiveWeight` 且 `sum > 0`：占比 = `formatPercent(Math.round(w / sum * 100))`（复用 `@/lib/format`）。
- `sum == 0`、或该档存在无法确定的 `effectiveWeight`（渠道列表加载失败 / 渠道不存在）：该档显示 `-`，并在 section 内给一条说明，**不猜数字**。
- 占比是展示信息，不参与写入。

### 5.7 UI 结构与组件复用

```
[全局] 快速切换  预设A  预设B  管理预设            ← 只列全局预设，语义不变

deepseek-v4.1-flash                 比例 [订阅优先 ▾]      ← 单模型预设 + 自定义
  优先级 501
    #9    输入订阅   weight 700  70%
    #36   ollama     weight 300  30%
glm-4.6                             比例 [自定义 ▾]
  优先级 —
    #9    输入订阅   weight 500  100%
```

- 行按模型分组渲染；组顺序 = 该模型在行数组中首次出现的顺序；组内按有效优先级降序、再按渠道 ID 升序。
- 每组的「比例」用 `@/components/ui/select`（本节已在用），选项 = 该模型的单模型预设 + `自定义`。
- 选预设 → `@/components/confirm-dialog` 确认（文案说明「只替换 <model> 的覆盖行」）→ 合并 → PATCH。
- 选 `自定义` → `@/components/dialog` 打开比例输入；确认后归一化 → 合并 → PATCH；**取消不写库**。
- 「管理预设」对话框新增作用域：保存时可选择 `全局（所有模型）` 或 `仅 <模型>`（模型列表来自当前编辑行去重）；列表项显示 `名称 · 作用域`；删除按 (scope, name)。
- 组件复用遵守 `web/AGENTS.md` 3.3：只组合 `Select` / `Dialog` / `ConfirmDialog` / `Input` / `Button` / `formatPercent`，不新增同类通用控件。

### 5.8 移动端设置页（新增）

- 入口：`web/src/mobile/types.ts` 的 `MobileTab` 增加 `routing-weights`，`MOBILE_TABS` 末尾追加；`mobile-tab-bar.tsx` 的 `TAB_LABEL_KEY` 增加标签 `Model routing`；`app.tsx` 渲染新页面。
- 数据：`GET /api/option/request_policy` 读 `model_weight_setting.weights` 与 `model_weight_setting.presets`；写入用 `PATCH /api/option/request_policy`（后端 `router/api-router.go:214-215`，`RootAuth`）。
- `web/src/mobile/lib/api-client.ts` 新增 `mobileApiPatch`，复用同一个 `request()`（保留 401/403/业务错误映射）。
- 交互与桌面一致：每个模型一个比例选择器（该模型的单模型预设 + `自定义`）；选预设 → 确认 → 合并 → PATCH；选 `自定义` → 按优先级档填百分比 → 归一化 → 合并 → PATCH。
- 移动端**不导入桌面 UI 组件**，只导入纯函数模块 `@/features/system-settings/models/model-weight-presets`（该模块零依赖）；行渲染用移动端既有组件与 `@/components/ui/*` 基础控件。
- 渠道名称/权重：复用 `web/src/mobile/features/channels/api.ts` 的查询（`ChannelRow` 有 `priority`、`weight`）；查不到的渠道按 `#<id>` 降级，占比同样降级为 `-`。
- 权限失败沿用 `mobileErrorCopy` 的既有文案（403 = 需要管理员令牌），不做静默失败。
- 移动端首屏预算不受影响：新增页面只在选中该 tab 时渲染。

## 6. 分阶段 TDD

依赖 P1 → P2 → P3 → P4 → P5。每阶段先写**行为不满足**的红用例（不能只是编译失败），再写最小实现，最后回归。建议每阶段一个提交，提交信息用 Conventional Commits。

| 阶段/建议提交 | Red：先失败的行为 | Green：实现边界 | Refactor/验收 |
| --- | --- | --- | --- |
| **P1** `feat(routing): scope model weight presets to one model` | 后端：带 `model` 的作用域预设当前被当作普通字段忽略；越界条目（作用域 A、条目 model B）当前被接受；同名不同作用域当前被判重复；作用域超长当前被接受。 | `setting/operation_setting/model_weight_setting.go`：新增 `Model` 字段、作用域规范化、越界校验、(scope,name) 唯一性、长度上限；`controller/option.go` 无需改动（复用同一校验函数）。 | 扩展 `model_weight_setting_test.go`（新增 `TestValidateModelWeightPresetsScope`）与 `model/request_policy_test.go` 的既有用例；旧全局预设 JSON 必须仍全部通过。 |
| **P2** `feat(routing): merge scoped presets per model` | 前端纯函数：解析丢弃 `model`；序列化给全局预设输出空 `model` 键；按模型合并会动到其他模型的行或改变其顺序；无匹配时选中态错误。 | `web/src/features/system-settings/models/model-weight-presets.ts`：类型、解析、序列化、`applyPresetToModel`、`findActiveScopedPresetName`、`presetsForModel`、`normalizeRatioWeights`。 | 扩展 `__tests__/model-weight-presets.test.ts`；纯函数测试不渲染组件。 |
| **P3** `feat(routing): switch the routing ratio per model` | 组件：现在没有模型分组、没有单模型选择器、选择器跟随未保存编辑行、应用预设会整表替换。 | `model-weight-section.tsx`（分组渲染 + 选择器 + 未保存标记）、`model-weight-preset-bar.tsx`（全局栏只列全局预设、管理对话框加作用域）。 | 扩展 `__tests__/model-weight-section.test.tsx`；断言 PATCH 的 payload 与调用次数。 |
| **P4** `feat(routing): show the tier share and edit a custom ratio` | 组件：没有占比展示；`自定义` 选项没有输入入口；百分比未归一化。 | 占比计算与展示、`自定义` 比例对话框（按档输入百分比、校验、归一化后走同一写路径）。 | 扩展 `__tests__/model-weight-section.test.tsx`；覆盖两档、sum=0、渠道列表失败降级。 |
| **P5** `chore(i18n): translate the routing ratio copy` | `bun run i18n:sync` 报告缺失键 / 未翻译键。 | 按 `i18n-translate` 技能：用 `web/scripts/add-missing-keys.mjs` 写 7 个语言，再 `bun run i18n:sync`；临时脚本用完删除。 | 第 8 节键表；`node scripts/find-missing-keys.mjs` 必须报「All t() keys found」。 |
| **P6** `feat(mobile): switch the routing ratio per model on mobile` | 移动端：只有只读统计页；`api-client` 没有 PATCH；没有设置入口。 | 新增 `web/src/mobile/features/routing-weights/**`（页面、api、lib）、`mobileApiPatch`、`MobileTab`/`MOBILE_TABS`/tab 文案/`app.tsx` 接线。 | `bunx vitest run src/mobile` + `bun run typecheck`；覆盖切换预设写库、自定义比例归一化、403 降级。 |

## 7. 确定性测试矩阵

后端新增/重写测试用 `testify/require` 做初始化与致命断言、`assert` 做值检查。前端用 Vitest + React Testing Library，查询用 role/label/aria，不用语言文案做脆弱断言；`api.patch` 只 mock 边界。

| ID | 输入 | 必须断言 | 归属 |
| --- | --- | --- | --- |
| T01 | 旧格式预设（无 `model`） | 校验通过；序列化后不含 `model` 键；全局栏仍显示 | P1/P2 |
| T02 | 作用域预设，条目 model 与作用域大小写/空白不同 | 校验通过；视为同一作用域 | P1 |
| T03 | 作用域 A，条目 model = B | 校验失败，错误信息点名作用域 | P1 |
| T04 | 同名预设，作用域分别为全局与某模型 | 校验通过；两者可共存 | P1 |
| T05 | 同作用域同名（大小写/空白不同） | 校验失败 | P1 |
| T06 | 作用域超 255 字节；预设数 > 50；名字 > 64 字节 | 校验失败，边界值本身通过 | P1 |
| T07 | `BuildRequestPolicy` / `UpdateRequestPolicyOptions` 写入作用域预设 | 往返一致；越界条目被拒且配置快照不变 | P1 |
| T08 | 合并：表内有 3 个模型的行，应用作用于中间模型的预设 | 其他模型行**内容与顺序不变**；目标模型行替换到原首行位置；无重复 (channel_id, model) | P2 |
| T09 | 合并：目标模型此前没有任何行 | 预设行追加到末尾 | P2 |
| T10 | 合并：全局预设 | 整表替换（与现状一致） | P2 |
| T11 | 选中态：目标模型行与预设一致但其他模型不同 | 显示该预设名；其他模型不参与比较 | P2 |
| T12 | 选中态：行顺序打乱、model 大小写不同 | 仍匹配同一预设 | P2 |
| T13 | 归一化：70 / 30 | → 70 / 30 | P2 |
| T14 | 归一化：0.4 / 99.6；0 / 100 | 前者 → 1 / 100（正数不归零）；后者 → 0 / 100 | P2 |
| T15 | 归一化：负数、NaN、>100 | 被拒绝或裁剪为合法值，不写出非法 weight | P2 |
| T16 | UI：一行含两个模型 | 渲染两个组，每组一个「比例」选择器，选择器可访问名称含模型名 | P3 |
| T17 | UI：在某组选一个预设并确认 | 只 PATCH 一次，payload 的 `model_weight_setting.weights` 只改该模型的行 | P3 |
| T18 | UI：选 `自定义` | 打开比例对话框，**未确认前不 PATCH**；取消后不 PATCH | P3/P4 |
| T19 | UI：确认自定义比例 | PATCH 的 weight 为归一化后的整数；priority 不变 | P4 |
| T20 | UI：占比 | 同档 700/300 → 70%/30%；两档各自计算不跨档；sum=0 → `-` | P4 |
| T21 | UI：渠道列表查询失败 | 不显示占比（`-`）与说明，不崩溃、不猜数字 | P4 |
| T22 | UI：编辑行与生效行不一致 | 显示「未保存」标记，选择器仍按生效行显示 | P3 |
| T23 | i18n | 7 个语言都含新增键；`find-missing-keys.mjs` 无输出 | P5 |
| T24 | 移动端：切换某模型的预设 | 只 PATCH 一次 `/api/option/request_policy`，payload 只改该模型的行 | P6 |
| T25 | 移动端：自定义比例确认 | 写入归一化整数 weight；取消不 PATCH | P6 |
| T26 | 移动端：RootAuth 403 | 显示既有管理员权限文案，不静默、不重试 | P6 |
| T27 | 移动端：渠道列表缺失 | 渠道名降级为 `#<id>`，占比显示 `-` | P6 |
| T28 | 移动端：tab 注册 | `#/routing-weights` 可进入；切换 tab 后新页面渲染 | P6 |

## 8. 拟定新增 i18n 键

最终文案在 P5 按 `i18n-translate` 技能逐语言重写，此处只定「需要哪些语义」。键名以 `t()` 字面量为准。

| 拟定键（英文源文案） | 用途 |
| --- | --- |
| `Routing ratio` | 每组比例选择器的可访问名称 |
| `Custom ratio` | 选择器里的自定义选项 |
| `Apply preset to this model?` | 单模型应用确认标题 |
| `This replaces the overrides of {{model}} only.` | 确认说明 |
| `Preset scope` | 管理对话框的作用域字段 |
| `Global (all models)` | 作用域选项 |
| `Only {{model}}` | 作用域选项 |
| `Share` | 占比列头 |
| `The share is computed inside one priority tier.` | 占比说明 |
| `Set the ratio as percentages. They are stored as integer weights.` | 自定义比例说明 |
| `Enter a ratio greater than 0.` | 自定义比例校验 |
| `Total` | 自定义比例合计 |

## 9. 运行命令与前置条件

全部在本地 Linux 文件系统执行，日志写 `/tmp/`，命令带显式墙钟上限。

前置：`cd web && bun install`（本工作树当前无 `web/node_modules`，本轮已执行成功）。

```bash
# 后端（P1）
timeout 300s go test -count=1 -timeout 240s ./setting/operation_setting ./model > /tmp/mrw-go.log 2>&1

# 前端受影响文件（P2–P4）
cd web
timeout 600s bunx vitest run src/features/system-settings/models > /tmp/mrw-web.log 2>&1

# 前端静态检查（P3–P5）
timeout 600s bun run typecheck > /tmp/mrw-typecheck.log 2>&1
timeout 600s bun run lint > /tmp/mrw-lint.log 2>&1
timeout 600s bun run format:check > /tmp/mrw-format.log 2>&1

# i18n（P5）
cd web
node scripts/add-missing-keys.mjs
node scripts/find-missing-keys.mjs
timeout 300s bun run i18n:sync > /tmp/mrw-i18n.log 2>&1
```

- 不跑 `bun run build` 作为本轮验收：前端生产构建与 embed 只在需要发布候选时执行。
- 不涉及数据库行为变更，因此**不需要** SQLite/MySQL/PostgreSQL 三库矩阵；`model_weight_setting` 仍是字符串 option，无 schema 变更。
- 不涉及 `relaykit/`，无需 `GOWORK=off` 构建。

## 10. 风险与回滚

| 风险 | 处理 |
| --- | --- |
| 旧全局预设被误判或丢失 | P1 用旧 JSON 全量回归；序列化对全局预设不输出 `model` 键 |
| 单模型预设的条目越界（历史脏数据） | 后端拒绝写入；前端合并前先过滤，并提示该预设不可用 |
| 应用单模型预设会删除该模型「不在预设里」的行 | 这是有意的整段替换语义；确认弹窗必须写明范围；测试 T08 固定该行为 |
| 先应用全局预设、再应用单模型预设 | 后者覆盖前者，属预期；文档写明顺序 |
| 同名不同作用域导致误删 | 管理对话框必须显示作用域，删除按 (scope, name)；测试 T04 |
| 百分比取整偏差 | 文案写明「按整数权重近似」；不承诺精确到小数 |
| 占比在渠道数据缺失时误导 | 明确降级为 `-` + 说明（T21），不猜数字 |
| 两个管理员并发切换 | 沿用现状：后写覆盖，无审计。本版不引入锁；在文档写明 |
| i18n 漏语言 | 只能通过 `add-missing-keys.mjs` + `i18n:sync`；P5 收尾必须跑 `find-missing-keys.mjs` |

回滚：本次改动只影响一个字符串 option 的**读取与校验**，以及前端页面。回滚二进制后，带 `model` 字段的预设会被旧版本当作未知字段忽略（全局预设仍可用），不需要数据恢复；若需要恢复旧行为，把前端改回并保留/清理带作用域的预设即可。

## 11. 已决项

用户已确认「按建议值执行」，以下决策冻结；实现不得偏离，若确需改动必须先回到本节。

| 决策 | 结论 |
| --- | --- |
| 预设作用域 | 加可选 `model` 字段；空 = 全局（兼容旧数据），非空 = 单模型 |
| 同名跨作用域 | 允许；唯一性按 (scope, name)；展示与删除都带作用域 |
| 应用单模型预设 | **整段替换**该模型的行（含不在预设里的行）；确认弹窗必须写明范围 |
| `自定义` 是否记住上次比例 | 不记住；每次从当前生效值初始化输入框 |
| 占比是否用渠道权重兜底 | 用；缺失时降级为 `-` + 说明，不猜数字 |
| 移动端 | 做**写**能力（切换预设 + 自定义比例），新增独立设置页；不改只读统计页 |
| 页面布局 | 桌面按模型分组；移动端独立 tab |

## 12. 执行编排（Orca worker）

协调者（本会话）负责计划、验证与集成；实现由 Orca worker 在各自 worktree 完成。基线分支：`codex/model-routing-ratio-selector`（从 `prod/251` @ `12d8ad4e0` 创建）。一个文件只有一个写入者。

| 任务 | 拥有的文件 | 依赖 | 验收 |
| --- | --- | --- | --- |
| W1 后端作用域 | `setting/operation_setting/model_weight_setting.go`、`setting/operation_setting/model_weight_setting_test.go`、`model/request_policy_test.go` | 无 | `go test -count=1 ./setting/operation_setting ./model` |
| W2 前端共享纯函数 | `web/src/features/system-settings/models/model-weight-presets.ts`、`web/src/features/system-settings/models/__tests__/model-weight-presets.test.ts` | 无 | `bunx vitest run src/features/system-settings/models/__tests__/model-weight-presets.test.ts` |
| W3 桌面 UI | `web/src/features/system-settings/models/model-weight-section.tsx`、`model-weight-preset-bar.tsx`、`__tests__/model-weight-section.test.tsx` | W2 | 同目录 vitest + `bun run typecheck` |
| W4 移动端设置页 | `web/src/mobile/**` | W2 | `bunx vitest run src/mobile` + `bun run typecheck` |
| W5 i18n | `web/src/i18n/locales/*.json`（只能通过脚本） | W3、W4 | `node scripts/find-missing-keys.mjs` 无输出 |

波次：`W1 ∥ W2` → `W3 ∥ W4` → `W5`。W3 与 W4 文件不相交，可并行；两者都依赖 W2 的共享模块，因此 W2 必须先合并进基线分支。

共享模块归属：`model-weight-presets.ts` 是零依赖纯 TS 模块，桌面与移动端都从 `@/features/system-settings/models/model-weight-presets` 导入；**不移动文件**，避免跨任务重命名冲突。

## 13. 本轮检查记录

- 已读取：`AGENTS.md`、`web/AGENTS.md`、`.agents/skills/i18n-translate/SKILL.md`、第 4 节列出的源码与测试文件、`docs/plans/virtual-pool-sticky-session-tdd-plan.md`（格式参照）。
- 已执行并记录基线结果：
  - `go test -count=1 ./setting/operation_setting ./model` → `ok`（0.027s / 8.950s）
  - `cd web && bun install` → 1228 packages，退出码 0
  - `cd web && bunx vitest run src/features/system-settings/models` → 10 test files / 90 tests 全部通过（27.51s）
- 未实施任何代码改动；未提交、未推送、未部署；未访问生产、未调用真实模型 API。
