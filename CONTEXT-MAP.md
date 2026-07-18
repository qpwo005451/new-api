# Context Map

## Contexts

- [Channel Balance Protection](./CONTEXT.md) — how a channel's model availability changes when upstream balance is low
- [In-Flight Usage Logging](./docs/domain/in-flight-usage-log/CONTEXT.md) — usage-log visibility for requests still talking to upstream

## Relationships

- **In-Flight Usage Logging → Channel Balance Protection**: none required for v1; a pending usage log may later show the channel that balance protection filtered or selected, but the two policies are independent
