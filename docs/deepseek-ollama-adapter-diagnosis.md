# DeepSeek Ollama 适配器异常诊断

## 目的

本文档用于移交 NewAPI 侧继续定位 Prime Agent 使用 `deepseek-v4-flash` 时出现的两类异常：

- 单次响应中重复生成大量工具调用；
- thinking 内容重复，直到达到输出上限后以 `length` 中断。

本文档只记录脱敏后的诊断信息，不包含 API key、token、完整生产配置、请求正文或上游真实 URL。

## 当前链路

```text
Prime Agent
  OpenAI Chat Completions
        |
        v
NewAPI
  channel 46, Ollama adapter
  /v1/chat/completions -> Ollama /api/chat
        |
        v
Ollama Cloud
  deepseek-v4-flash:0731
```

远端 NewAPI 运行方式：systemd 服务 `new-api.service`，监听端口 `4002`。检查时生产源码 HEAD 为：

```text
d36f64eaf1332fd3a30793de2ca16f7b6ca5636c
```

尚未证明运行中的二进制一定与该源码完全一致。

## Prime Agent 侧现象

目标会话：

```text
/home/ra/.prime/agent/sessions/01a07e72-ad81-73da-a616-e4ca70f4de28.jsonl
```

### 工具调用循环

某次 assistant 响应中：

- 输入约 `103746` tokens；
- 输出正好 `32000` tokens；
- `stopReason` 保存为 `toolUse`；
- 共 `559` 个工具调用；
- 其中 `558` 个调用参数完全相同，但 call ID 不同。

此前在输入约 `56K` 和 `91K` 时，也分别出现过约 `222` 和 `214` 个工具调用。

### thinking 重复并中断

会话后来切换到 `high`，NewAPI 日志也记录到 `reasoning_effort: high`。随后出现两次：

| 时间（UTC） | 输入 tokens | 输出 tokens | 结果 |
|---|---:|---:|---|
| 05:44:47 | 80840 | 16384 | `length`，只有 thinking |
| 05:51:53 | 80844 | 16384 | `length`，只有 thinking |

两次响应中 thinking 存在长段落重复：第一轮约重复 47 次，第二轮约重复 56 至 57 次。没有生成最终正文，也没有工具调用。

## NewAPI 侧已观察到的记录

对应两次 `high` 响应，NewAPI consume log 记录：

- channel id：`46`；
- model：`deepseek-v4-flash`；
- upstream model：`deepseek-v4-flash:0731`；
- request path：`/v1/chat/completions`；
- request conversion：`OpenAI Compatible`；
- reasoning effort：`high`；
- prompt tokens：`80840`、`80844`；
- completion tokens：均为 `16384`；
- streaming：`true`。

NewAPI 日志还出现过一次 `504 upstream response timed out`。当时环境中的：

```text
RELAY_TIMEOUT=900
STREAMING_TIMEOUT=300
```

## 源码确认的行为

检查文件：

```text
relay/channel/ollama/relay-ollama.go
relay/channel/ollama/stream.go
relay/channel/ollama/dto.go
```

### 1. thinking 和 content 没有累计帧去重

流式转换逐帧读取 Ollama NDJSON，并将字段转为 OpenAI SSE delta。非流式聚合逻辑则直接对每一帧执行：

- `Message.Content` 追加到正文；
- `Message.Thinking` 追加到 reasoning content；
- `Message.ToolCalls` 追加到工具调用列表。

代码假设上游每帧都是增量内容。如果上游发送累计内容，例如：

```text
A
AB
ABC
```

NewAPI 会生成：

```text
AABABC
```

thinking 同理。这是已确认的协议兼容风险，足以制造重复内容。

### 2. 工具调用跨帧累加并重新编号

`stream.go` 使用运行中的 `toolCallIndex`。每个帧中的工具调用都会递增 index。缺少 ID 时会生成 `call_<index>`。

如果上游重复发送同一个工具调用，NewAPI 会把它作为多个调用转发，并可能生成不同的 ID。这可以解释 Prime Agent 看到大量不同 call ID 的重复调用。

### 3. 结束原因可能被错误覆盖

只要转换过程中出现过工具调用，流式和非流式逻辑都会把最终结束原因强制设为 `tool_calls`，即使 Ollama done 帧的 `done_reason` 是 `stop` 或 `length`。

这会使客户端误以为响应正常结束于工具调用，并继续执行工具或发起下一轮。

### 4. `max_tokens` 的语义

OpenAI 请求中的 `max_tokens` 被转换为 Ollama：

```json
{"options":{"num_predict":16384}}
```

这限制的是整个生成过程，包含 thinking 和最终答案，不是只限制最终正文。因此 thinking 重复时，会在生成正文前消耗完预算。

### 5. `parallel_tool_calls` 没有映射

Prime Agent 发出的 `parallel_tool_calls: false` 不会出现在当前 Ollama 请求结构中，适配器 DTO 和转换函数均未处理该字段。该参数被静默丢弃。

这是真实存在的兼容性缺口，但目前不能单独证明它是本次重复 thinking 的根因。

### 6. usage 不是跨帧累加

`eval_count` 取自 Ollama done 帧并使用一次。`completion_tokens=16384` 与 Ollama done 帧的 `eval_count` 一致，说明 NewAPI 没有因为重复拼接而重复计算 token。

## 当前根因判断

当前最强假设是：

```text
Ollama Cloud 或其上游模型返回异常重复帧、累计帧或重放帧
        |
        v
NewAPI Ollama adapter 按增量帧直接追加，且不去重
        |
        v
重复 thinking、重复 content、重复 tool calls
        |
        v
Prime Agent 继续执行工具或耗尽 thinking 输出预算
```

已确认 NewAPI 适配器具备放大该问题的行为，但还缺少一项闭环证据：Ollama Cloud 返回给 NewAPI 的原始 NDJSON。

## 建议的验证方法

对一次可控、脱敏的请求同时保存以下两层流：

1. Ollama 原始 NDJSON；
2. NewAPI 输出给客户端的 OpenAI SSE。

不要记录 API key、Authorization、完整 messages、工具参数或业务正文。可以只记录每帧的：

- 帧序号；
- `done`；
- `done_reason`；
- thinking/content 的长度、hash 和前缀 hash；
- 工具 name、ID、arguments hash；
- `eval_count`；
- request ID。

判定规则：

- Ollama 原始帧已经累计或重复：根因在 Ollama Cloud、模型或其上游重放；NewAPI 是放大器。
- Ollama 原始帧是正常增量，NewAPI SSE 出现重复：根因在 NewAPI 适配器。
- NewAPI SSE 正常，Prime Agent 最终消息重复：根因在 Prime Agent 客户端流式累加。

## 建议添加的离线回归测试

在 `relay/channel/ollama` 增加确定性测试，至少覆盖：

1. 两个累计 content 帧，确认不会产生 `AAB`；
2. 两个累计 thinking 帧，确认不会重复；
3. 相同工具调用在多个帧重复出现时，不会生成多个调用；
4. 工具调用带 ID 和不带 ID 的情况；
5. `done_reason=length` 且曾出现工具调用时，结束原因不会被无条件改成 `tool_calls`；
6. 增量帧仍保持原有行为；
7. `eval_count` 只计一次；
8. `parallel_tool_calls=false` 的 OpenAI 请求是否需要映射为 Ollama 支持的等价控制。

推荐先确定 Ollama API 对 `message.content`、`message.thinking`、`message.tool_calls` 的流式语义：是增量、累计，还是模型/版本相关。不要在未确定协议前盲目加入通用字符串去重，因为真实增量文本可能合法重复。

## 低风险处理顺序

1. 先用脱敏观测确认 Ollama 原始帧形态；
2. 在本地添加适配器回归测试；
3. 修复累计帧处理和工具调用去重，或明确适配器只接受增量格式并在异常时失败；
4. 修复结束原因覆盖逻辑，优先保留上游 `done_reason`；
5. 评估 `parallel_tool_calls` 映射；
6. 本地构建并在隔离环境验证；
7. 通过现有发布、部署和回滚流程上线。

本次调查没有修改生产 NewAPI，没有重启服务，没有发送推理测试请求。
