# OpenRouter / NVIDIA NIM 免费模型审计与渠道模型清理报告

日期：2026-09-17
目标实例：`10.0.0.251:/opt/new-api`（生产，端口 4002）
范围：渠道 5 `OpenRouter Free`、渠道 6 `NVIDIA Free` 的免费模型失效排查、新免费模型评估、失效项清理

## 1. 结论摘要

| 渠道 | 审计前 | 结论 | 审计后 |
| --- | --- | --- | --- |
| 5 OpenRouter Free | 17 个 `:free` 模型 | 无一个在公开目录中彻底消失，但存在到期、端点降级、失效 `test_model`、abilities 残留四类问题 | 18 个模型 |
| 6 NVIDIA Free | 14 个模型 | 3 个已失效、1 个 2 天后退役、1 个死映射、1 个失效 `test_model` | 11 个模型 |

新增可用免费模型只出现在 OpenRouter 侧；NIM 侧 38 个免费端点中 `AVAILABLE=true` 的 20 个里没有值得新增的通用 chat 模型（未配置的可用项只有 `nvidia/ising-calibration-1-35b-a3b`，用途很窄）。

## 2. 判定方法

- 生产侧：`sqlite3 -readonly` 只读查询 channels/abilities/logs/options。
- 上游侧：公开目录与文档，不用上游 key 做探活。
  - OpenRouter：`/api/v1/models`（存在性 + `pricing` 是否为 0）、`/api/v1/models/{id}/endpoints`（端点 `status`、uptime）、官方文档（限流政策、字段定义）。
  - NVIDIA：`build.nvidia.com` 模型目录（`available` / `deprecation` / `Free Endpoint` 属性）、官方公告与 models 清单。
- 交叉核对：两轮独立抓取（含一次由独立复核流程重抓），目录规模与免费集合一致（OpenRouter 444 模型 / 24 免费；NIM 38 免费端点）。

## 3. 关键发现

### 3.1 OpenRouter（渠道 5）

- 17 个已配置 `:free` 模型在公开目录中全部仍然存在且价格为 0，**没有彻底失效项**。
- `dots-studio/dots-3-note-preview:free` 目录字段 `expiration_date=2026-09-30`，13 天后到期。
- `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free` 唯一端点 `status` 为负值、`uptime_1d≈79%`，属降级（负 status ≠ 下线）。
- `test_model` 原为 `openai/gpt-oss-120b:free`，该 `:free` 变体已从目录消失（生产 55 条 404），渠道 `auto_ban=1`。
- `abilities` 残留 `thinkingmachines/inkling:free` / `inkling-small:free` 6 行，上游固定 403（仅允许 agentic harness）。
- 近 4 天只有 3 个模型实际有流量；主要失败是上游 provider 限流透传（`laguna-s-2.1` 66%、`nemotron-3-ultra` 61%），平台级免费限流仅 6 条。每个 `:free` 只有 1 个 provider，无 fallback。
- 可新增候选 7 个：建议 2 个（`inclusionai/ling-3.0-flash-vl:free`、`z-ai/glm-5.2:free`），可选 1 个（`stealth/union-alpha`，隐身模型、数据政策未知），不建议 4 个（2 个仅 agentic harness，2 个 Lyria 音频生成）。

### 3.2 NVIDIA NIM（渠道 6）

- 渠道 base_url 为 NVIDIA 官方 `https://integrate.api.nvidia.com`（经 `relay/channel/openai/adaptor.go` 拼成 `/v1/chat/completions`），非第三方聚合站。
- 失效：`nvidia/nemotron-nano-3-30b-a3b`（模型页 404，不在 models.md / 目录 / 站点地图，生产零成功）、`deepseek-ai/deepseek-v4-pro-0813`（页面 Deprecated）。
- 死映射：`model_mapping` 中的 `minimax-m3 -> minimaxai/minimax-m3`，上游 410「end of life 2026-09-09」，且该源名不在渠道 models 里、不产生 ability。
- 退役预警：`deepseek-ai/deepseek-v4-flash-0731`，官方横幅 "will be deprecated on 09/19/2026"，`available=false`。
- `test_model` 原为 `qwen/qwen3-coder-480b-a35b-instruct`，已不在 NVIDIA 目录（生产 6 次调用 0 成功）。
- 容量问题（非退役）：`nemotron-3-nano-omni` 有 2237 次 `503 ResourceExhausted: Worker local total request limit reached (16/16)`，为账号并发上限；`nemotron-3-super` 约 17% 错误。

### 3.3 计费与监控前置条件

- 两个渠道 `other_info` 为空，未启用余额保护（不存在 `free_models ⊆ models` 约束）。
- 全站 `SelfUseModeEnabled=true`，`GroupRatio` 三组均为 0，`ModelRatio` 无任何 `:free` 键；即当前未定价模型恰好按 0 计费，属隐式依赖。
- 生产 `model_monitor_*` 只覆盖 site_channels 9/21/37/38，**不覆盖渠道 5/6**。

## 4. 已执行的变更（经逐项确认）

执行通道：生产主机本机管理 API `PUT /api/channel/`，走应用自身逻辑（校验 → GORM 更新 → 重建 abilities → 刷新渠道缓存 → 写审计）。不使用直连 SQLite 写库，因为那样不会重建 abilities。

| 项 | 目标 | 变更 |
| --- | --- | --- |
| 1 | 渠道 5 models | 删 `dots-studio/dots-3-note-preview:free`、`nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free`；增 `inclusionai/ling-3.0-flash-vl:free`、`z-ai/glm-5.2:free`、`stealth/union-alpha`（17 → 18） |
| 2 | 渠道 5 test_model | `openai/gpt-oss-120b:free` → `google/gemma-4-31b-it:free` |
| 3 | 渠道 6 models | 删 `nvidia/nemotron-nano-3-30b-a3b`、`deepseek-ai/deepseek-v4-pro-0813`、`deepseek-ai/deepseek-v4-flash-0731`（14 → 11） |
| 4 | 渠道 6 model_mapping | `{"minimax-m3":"minimaxai/minimax-m3"}` → `{}` |
| 5 | 渠道 6 test_model | `qwen/qwen3-coder-480b-a35b-instruct` → `openai/gpt-oss-20b` |

未改动 key、base_url、status、group、priority、weight、auto_ban；未重启服务。

## 5. 执行后复核

- API：`GET /api/channel/5` = 18 模型；`GET /api/channel/6` = 11 模型、`model_mapping={}`、新 test_model；断言全部通过。
- 数据库：ch5 abilities 57 → 54、ch6 42 → 33（均为 模型数 × 3 组）；inkling 残留归零；被删模型无 abilities 残留。
- 对外可见性：本机 `/v1/models` 中 3 个新模型可见、5 个被删模型不可见、渠道 6 保留的 11 个模型全部可见；`/api/status` 200。
- 审计：`logs` 表记录 `Updated channel OpenRouter Free (ID: 5)` 与 `Updated channel NVIDIA Free (ID: 6)`。

## 6. 渠道模型增删操作要点

- 路由与对外列表的真相源是派生的 `abilities` 表，其来源是 `channels.models`；`model_mapping` 不产生 ability。
- 入口：管理端 `/channels` → Edit → `Models & Groups` → `Models` chips → 保存（`PUT /api/channel/`）；上游列表可用 `GET /api/channel/fetch_models/:id`。
- 陷阱一：`PUT /api/channel/` 为 patch 语义且 GORM 跳过零值，清空字段必须写非零值（例如 `model_mapping` 写 `{}` 而非空串），保存后应 GET 复核。
- 陷阱二：若启用余额保护，`free_models` 必须 ⊆ `models`，删模型需在同一次请求内同步移除。
- 禁止直连 SQLite 修改 `channels.models`：abilities 不会重建，路由与价格页会不一致，且无审计。

## 7. 遗留事项（未执行）

1. 为新增的 3 个免费模型显式配置 `ModelRatio[model]=0`（当前依赖自用模式，属隐式依赖）。
2. 将渠道 5/6 纳入 `model_monitor_*`，使模型失效可自动发现。
3. 对高错误率路径（`poolside/laguna-s-2.1:free` 66%、`nvidia/nemotron-3-ultra-550b-a55b` 61%、`nemotron-3-nano-omni` 并发 503）做限流、重试与降权。
4. `stealth/union-alpha` 的上游数据政策未确认，建议仅内部或灰度使用。
5. 免费模型池周转快（OpenRouter 近一个月有 7 个曾使用的免费模型下线），建议建立周期性目录 diff 审计。

## 8. 回滚

变更前 channels/abilities 行级备份保留在目标主机 `/root/newapi-free-model-audit-backup-20260917-2255/`（含完整行）。回滚方式：以原值再次调用同一条 `PUT /api/channel/` 接口即可恢复。
