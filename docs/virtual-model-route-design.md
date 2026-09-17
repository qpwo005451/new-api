# 虚拟模型路由（Virtual Model Route）设计说明

状态：已在 `prod/251` 使用（`auto-subagent`、`auto-subagent-codex`）；本文补充"轮询聚合"能力
范围：NewAPI 网关内的对外模型名到模型池的路由；不改变上游 provider 自身的路由
日期：2026-09-17

## 1. 目标

用一个对外模型名聚合多个上游模型，形成"模型池"，并支持：

- 每个请求按策略从池中选起点（有序 / 随机 / 轮询）；
- 该请求失败时沿池继续尝试下一个候选，实现自动切换模型（failover）；
- 单请求的尝试次数可上限，避免一个请求把整个池走一遍造成长尾延迟。

典型用途：把 `OpenRouter Free` 与 `NVIDIA Free` 两条渠道的免费模型聚合成一个
`auto-free`，对外表现为一个模型名，内部自动分摊与容错。

与 OpenRouter 的 `openrouter/free` 的差别：这里的池由本网关维护，成员是具体上游模型；
挑选与重试全部发生在本网关内部，不依赖上游的路由策略。

## 2. 配置

配置项：`model_retry_policy_setting.virtual_model_routes`（options 表，点号键）。
更新该 option 立即生效，无需重启。

```json
{
  "auto-free": {
    "rotation": "round_robin",
    "max_attempts": 3,
    "health": { "enabled": true, "failure_threshold": 2, "cooldown_seconds": 60, "max_cooldown_seconds": 600 },
    "targets": [
      { "model": "inclusionai/ling-3.0-flash-vl:free" },
      { "model": "z-ai/glm-5.2:free" },
      { "model": "nvidia/nemotron-3-super-120b-a12b" }
    ]
  }
}
```

字段：

| 字段 | 取值 | 说明 |
| --- | --- | --- |
| `rotation` | `ordered` / `random` / `round_robin` | 首试在池中的起点选择。缺省 `ordered`，即始终从池首开始 |
| `max_attempts` | 正整数 | 单请求最多尝试的池条目数。缺省或 0 表示可以走完整个池 |
| `targets[].model` | 上游模型名 | 池成员，必须是某条渠道真实可路由的模型名 |
| `targets[].reasoning_effort_map` | 可选映射 | 按客户端 `reasoning.effort` / `reasoning_effort` 改成该目标可接受的取值 |
| `health` | 可选对象 | 按真实请求结果做"健康优先"，见第 4 节 |

兼容性：`"name": [ ... ]` 这种旧的目标数组写法仍然接受，等价于 `ordered` + 无上限。

## 3. 行为

候选池构造顺序：遍历 `targets` → 对每个目标按 token 分组（`auto` 分组时按 auto 分组顺序）→
取该分组的可用渠道，按 优先级降序、权重降序、渠道 ID 升序 排列，跨分组按渠道去重。
因此池条目是「目标模型 × 渠道」的组合。

- 首试位置：`ordered` 取 0；`random` 取随机位置；`round_robin` 取该虚拟模型的自增计数位置。
- 第 N 次尝试位置：`(起点 + N) % 池大小`。
- 单请求尝试上限：`min(max_attempts, 池大小)`；未配置时等于池大小。
- 该上限会覆盖全局 `RetryTimes`（虚拟路由自己决定可重试次数）。
- 池为空或尝试次数用尽时返回 `ErrPriorityFallbackExhausted`，由调用方决定最终错误。
- 命中候选后，请求会被改写为对应上游模型名（并写入虚拟 reasoning effort），
  通过渠道 model_mapping 合并后发送上游。

## 4. 可用性优选（可选）

`health` 打开后，路由会用**本网关自己的请求结果**判断池条目是否还在服务，不需要探针、新表或额外请求：

```json
"health": {
  "enabled": true,
  "failure_threshold": 2,
  "cooldown_seconds": 60,
  "max_cooldown_seconds": 600
}
```

| 字段 | 默认 | 说明 |
| --- | --- | --- |
| `enabled` | `false` | 关闭时不改变池顺序（保持原有行为） |
| `failure_threshold` | `2` | 连续多少次"结构性问题"后进入冷却 |
| `cooldown_seconds` | `60` | 首次冷却时长 |
| `max_cooldown_seconds` | `600` | 连续失败时冷却按 2 倍递增的上限 |

- 计入冷却的状态码：`429`、`404`、`>=500`。客户端问题（400/401/403/499 等）不计入。
- 冷却中的条目**不会**被移出池，而是排到池尾：健康成员优先，只有它们都失败时才会尝试冷却成员。
- 命中成功即清零冷却；冷却到期后条目自动回到健康部分，若再次失败则继续按 2 倍递增，直到上限。
- 状态是**进程内**的：重启归零，多实例各自维护，冷启动阶段所有条目都视为健康。
- 与 `model_monitor_*` 的关系：监控是独立的可用性观测（当前未覆盖免费渠道），本机制只消费真实请求结果，两者互不依赖。

## 5. 可见性、权限与计费

- 路由的真相源是虚拟路由配置 + 目标模型在各渠道的 abilities；虚拟名本身**不需要**有 ability 才能路由。
- 但 `/v1/models`、token 模型白名单、价格页都按"模型"维度工作，所以对外暴露的虚拟名
  仍应像现有 `auto-subagent` 那样挂到渠道的 `models` 里，并在该渠道配置 `model_mapping`
  指向一个具体模型作为兜底（例如 `{"auto-free":"z-ai/glm-5.2:free"}`）。
- 计费按客户端请求的模型名（虚拟名）计算，因此虚拟名需要价格/倍率配置；
  自用模式（`SelfUseModeEnabled`）或分组倍率为 0 时不额外收费。

## 6. 运维注意

- 修改该 option 立即生效；不要直连 SQLite 改 `channels.models`（不会重建 abilities）。
- `round_robin` 的计数是**进程内**状态：多实例部署时各实例独立轮询，不构成全局严格轮询；
  需要无共享状态的均匀分摊时可用 `random`。
- 池内成员失效（上游下线）不会自动剔除，建议按周期对免费池做目录 diff 并更新 targets。
- 与 `single_pass_priority_models` 的区别：后者控制"同一模型是否只走每个优先级一次"，
  不改变模型名；虚拟路由改变的是"用哪个上游模型"。
