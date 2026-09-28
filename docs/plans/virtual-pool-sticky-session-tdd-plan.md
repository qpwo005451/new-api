# 虚拟模型池会话粘滞与负载均衡 TDD 计划

状态：S1–S5、S7 已在本地工作树实施并通过回归；S6 客户端抓取脚本已实现，但 Hermes/Pi/Codex/Prime 安装版抓取与 C7 发布候选构建仍未完成。未提交、未推送、未部署，生产步骤不是授权。

## 1. 范围与基线

目标：fork 的虚拟模型 `deepseek v4.1 flash` 聚合 Ollama Pro 与 Command Code 两渠道。新会话按负载分散，同会话优先复用稳定候选；故障迁移服从响应、上游状态、权限与计费安全边界。客户端覆盖 Hermes、Pi、Codex、Prime Agent。

- 本工作树：`/home/ra/orca/workspaces/Newapi/codex-virtual-pool-sticky-tdd-plan`。
- 本轮核对 HEAD：`dde3361911849ac4f1a08b9507405dcad1503e4b`，与交接的父工作树 `prod/251` 基线一致；未联网确认远端最新状态。
- 当前分支从 `qpwo005451/codex-virtual-pool-sticky-tdd-plan` 改名为 `codex/virtual-pool-sticky-tdd-plan`。未操作父工作树 `/home/ra/workspaces/projects/Newapi` 或其未跟踪 `.tmp/`。
- 本轮只写本文；未实施代码、修改客户端配置、提交、推送、合并、访问生产或调用真实模型 API。
- 已读取本工作树 `AGENTS.md`、下述相关源码、现有路由设计和发布脚本。当前工作树没有 `.local-tools/`，工具链准备是实施前置项。`go.mod` 要求 Go **1.25.1**，不能只按概述中的 Go 1.22 准备。
- 第一版覆盖同步 HTTP Chat Completions、Responses 及 SSE。不承诺 Realtime/WS、异步任务、私有上游状态跨渠道迁移；这些入口保留现有路径，明确不具备新粘滞保证。客户端 WS 字段证据不等于网关 WS 支持。
- 第一版不做 UI、不新增数据库表、不做主动生产探针、不复刻复杂评分。配置走现有 options 通路，向后兼容；新增功能默认关闭。若另案增加 UI，先读 `web/AGENTS.md` 及 React、shadcn、i18n skills，单独评审与验证。

## 2. 证据与版本限制

### 2.1 本工作树已复核

下列路径相对仓库根，行号对应上述 HEAD。

| 证据 | 当前行为与实施影响 |
| --- | --- |
| `middleware/distributor.go:115–150,181–183` | HasPool 时跳过现有亲和；middleware 创建 RetryParam 选首候选；HTTP <400 回写不适用于 SSE 成功判定。 |
| `middleware/distributor.go:455–498`、`model/channel.go:221–280` | 多 Key 渠道在候选选定后才调用 `GetNextEnabledKey`；random/polling 可能每次请求换 key，现有 channelID-only 绑定和容量无法保证账号粘滞或私有状态归属。 |
| `service/channel_affinity.go:337–350,550–620,713–740` | 默认键无强制用户/Token 隔离；KeySources 是 first-nonempty，不是拼接；缓存仅存 channelID，不能表达池成员、版本和认领。 |
| `setting/operation_setting/channel_affinity_setting.go` | 默认规则针对 GPT Responses、Claude Messages；不能仅扩 regex 就完成本虚拟池的会话调度。 |
| `service/channel_select.go:85–206`、`service/virtual_route_pool.go` | Targets 与 Sources 展开后可重复同真实候选；按索引重试；UsesVirtualRoute 只看 Targets，与 HasPool 不一致。 |
| `controller/relay.go:224–236,482–515` | controller 又创建 RetryParam，首试可能再次 prepare/rotation；必须通过完整 middleware→controller 测试证明只推进一次。 |
| `relay/helper/model_mapped.go` | 有链式模型映射和 reasoning 重写。虚拟名、target 名、最终 upstream 名应分别保存；去重、健康和容量不能误用名称。 |
| `service/virtual_route_health.go`、`channel_select.go:142–159` | 冷却只排后，不跳过；健康仅进程内。本计划严格跳过是新开关内的行为变化。 |
| `controller/upstream_rate_limit.go`、`controller/relay.go:288–320` | RPM gate 控制起步速率，不是并发槽；调用使用 OriginModelName，须核对最终映射目标作用域。 |
| `controller/relay.go:367–418,517–550` | nil error 被记录为健康成功；shouldRetry 没检查下游提交。 |
| `relay/channel/openai/relay-openai.go:173–180` | 未收到 finish reason 可发送 SSE error 后返回 usage,nil；直接改成 error 可能触发外层退款，必须分离路由 outcome 和结算结果。 |
| `relay/common/stream_status.go` | IsNormalEnd 接受 EOF/HandlerStop；不能直接用作所有协议的业务成功判据。 |
| `service/billing_session.go`、`relay/common/billing.go` | 已有预扣、幂等结算/退款及资金已结算保护；Refund 异步。不能每次 attempt 新建计费会话。 |
| `docs/virtual-model-route-design.md` | 既有设计是请求轮询、进程内健康；其 Sources 全空回退描述应与新回归核对。新开关下建议空池明确不可用，不静默越池。 |

### 2.2 承接的客户端调研证据

本节为交接证据，不冒充本轮安装版抓包。实施时按固定版本复核；临时研究副本丢失则按 commit 恢复，不用 main 替代。只定点读文本，不读 secret、不扫描大二进制、不修改其他仓库。

| 客户端与固定版本 | 已知行为 | 待验证/适配 |
| --- | --- | --- |
| Codex npm 0.155.1，tag `rust-v0.155.1`，commit `be2951ea34f0d295ed0becf97079f92fa5f6950e`；`/tmp/newapi-codex-research` | `core/src/session/session.rs:785–821`、`core/src/client.rs:1213–1222,869–887`：HTTP/WS session-id 是 root，thread-id、x-client-request-id 是独立 thread；prompt_cache_key 默认 root，另有 client_metadata。子代理同 root 不同 thread，resume/compact 稳定。 | 建议独立 thread 分散子代理；root 模式可选，尚未获用户最终确认。不能引用 main 新增行为作为安装证据。 |
| Pi Windows `@earendil-works/pi-coding-agent` 0.87.0，公开对照 `898ab804050730e9dcefb4443875d5a932aa6a32`；`/tmp/newapi-pi-research` | custom completions 默认无可靠 session；Responses 有 session_id/prompt_cache_key，cacheRetention:none 抑制自动字段。resume 稳定、fork 新 ID、compact 持久 ID 稳定。 | 安装包在 `/mnt/c/Users/RA/AppData/Roaming/npm/node_modules/`；`before_provider_headers` 可原地改 event.headers，用 ctx.sessionManager.getSessionId()。compat.sendSessionAffinityHeaders 需验证禁缓存组合。 |
| Hermes 本地 `/home/ra/orca/workspaces/hemes-studio/merge-update/src/hermes-agent`，HEAD `3c9e3619f8df5a9c5584c35a4a14954dcf2278f5`，相关文件有用户暂存修改 | custom completions 默认无可靠 session；Responses pck_hash(scope+instructions+tools) 是缓存指纹；HTTP session_id 只在 codex backend 特殊路径；入站 X-Hermes-Session-Id 不是出站字段。in_place 压缩稳定，旧压缩 physical ID 改变而 logical scope 可稳定。 | 公开 `/tmp/newapi-hermes-research` 的 `9039b690fc34b965b2614bea33d88b0216752aa0` 有 opt-in session_affinity_header，本地没有。单列跨仓库适配，使用 logical ID；不覆盖用户修改。 |
| Prime Agent 安装 0.9.5；公开 `/tmp/newapi-prime-research`，`1e2cc22784311bd9799cd9489573922552b537da` 也标 0.9.5 | 公开源码 completions 无默认 session，Responses 非 none 有 session_id/prompt_cache_key；安装 docs 支持 registerProvider 自定义 streamSimple(options.sessionId/headers)。before_provider_request 只能改 body，不能注入 HTTP header。 | 未证明 commit 对应安装二进制，安装版 capture 未完成，是独立验收阻断项。不能冒充 Pi 或套用 Pi hook。 |
| sub2api `/tmp/newapi-sub2api-research`，`20a94fbb567b62208751292ed7786b24a7e7c0fe` | Redis(group,session)→account，滑动 TTL、existing 优先/等待/可选 escape；新会话优先级/负载/LRU 或 OpenAI 高级评分。 | 借鉴状态、等待和容量机制，不直接复制代码，不在第一版复刻评分。 |

## 3. 建议契约与安全边界

本节为评审建议，待第 9 节决策后固化测试。

### 3.1 会话身份与隔离

认证后从服务端 context 取 userID（当前用户作为租户边界）、tokenID、授权组；不可相信请求自报租户。未来组织租户需额外维度。建议键为 `new-api:virtual-sticky:v1:<scopeDigest>:<sessionDigest>`，scope 结构化编码 userID、tokenID、授权 group/auto 策略、规范化虚拟模型、语义版本；session 编码提取策略、ID 类型和值。不要用可碰撞的冒号拼接，不存 Token secret、原始提示词或会话 ID 日志。Token 轮换默认新绑定；跨 Token 共享不是默认能力。

建议优先级：`X-NewAPI-Session-ID` > `thread-id`/`thread_id` > `session-id`/`session_id` 兼容头 > 显式允许的 body session_id > body prompt_cache_key。HTTP 头名大小写不敏感，但连字符/下划线别名须显式处理。x-client-request-id 仅在版本验证过的 Codex profile 中作 thread 兼容来源，不能全局把 request ID 当 session。User-Agent 不能用于放宽权限。

- 同层重复头/别名有不同值：400；空白视为无值。建议 ID 为 1–256 UTF-8 字节，控制字符或超长拒绝，不能截断合并。
- 不同层冲突：按优先级选取，记录无原值的 source/conflict 指标。Codex thread 与 root 不同是预期，不拒绝。专用头优先意味着客户端显式选择。
- 无 ID：正常按新请求分配，不缓存绑定，不让所有无 ID 请求共享常量键。
- prompt_cache_key 是低置信缓存作用域；是否允许作为 fallback 待决。Hermes 缓存指纹不能宣称严格会话 ID。
- 内容指纹第一版默认关闭；未来若启用必须短 TTL、隔离、脱敏，承认碰撞，不能作为私有状态归属证据。
- 建议 Codex 默认独立 thread；root 模式会聚集子代理，需显式选择。resume/compact 保持绑定，fork/新子 thread 新绑定。
- 专用头默认仅网关消费，不无条件向上游转发；不为了路由覆盖 prompt_cache_key 原有缓存语义。

### 3.2 候选身份与一次准备

候选包含 channelID、accountIdentity、targetModel、finalMappedModel、effective reasoning、授权组、endpoint/协议能力和配置 revision。单 Key 渠道的 accountIdentity 为 channelID；多 Key 渠道为 channelID + 稳定 key index/fingerprint，绝不保存原始 key。执行去重依据同 accountIdentity、同最终模型、同 reasoning、同授权语义；不同 reasoning 不可被错误折叠。保留 target 别名用于配置失效与日志。模型大小写遵循现有匹配规则，不擅自把上游大小写敏感名称归并。

多 Key 渠道必须在候选认领前确定 accountIdentity：首次认领可沿用现有 random/polling 策略选出一个 enabled key，随后绑定其 index；复用与发送前必须验证同一 index 仍 enabled，并由 `SetupContextForSelectedChannel` 使用绑定 index，不能再次调用无约束的 `GetNextEnabledKey`。若渠道无法固定到可验证的 key index，或绑定 index 已禁用/失效，该候选不得复用：撤销旧绑定并按新请求重新认领。原始 key 不得进入 Redis、日志、指标或测试 fixture。

middleware 创建 request-scoped PreparedRoute，controller 复用，rotation 每请求至多推进一次。Sources-only/Targets-only 统一 HasPool。attempted set 使用执行身份，仅真正开始 attempt 时加入；等待槽不计 attempt。max_attempts 限制唯一执行总数，controller 和 provider 内部重试不得绕过。

每次复用与发送前检查 enabled、ability、模型/路径、当前组授权、mapping revision、冷却。排序变化不改变身份；删除成员、禁用渠道、撤权、修改映射/reasoning 必须使旧绑定失效。在途保留不可变快照，但晚到成功不能写回新 revision。auto 组绑定不能绕过当前授权或改变既有收费组合同。

配置要明确两渠道的目标模型。Sources 展开 channels.models 的全部模型，不能未经审查把所有模型混入 `deepseek v4.1 flash`。真实 channelID、精确模型名、协议和 reasoning 支持待评审，本文不编造。

### 3.3 认领、确认、迁移与故障

建议状态机：`Absent → Pending(owner,generation,leaseUntil,candidate,revision) → Confirmed(candidate,generation,idleExpiry)`。首请求原子认领短租约，协议成功后 CAS 确认；并行首请求建议有界等待认领完成，再各自占容量槽。

- Redis 用原子脚本/事务 claim、confirm、renew、invalidate、release，不是 GET 后 SET。相关键需要原子操作时设计同一 cluster hash slot，明确其分片代价。
- owner/generation 必须防 ABA：过期重建也不能复用旧身份。旧成功、旧失败、旧取消、旧 release 只作用于自己版本，不能覆盖迁移或删除新租约。
- 参数评审起点：Pending 30 秒，每 10 秒续租；Confirmed 空闲 TTL 60 分钟；等待 2 秒。长流续租，测试用虚拟时钟。参数不是容量实测结果。
- 失败/取消撤销自己的 Pending；完整协议成功才确认/滑动续期。Confirm 存储失败不能谎报已绑定，也不能追溯撤销已经交付的响应。
- 失去认领租约不得晚写成功；失去容量租约应取消上游、停止继续执行，标记结果未知；有输出仍绝不重试。
- 单实例显式 memory 模式实现同一锁、版本和 TTL 合同，重启丢绑定。多实例必须 Redis；Redis 故障默认对 sticky 新 admission 返回 503，不静默各自内存降级。已开始请求按可用证据完成/取消和结算。
- 恢复后检查 epoch/revision，禁止旧内存无条件回灌。Redis 数据丢失后私有状态归属无法恢复的请求不能迁移。

### 3.4 容量与健康

容量作用域默认 accountIdentity + finalModel；单 Key 渠道的 accountIdentity 为 channelID，多 Key 渠道为 channelID + 绑定 key index/fingerprint。同账号多 channel 若共享配额，配置 shared capacity group。不同虚拟别名不能绕过同一容量。RPM gate 独立保留，且必须使用绑定 accountIdentity 对应的 key，而不是重新选取下一个 key；虚拟名与实际目标规则匹配优先级要兼容测试。

建议新会话先满足既有优先级层，再最小化 `(inflight+1)/(capacity*weight)`，硬约束 inflight < capacity；平局用稳定轮转。容量有限、权重正数，缺少配置不启用新策略。评分选择与占最后一槽要原子，冲突有界重选。

旧会话满载有限等待，建议超时 429 + Retry-After，不默认因忙迁移；escape 待决。冷却严格跳过，全冷却/无资格返回 503，不触发真实主动探针；到期可由真实请求获取单一恢复尝试资格。多实例共享健康状态。

容量槽为 request/attempt owner 租约，不是会话绑定 TTL。正常完成、预发送错误、取消、异常都幂等释放；崩溃由 TTL 回收，旧 release 不能减少新槽。网络分区时租约过期不能证明真实上游停止：取消、硬截止和保守重用共同控制，记录外部并发可能短暂超限的限制，不能宣称分布式绝对精确容量。

### 3.5 AttemptOutcome、重试与用量

拟在 `relay/common` 增加 AttemptOutcome：attempt ID、候选身份、downstreamCommitted、上游接受/未知状态、protocolCompleted、失败类别、replaySafe、usage/settlement 状态。使用现有 StreamStatus 信号，由协议判定完整终止；HTTP 200、EOF、usage 到达、nil error 都不能单独证明成功。

**AttemptOutcome 接线契约（评审后固化）**

- request-scoped recorder 放在 `relay/common`，由 `RelayInfo` 持有当前 attempt 的 outcome。协议 handler/adaptor 是唯一写入者；每次 attempt 只能提交一次，重复提交、错 attempt ID 或缺失 attempt 必须显式失败。
- 协议层用 `BeginAttempt`、`MarkDownstreamCommitted`、`MarkProtocolCompleted`、`MarkFailure(category,replaySafe,upstreamState)` 和 `Snapshot` 一类最小接口交接；字段至少覆盖 attempt ID、候选身份、downstreamCommitted、upstreamAccepted/unknown、protocolCompleted、失败类别、replaySafe、usage/settlement 状态。
- controller 在 `DoResponse` 后只消费 outcome 快照来决定确认、健康、重试和迁移；不能再以 `newAPIError == nil`、HTTP 200、EOF、usage 到达或 `StreamStatus.IsNormalEnd()` 单独推断成功。没有 outcome 时按“结果未知”处理，已提交则禁止重试。
- `OaiStreamHandler` 未收到 finish reason 时仍可保留 `usage,nil` 的结算兼容返回，但必须把 outcome 标为协议未完成；controller 不得确认绑定或健康成功。Responses overload、buffered stream、provider 内部重试同样必须在同一 outcome 边界内记录。
- outcome 与结算结果分离：协议失败、路由重试和驱动退款的 `newAPIError` 不互相伪造；计费继续由 BillingSession 按既有 usage/refund 合同处理。

| 结果 | 绑定/健康 | 重试和计费 |
| --- | --- | --- |
| 完整协议成功 | CAS 确认、健康成功 | 一次结算、释放槽 |
| 未提交且明确未执行的可重放失败 | 不学习成功，按类别冷却 | 按 attempted set 换候选，复用一次预扣 |
| 部分 SSE、已 Flush 头/心跳、已发送 SSE error | 不学习成功，记录真实失败类别 | 绝不再请求另一上游；保留已耗用量结算，不盲退 |
| 客户端取消 | 不学习，不作为渠道故障 | 不重试；按既有 usage/refund 合同收尾 |
| 上游可能执行，结果未知 | 不确认成功 | 默认不重放；未向下游输出不是充分条件 |
| previous_response_id/provider 私有状态 | 必须验证原 candidate/账号归属 | 原归属未知或不可用则明确失败，不能跨上游；完整上下文重放需另证协议能力 |

Committed 覆盖 WriteHeaderNow、Flush、首次字节及 wrapper，不能仅测 Status()==200；仅设置未发送的 header 不等于提交。Responses overload、buffered stream 等 provider 内部重试服从同一边界，不能只改 controller。

BillingSession 跨整个用户请求存在。路由失败与驱动退款的 newAPIError 分离，不把 usage,nil 简单替换成 error。固定 fixture：预扣 100、最终用量 60，净扣 60；全失败无用量净扣 0；部分流按既有策略结算 30，净扣 30，不能又全退 100。分别核对钱包、订阅、Token 和 consume/pending log。资金已结算但 Token 调整失败不能重复结算/退款。异步退款通过完成信号等待，不 sleep。估算用量沿用现有规则，不借功能改变计价。

## 4. 分阶段 TDD 与提交拆分

依赖：S0 → S1 → S2 → S3 → S4 → S5 → S6 → S7。S6 可在 S1 契约确定后独立采证；S7 隔离脚本工作可并行，但发布须等待全部验收。每阶段记录 Red 失败原因、Green 结果与 Refactor 回归；Red 应是行为不满足，不能仅编译失败。以下新文件与测试名均为拟建。

| 阶段/建议提交 | Red：先失败的行为 | Green：文件与实现边界 | Refactor/验收/命令 |
| --- | --- | --- | --- |
| S0 `test: define virtual sticky contracts` | middleware→controller 双选；Sources-only 分歧；多 Key 渠道两次请求可能换 key；部分 SSE 被记成功；adaptor 无法把协议完成/未知结果交给 controller。 | 复用 `controller/virtual_model_route_test.go`、`middleware/virtual_model_mapping_test.go`、`service/virtual_route_pool_test.go`；拟建 `controller/virtual_pool_sticky_test.go`，本地 fake provider、时钟、屏障和 test-local outcome recorder fixture。 | 审定第 3 节，旧行为缺陷不能固化为正确断言；C1。Red 提交可评审，合并时必须含对应 Green。 |
| S1 `feat: resolve scoped virtual session identity` | 同 ID 跨 user/token/group/model 不共享；冲突/无 ID/超长；thread/root 区分。 | 新建 `service/virtual_pool_session.go` 及 `_test.go`；`setting/operation_setting/model_retry_policy_setting.go` 和测试增加默认关闭、参数边界、旧配置兼容。 | 结构化 namespace、提取与权限分离，common JSON wrappers；C1。 |
| S2 `fix: prepare virtual candidates once` | target/source 重复只执行一次；Sources-only；轮询一次；多 Key 渠道换 key；mapping 链/循环/reasoning；pool 编辑失效。 | `service/channel_select.go`、`virtual_route_pool.go`、`middleware/distributor.go`、`controller/relay.go`、`model/channel.go`、必要的 `relay/helper/model_mapped.go`；PreparedRoute、accountIdentity、绑定 key index 的 Setup 变体、稳定身份、attempted set。 | 统一 HasPool，不改非池选择语义；多 Key 不可固定账号时排除 sticky；C1、C2；mock 收到模型、key index 和次数精确匹配。 |
| S3 `feat: add fenced sticky leases` | 并行首请求仅一个认领；到期重认领；旧成功/取消不覆盖；两实例共享；Redis 断开拒绝 admission。 | 新建 `service/virtual_pool_binding.go`、`virtual_pool_binding_redis.go` 及测试；Store 契约、CAS、续租、memory 模式。 | memory/Redis 共用合同；真实本地 Redis 验证原子性，fake 只验证分支；C1、C3、C4。 |
| S4 `feat: schedule by pool capacity` | 确定容量/权重分配；多 Key 账号共享容量；旧会话等待/取消；全冷却零外呼；崩溃槽回收；别名共享容量；最后一槽不重复占。 | 新建 `service/virtual_pool_capacity.go`、`virtual_pool_scheduler.go` 及测试；`service/virtual_route_health.go`、`controller/upstream_rate_limit.go`。 | 槽与 binding 生命周期分离；容量按 accountIdentity，RPM 不代替并发；C1、C3、C4；所有终结路径无泄漏/负计数。 |
| S5 `fix: gate retry and learning on attempt outcome` | SSE 一段断流后第二上游请求为 0，不确认成功；Flush 同样禁止重试；未知执行不重放；私有状态不跨候选；账目精确。 | `relay/common/relay_info.go`、`stream_status.go`、新建 `relay/common/attempt_outcome.go`；`relay/channel/openai/relay-openai.go`、`controller/relay.go`、`relay/responses_overload_retry.go`、`relay/opencode_buffered_stream_retry.go`；沿实际 Responses handler 接线。新建 `controller/virtual_pool_outcome_test.go`、`service/virtual_pool_billing_test.go`。 | outcome 与结算解耦；协议 handler 单写、controller 单读；统一终结点确认；C1–C4；不改变用量保留规则。 |
| S6 `test: capture client session contracts locally` | 四客户端新建/resume/fork/compact/子代理/禁缓存字段逐项比较；无 ID 是缺口，不能标通过。 | 新建 `scripts/capture_virtual_pool_client.py`、`docs/testing/virtual-pool-client-capture.md` 和脱敏 fixtures；Hermes、Pi、Prime 适配作为独立客户端任务，另行授权。 | capture 解析字段而非仅看日志；Prime 安装版验证未完成不得签收；C5。 |
| S7 `test: harden isolated release acceptance` | copied env/DB 有真实出口时拒绝启动；full smoke 不能向非隔离目标发模型请求；清理不删除唯一回滚材料。 | `scripts/stage_release_runtime.sh`、`smoke_release.sh`、`test_release_helpers.sh`、必要的 PowerShell helper 与测试；补充本计划与现有路由文档。 | 本地 fake systemctl/网络/DB 验证 fail-closed；C6、C7；再走第 8 节审批流程。 |

每阶段提交仅含该合同测试与最小实现；不要跨阶段顺手清理业务逻辑。最终 PR 可合并若干相邻阶段，但 reviewer 应能分别核对身份、路由、存储、容量、outcome/计费、客户端证据、发布隔离。

## 5. 确定性测试矩阵

新建/重写后端测试使用 testify/require 做初始化及致命断言、assert 做值检查。显式初始化 DB、用户、Token、分组、settings、cache，并用 t.Cleanup 恢复；共享全局状态的测试不盲目 t.Parallel。用 channel/barrier 控制并行顺序、fake clock 控制过期、可注入完成信号控制异步退款；禁止 sleep、随机循环或时长比较充当正确性证明。

| ID | 输入/故障 | 必须断言 | 归属 |
| --- | --- | --- | --- |
| T01 | 相同 ID，变更 user/token/group/virtual model | 不共享绑定，不越权，原始 ID 不入日志 | S1 |
| T02 | 无 ID、空值、重复头、大小写/别名、控制字符、超长、各层冲突 | 明确提取优先级或 400；无 ID 不写绑定；thread/root 不同不误拒绝 | S1 |
| T03 | Codex 同 root 两 thread；resume/fork/compact | thread 模式分开、root 模式合并；resume/compact 稳定、fork 新绑定 | S1/S6 |
| T04 | Pi/Hermes/Prime completions 与 Responses，cache none | 默认缺字段如实记录；适配专用头独立于缓存开关；Prime 安装证据单列 | S6 |
| T05 | 两首请求在 claim 点同时进入 | 一个 owner；另一个受控等待；无双绑定/绕过容量 | S3 |
| T06 | Pending 到期、续租、崩溃、新认领、旧成功/旧 release | 新 generation 不被覆盖或释放；无 ABA；过期可恢复 | S3/S4 |
| T07 | 池排序、删除/禁用、撤权、修改 model mapping/reasoning | 排序不破粘滞；失格不复用；在途旧 revision 不覆盖新状态 | S2/S3 |
| T08 | Sources-only、Targets-only、混合重复、两别名映射同目标 | HasPool 一致；执行身份去重；reasoning 不同保留；一次 request 只轮转一次 | S2 |
| T09 | 模型链/自映射/循环、auto group 变化 | 真实上游 payload 模型/reasoning 正确；循环不外呼；组授权/收费正确 | S2/S5 |
| T10 | 一个冷却、全部冷却、到期恢复、客户端 400/取消 | 冷却不访问；全冷却 503；到期受控 admission；客户端问题不误冷却 | S4/S5 |
| T11 | 固定容量/权重、平局、满槽、等待中取消、预发送失败 | 分配结果精确、旧绑定优先、429/Retry-After；释放一次且不负数 | S4 |
| T12 | 两实例争最后一槽、Redis 断连/恢复/数据丢失 | 原子上限、fail-closed、无静默 split-brain；旧 epoch 不回灌 | S3/S4 |
| T13 | 非流成功、流正常完成、HTTP200 后断流、soft error/EOF | 只有协议 handler 写入完整 outcome 才确认；失败不学习/不清健康失败；usage 保留 | S5 |
| T14 | SSE 一段/仅头 Flush/心跳/SSE error 已提交 | outcome 标为未完成/不可重放；捕获第二上游调用数为 0，不改成新的 HTTP error 响应 | S5 |
| T15 | 未提交的安全失败 vs 上游已接受但未知结果 | 仅前者可迁移；attempted set/max_attempts 对内部重试同样有效 | S2/S5 |
| T16 | previous_response_id、私有 turn state、多 Key 账号轮换 | 无归属证明不跨渠道/账号；绑定 key index 禁用即失效；多 Key 渠道不能固定账号须拒绝状态续接 | S2/S5 |
| T17 | 预扣→失败→成功、全失败、部分流、取消、结算部分失败 | 钱包/订阅/Token 精确净额；无重预扣、双结算、误退已消费；日志不双写 | S5 |
| T18 | 关闭开关、非池/非目标模型、旧 JSON 配置 | 原路由、既有亲和、价格合同不回归；不创建新缓存或容量槽 | S1–S5 |
| T19 | SQLite/MySQL/PostgreSQL settings 持久化/重启 | 同配置往返一致，无 dialect-only DDL；默认关闭不需 schema 迁移 | S1/C4 |
| T20 | 候选复制真实 env/DB，后台任务试图外呼，错误 smoke target | 启动前拒绝或网络层拦截，真实系统零测试写入；mock 命中计数正确 | S7 |
| T21 | 多 Key 渠道首次绑定、key 禁用、random/polling、同账号多 channel、容量共享 | 绑定同一 key index；禁用即失效并重认领；容量不按 channelID 重复计数；日志/缓存无 key 原文 | S2–S4 |

多实例 Redis 测试必须用两个独立 Store/client；进程内共享锁不能充当分布式证明。真实本地 Redis 脚本测试与纯 fake 时钟契约测试分开。对于 Redis 服务端时间，使用测试专用可注入时钟/明确过期状态 fixture；不要以生产脚本随意接受客户端时间来换取测试便利。

## 6. 实施运行命令与环境

**本轮未执行以下命令。** C1–C7 是计划标签，不是已存在的仓库任务。只在本地 Linux 文件系统、隔离测试 DB/Redis、假凭据环境执行。安装项目私有 `.local-tools/go/bin/go`、`.local-tools/bun/bun`，核对 Go 1.25.1+ 与锁文件；不加入系统 PATH。PowerShell `pwsh` 也需事先可用。本轮发现工具目录缺失，不能跳过准备。

在仓库根执行，先设置测试专用 GOMODCACHE/GOCACHE/GOPATH 于 `.local-tools/release-cache` 下经工具版本区分的目录；下载依赖也只在本地。日志放 `/tmp/`，结束后仅保留脱敏验收摘要并删除临时日志。墙钟超时失败即停，不能当作测试通过。

### C1：受影响后端包

```bash
timeout 300s .local-tools/go/bin/go test -count=1 -timeout=240s ./setting/operation_setting ./service ./middleware ./controller ./relay/helper ./relay/common ./relay/channel/openai ./relay > /tmp/virtual-pool-c1.log 2>&1
```

Red/Green 开发期间可追加 `-run 'TestVirtualPool|TestVirtualRoute|TestGetChannelUsesVirtualRoute|TestShouldRetry'` 缩小反馈；新增合同测试统一 TestVirtualPool 前缀。最终必须去掉过滤运行包回归，确认有测试实际执行，不能把零匹配当 Green。

### C2：竞态与映射回归

```bash
timeout 600s .local-tools/go/bin/go test -race -count=1 -timeout=540s ./service ./middleware ./controller ./relay/common ./relay/helper ./relay ./relay/channel/openai > /tmp/virtual-pool-c2.log 2>&1
```

race 在本地主机可用的 CGO/C 编译器环境执行，不沿用 release 的 CGO_ENABLED=0；缺工具明确阻断，不悄悄跳过。若改动 `relaykit/` 或其公共 API，必须额外在该模块目录执行（根模块 build 不替代）：

```bash
cd relaykit
timeout 300s env GOWORK=off ../.local-tools/go/bin/go build ./... > /tmp/virtual-pool-relaykit.log 2>&1
```

第一版优先把 outcome 留在根模块 `relay/common`，不迫使 relaykit 依赖根模块。确需公共 API 变化时单列提交与独立验证。

### C3：本地 Redis 多实例合同（测试入口待 S3 实现）

拟新增测试开关 `VIRTUAL_POOL_TEST_REDIS_ADDR`，只接受 loopback 且专用空实例/命名空间；未配置时集成测试显式 skip，验收必须确认不是 skip。Redis 启动方式依据本地已安装服务确定，不能复用生产连接或清空共享 DB。下例端口是测试约定，须先确认空闲：

```bash
timeout 300s env VIRTUAL_POOL_TEST_REDIS_ADDR=127.0.0.1:16379 .local-tools/go/bin/go test -count=1 -timeout=240s ./service -run '^TestVirtualPoolRedis' > /tmp/virtual-pool-c3.log 2>&1
```

### C4：计费/兼容集成（入口待实现）

SQLite 内存 fixture 进入 C1。新增 `TestVirtualPoolDatabaseContract`，覆盖 T19 settings 持久化/重启及计费合同，并显式连接本地一次性 MySQL >=5.7.8、PostgreSQL >=9.6 沙箱，建议同时覆盖实际维护版本。连接通过仅本地测试环境注入，不写仓库或命令日志；未配置必须报告 skip，不能宣称三库验证完成。

```bash
timeout 300s .local-tools/go/bin/go test -count=1 -timeout=240s ./service ./model -run '^TestVirtualPool(DatabaseContract|Billing)' > /tmp/virtual-pool-c4.log 2>&1
```

沿用 GORM 和 common JSON wrapper；如果需锁用 model 的 lockForUpdate，不能照搬 MySQL 专有语法。任何新增 schema 需求应回到设计评审，不随手扩展此无迁移方案。

### C5：客户端 mock capture（脚本/参数是待实现契约）

```bash
timeout 600s python3 scripts/capture_virtual_pool_client.py --bind 127.0.0.1 --port 18081 --output /tmp/virtual-pool-capture.jsonl
```

capture server 只监听 loopback，实现客户端所需的最小非流/流协议响应；使用独立临时 profile、假 API key、短合成 prompt。deny egress，禁遥测、更新、真实工具和 OAuth 流程；若某客户端无法把完整请求限制在本地则停止该用例，不能“试一次”真实 API。不要改用户原 profile。本轮不启动这些客户端。

每个安装版本记录二进制版本/可校验来源、endpoint、provider、缓存设置、字段存在性、同一会话相等/新会话不同关系；不保存 Authorization、真实 session ID 或 prompt。分别测试两次请求、resume、fork、compact、父子并行；没有某能力时标 N/A 并说明，不伪造。HTTP 和 WS 分开记录，WS 网关支持不纳入本版。适配前后各捕获一次，Hermes 的 logical scope 与 Prime 的真实安装 API 必须有证据。

### C6：发布 helper 本地回归

已确认两脚本存在；测试会创建/删除本地 fixture，应在实现用干净测试 checkout 中执行，避免覆盖现有构建产物。

```bash
timeout 300s bash scripts/test_release_helpers.sh > /tmp/virtual-pool-c6-shell.log 2>&1
timeout 300s pwsh -NoProfile -File scripts/test_local_release_helpers.ps1 > /tmp/virtual-pool-c6-ps.log 2>&1
```

现有测试中包含脚本文本断言，新增隔离合同应检验可观察启动/网络行为，不只断言某行 env 文本存在。

### C7：本地 Linux amd64 候选编译

实现阶段 PR 经验证合入 `prod/251` 后，以固定 commit 构建，不用浮动 HEAD；`<release-id>`、`<verified-prod-commit>` 为必须替换的占位值：

```bash
timeout 1800s pwsh -NoProfile -File scripts/build_release_candidate_local.ps1 -ReleaseId <release-id> -ReleaseTag <verified-prod-commit> > /tmp/virtual-pool-build.log 2>&1
```

已核实脚本使用 `.local-tools` 私有 Go/Bun，Linux amd64、CGO_ENABLED=0、GOWORK=off，产出 `releases/<id>/bin/new-api` 和 `manifest.env`，校验 ELF；底层脚本构建前端并管理 detached source checkout。`main.go` embed `web/dist`，不能用缺资源的根 build 伪造完整发布验证。编译不等于 C1–C6 测试通过。若要根模块全套 `go test ./...`，先按真实前端构建流程准备 embed 资源，不能写空 index.html 绕过。

## 7. 发布前配置与可观测性

建议配置合同（字段名待 S1 固化）：全局 sticky_enabled=false；按精确虚拟模型、授权 group、用户/Token allowlist 启用；session_mode、允许的 ID 来源、TTL/lease/wait、容量与权重、memory/redis 模式、multi_key_policy（bind_index 或 exclude）、busy escape=false。所有时长/容量/权重验证上下限。不得从请求头开启功能或选择不授权候选。

第一版不改表。options 中新字段旧版本可忽略的行为必须实测；保留原配置快照和新增键清单，不依赖老二进制一定能解码新格式。新缓存用独立 v1 namespace/epoch，不覆盖既有 channel_affinity:v1。关闭功能时停止新 admission，已开始请求沿原 attempt 完成并释放；不能强行迁移已经输出的流。

建议指标：session source/conflict/no-ID、binding hit/miss/invalidated、claim wait/timeout、CAS reject、Redis error、migration reason、candidate inflight/capacity、lease lost/recovered、cooldown reject、attempt outcome、retry blocked(committed/private/unknown)、结算/退款异常。标签限于有限模型/候选/原因，session/token/user 不作高基数指标标签；审计只记录受限访问的脱敏关联摘要。

停止条件建议：任何跨租户命中、已提交后重试、重复扣款/误退款、真实测试外呼立即停用；lease 负数/不可回收、CAS 旧写覆盖也是硬阻断。错误率/等待 p95/429 增量的阈值与观察窗口需上线前按只读历史基线填写并审批；未填写不启用。不能在缺历史数据时编造改善百分比。

## 8. 发布 runbook 与回滚

### 8.1 已核实脚本能力与缺口

| 脚本 | 当前行为 | 本计划要求 |
| --- | --- | --- |
| `build_release_candidate_local.ps1` / `.sh` | 本地私有工具、版本化 release-cache、候选二进制/manifest、临时 source 清理 | 所有依赖、测试、编译均本地；不得在生产运行 build/install/test。 |
| `stage_release_runtime.sh <id>` | 默认 APP_ROOT=/opt/new-api；复制 `.env`，SQLite `.backup` 到 runtime；4003；NODE_TYPE=slave、清空 REDIS_CONN_STRING、关闭 pprof/batch | 这些措施不等于禁全部外呼。复制 DB 有真实渠道 key，复制 env 也有凭据；必须在启动前脱敏/禁任务，并有网络 deny egress。当前脚本立即启动，没有完整净化门禁，S7 修复前不得照搬执行。 |
| `smoke_release.sh <base-url> [db-path] [full\|fast]` | fast 读 `/`、`/api/status`、带 DB token 读 `/v1/models`；full 另发 Chat Completions 和 Responses `ping` | full 只能用于彻底隔离、假凭据、mock-only 候选；不得对生产或真实外部 API 做 full smoke。fast 也读真实 token，不能日志暴露。 |
| `cutover_release.sh <id>` | 备份 binary/SQLite，停服务、替换、重启，调用生产 fast smoke；失败可能自动恢复 DB | 是真实生产写入及停机；须逐项审批。无迁移版本建议显式禁止自动 DB 恢复，避免覆盖新业务写入。 |
| `rollback_release.sh <runtime>/cutover-backup.env` | 恢复 binary；RESTORE_DB=auto/1/0 控制 DB 恢复；GET health | 恢复 DB 有数据损失风险；无迁移版本默认 RESTORE_DB=0，DB 恢复需单独具体批准。 |
| `finalize_release.sh <id>` | 校验在线 binary/candidate/manifest hash；停 4003，删候选 env/DB/log，保留 candidate 和 cutover-backup.env/备份 | 验收证据先脱敏导出，确认回滚材料完整后 finalize；不可提前丢唯一证据。 |
| `cleanup_local_release.ps1 -ReleaseId <id>` | 删除本地 release、source worktree、项目缓存/前端产物，保留 release-cache；KeepCandidate、KeepCache、PurgeBuildCache 可选 | 接收验收及回滚材料另存后清理；KeepCandidate 仅临时保留，不作为任务结束遗留理由；不随意 PurgeBuildCache。 |

### 8.2 顺序与门禁

1. **实现起点（未来）**：确认工作区干净且父工作树用户文件不动；fetch origin/upstream，确认本地 `prod/251` 与 `origin/prod/251` 同步后，从更新后的生产分支创建 `codex/` 实现分支。不得 reset 生产分支丢定制，不开发/部署 main。本计划分支只承载文档，不把当前 HEAD 当未来永远最新。
2. **本地验证**：完成 S0–S6 和 S7 隔离修复，保存 Red/Green、C1–C6、DB/Redis 实际执行/skip、四客户端 capture 证据。缺 Prime 安装版证据不能宣称四客户端全覆盖。若批准缩小支持范围，PR 和 release notes 同步改范围。
3. **PR**：比较 git user.name/email 与 git log 历史核心作者，不改 git config；非核心作者在 PR 声明 AI-generated/assisted。使用 `.github/PULL_REQUEST_TEMPLATE.md` 原结构。评审合入 `prod/251`，推送/合并在未来实施授权范围内处理，本轮不做。绝不部署 upstream/main/origin/main。
4. **本地候选**：C7 构建已验证 prod commit；核对 manifest、binary SHA256、源 commit、配置摘要和工具版本。保留旧 binary/配置及回滚清单。将候选/manifest 接收到发布目录；不上传源码依赖树让生产编译。
5. **任何生产接触前**：先说明目标、只读/写入影响面、维护窗口、停止条件和回滚。即使只读也先说明影响；只读不改业务数据，撤销方式是停止观测。任何写入须在执行当下逐项确认具体目标、字段/记录/文件、旧值新值、时间、恢复方式；“继续/完成剩余步骤”不是批准。
6. **隔离 staging 门禁**：4003 只能读生产 DB 的副本，不共享生产 DB/日志 DB/Redis。S7 必须使净化在进程启动前完成：去除真实上游和外部服务凭据，关闭监控/自动测试/同步/后台任务/通知，仅保留 mock 渠道与假 token；网络 namespace/firewall deny egress，只允许审核过的 loopback mock，阻止真实 DNS/公网/内网生产出口。仅 NODE_TYPE=slave 不足。真实生产 DB 绝不写测试记录；测试 fixture 仅写隔离副本。现有 staging 只支持 SQLite 副本；MySQL/PostgreSQL 兼容性在本地沙箱验证，不能声称脚本支持远端三库复制。
7. **候选 smoke**：完成隔离证明后，执行下表候选命令，非流/流正常、断流、并发、Redis 故障均打本地 mock。只读观察 4003 schema hash、日志和端口，确认无真实外呼、无共享缓存；失败停止候选，保留脱敏诊断，不 cutover。
8. **cutover 审批**：列具体 binary hash、在线目标、备份路径、预计停机、新配置（仍关闭 sticky）、自动恢复行为，请用户当下批准。批准后才替换/重启。生产仅 fast/read-only smoke，不发合成模型请求或 canary；确认线上默认关闭行为。
9. **灰度审批**：另行列 exact virtual model、user/token/group allowlist、channel/model、容量/TTL、配置旧新值和观察窗口。仅被授权的真实业务流量参与，无测试/canary 请求。先一小组再扩展；每次范围扩大均明确授权边界。出安全异常立即按预批停止/回滚动作处理。
10. **验收与清理**：真实流量只读指标达到事先约定窗口/阈值，核对回滚材料已在受控位置保留；导出脱敏证据后 finalize 4003。候选上传接受且本地不再是唯一回滚/证据来源后执行本地 cleanup；可以在远端观察期间清本地，只要远端材料完备。若验收失败，不用要求在线 hash 匹配新候选的 finalize 强行清理，按失败收尾流程停止候选并保留回滚证据。最终删除临时 capture、DB、日志、依赖树，保留允许的 release-cache 和审核摘要。

下面是**已核实接口的未来命令模板**，必须先完成 S7 和上述逐项门禁；不是可立即执行的清单。所有占位值先替换并人工核对。在发布脚本所在受控 checkout 根运行，不重新使用源工作树做远端编译。

| 操作 | 命令模板 | 前提 |
| --- | --- | --- |
| stage | `timeout 120s bash scripts/stage_release_runtime.sh <release-id>` | 仅经 S7 修复的脚本、启动前隔离已验证，获 staging 具体授权 |
| 隔离 full smoke | `timeout 120s bash scripts/smoke_release.sh http://127.0.0.1:4003 releases/<release-id>/runtime/new-api.db full` | 净化副本、假凭据、mock-only，绝不换成生产 URL |
| cutover | `timeout 180s env AUTO_RESTORE_DB_ON_FAILURE=0 bash scripts/cutover_release.sh <release-id>` | 无 schema 迁移、备份已验、停机/替换/恢复动作已批准 |
| 只读 fast | `timeout 60s bash scripts/smoke_release.sh http://127.0.0.1:4002 /opt/new-api/data/new-api.db fast` | 已说明生产只读范围；端口/路径以实际核验为准，不暴露 token |
| rollback binary | `timeout 120s env RESTORE_DB=0 bash scripts/rollback_release.sh releases/<release-id>/runtime/cutover-backup.env` | 旧 binary 兼容当前 DB，回滚动作已批准 |
| finalize | `timeout 120s bash scripts/finalize_release.sh <release-id>` | 在线 hash 匹配、证据保存、回滚材料保留、获清理批准 |
| 本地 cleanup | `timeout 180s pwsh -NoProfile -File scripts/cleanup_local_release.ps1 -ReleaseId <release-id>` | 接受完成，唯一回滚/证据另存；默认保留 release-cache |

超时不代表状态未改变：尤其 cutover/rollback 超时后先检查进程、在线 hash、服务状态和备份收据，不盲目重跑。生产禁止安装依赖、build、运行仓库测试；远端编译异常流程不属于本计划默认路径。

### 8.3 回滚细节

优先关闭新 admission 开关，保留在途请求完成/取消及租约释放，不把同一流改派别处。恢复此前 options 快照（仅新增/改动字段，避免覆盖其他管理员同期变更），旧缓存 namespace 不变，新 namespace 停写并等 TTL；需要清理只删明确新 namespace/epoch，不能 FLUSHDB。旧 binary 不读取新 binding 格式，回退后丢粘滞属于可接受冷启动，不宣称状态无损。

binary 回滚核对 manifest/备份 hash；无迁移时保留现有 DB，避免回滚覆盖真实账目。如果发现不兼容 schema，停止自动流程，提交精确 DB 恢复方案、影响时间段和数据丢失评估，另获具体审批；不能用 RESTORE_DB=auto 替代审批。恢复后只读 GET status、只读日志/指标和真实业务反馈，不补发测试模型请求。回滚后也需回收候选端口、临时凭据/副本并保留审核需要的安全材料。

## 9. 待决项与评审签收

| 待决项 | 建议 | 未解决的影响 |
| --- | --- | --- |
| Codex thread/root | 默认独立 thread，root 可显式选 | S1/S6 身份合同不能最终签收 |
| fallback 与无 ID | 无 ID 按请求分配；内容指纹关闭；prompt_cache_key 低置信可配置 | 不能承诺 Hermes 默认严格会话粘滞 |
| 租户/Token 边界 | userID+tokenID 强隔离；Token 轮换冷绑定 | 跨 Token 会话需求需另设计授权 |
| 两渠道候选范围 | 明确 channelID、实际 model、endpoint、reasoning；避免全 Sources 混池 | 不得写生产配置或编造目标名 |
| 容量/权重/优先级 | 保留优先级层，层内按容量负载；同账号共享容量 | 缺真实配额依据不能安全启用 |
| 满载 escape/私有状态 | 默认等 2 秒后 429；私有状态无归属拒绝迁移 | 可用性与一致性取舍需批准 |
| TTL/lease/截止时间 | 30s/10s/60min 仅起点，按长流与故障预算调参 | 网络分区与长请求续约风险未关闭 |
| Redis 故障 | 多实例 fail-closed；memory 仅显式单实例 | 若要求 fail-open，需接受容量/粘滞降级并另测 |
| Prime/Hermes 适配 | Prime 安装版 capture；Hermes 独立仓库 logical ID 适配 | 四客户端验收未完成，不能声称都开箱即用 |
| 协议完成/计费证据 | Chat 与 Responses 分别定义终止，保留既有 usage | 部分流失败错误接线可能误退款，是发布阻断项 |
| 多 Key 渠道 | 候选、binding、容量都使用 accountIdentity；不能固定 key index 则排除 sticky | S2–S4 必须实现并测试 key 禁用失效，否则会跨账号复用或重复计容量 |
| AttemptOutcome 接线 | 协议 handler 单写 outcome，controller 单读，nil error 不再代表成功 | S5 前必须实现接口和所有 protocol 接线，否则部分流仍会误确认 |
| 发布隔离与观察阈值 | S7 先修 staging/smoke；上线前填写阈值/窗口/审批内容 | 原脚本不能直接安全执行 full smoke，生产门禁未获授权 |

评审签收应确认：各阶段目标与依赖可接受；上述语义已定或明确缩小范围；每个测试矩阵项有实际证据/明确阻断，不能把 skip 当通过；所有生产写入在实际执行时另行具体审批。

## 10. 本轮检查记录

目标路径被仓库 `.gitignore:24` 的 `plans` 规则忽略，因此普通 `git diff`/`git status` 不显示此新文件。本轮使用 `git diff --no-index /dev/null docs/plans/virtual-pool-sticky-session-tdd-plan.md` 检查完整新增内容，并用同命令的 `--check` 检查空白；未修改 ignore 规则、未暂存。未来获准提交时需显式纳入该文件。

### 10.1 实施与验证记录（本地工作树）

已实施：会话身份提取与 scope 隔离（S1）；middleware 一次 prepare + controller 复用、多 Key 稳定 index、候选失效复核（S2）；memory/Redis binding store、generation fencing、租约续期与原子 CAS（S3）；容量/权重/inflight 调度与 capacity lease（S4）；AttemptOutcome 接入 Chat/Responses/转换链路/Ollama/Claude/Gemini，已提交流禁止重试（S5）；本地 capture server 与客户端合同测试（S6 脚本）；staging/full smoke 隔离 acknowledgement 门禁（S7）。

本地已通过：

```bash
.local-tools/go/bin/go test -count=1 -timeout=240s \
  ./relay/channel ./service ./middleware ./controller \
  ./relay/common ./relay/helper ./relay/channel/openai ./relay
```

- `go vet` 覆盖同一组包，通过。
- `-race` 覆盖 `./service ./middleware ./controller ./relay/common ./relay/channel/openai`，通过；`./service` 全包 race 仍有既有 `task_polling_test.go` 竞态，`./relay/helper` 仍有既有 `stream_scanner_test.go` 竞态，均与本次改动无关。
- 真实 Redis 7 合同：`VIRTUAL_POOL_TEST_REDIS_ADDR=127.0.0.1:16379` 下 `TestVirtualPoolRedis` 全部通过（非 skip）。
- 三库合同：`TestVirtualPoolDatabaseContract` 在 SQLite、真实 MySQL 8、真实 PostgreSQL 16 上全部通过；`TestVirtualPoolPartialStreamSettlementIsNotRefunded` 通过。
- 新增 `controller/virtual_pool_e2e_test.go` 覆盖 middleware→controller→fake upstream：同会话同候选复用、每请求只 prepare 一次（并修复了 middleware 在 selection 之前读取 prepared route 导致 controller 二次 prepare 的真实缺陷）、已提交部分 SSE 第二上游调用数为 0。
- 新增 `previous_response_id` 归属闭环：成功响应记录 `response id -> candidate`（memory/Redis，按 user/token/group/model scope 隔离）；续接请求在选候选前强制命中原 candidate，未知或不可用归属返回 409，不跨上游；覆盖 round-robin、未知 ID、已提交部分流三种 E2E。
- 新增虚拟路由健康 store：memory 保持原单实例行为，Redis 模式使用原子脚本共享连续失败计数与冷却时间；覆盖两个 client 的跨实例冷却与恢复。
- 新增 Codex thread/root 行为单测：thread 模式同 root 不同 thread 分开绑定，root 模式显式合并；resume/compact 依赖同一 session/thread，fork 使用新 ID。
- `gofmt` 与 `git diff --check` 通过。

仍未完成：C5 四个客户端安装版（Hermes、Pi、Codex、Prime）的新建/resume/fork/compact 实际抓取；C7 本地 Linux amd64 候选构建（当前工作树无 `.local-tools/bun/bun`，前端 embed 构建未验证）。未提交、未推送、未部署。

本地实施已完成到上述边界；提交、推送、部署、生产配置变更和真实模型调用均需另行明确授权。
