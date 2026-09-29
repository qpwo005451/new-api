# Local Change Register

| Change | Current Production Location | Fork Disposition | Notes |
| --- | --- | --- | --- |
| Input billing alias pricing | `relay/helper/price.go` | commit as source change | Channel 9 bills selected models as `input/*` aliases |
| Input pricing tests | `relay/helper/price_test.go` | commit as source change | Covers alias and tiered billing behavior |
| Stream ping timeout 60s | `relay/helper/stream_scanner.go` | commit as source change | Replaces the 10s timeout for long-running streams |
| Image-generation tool filter | `relay/responses_handler.go` | commit as source change | Prevents unsupported `image_generation` tool relay failures |
| Option override helper | `patches/apply-local-option-overrides.py` | keep as tracked helper | Applies DB-backed local option overrides |
| Option override manifest | `patches/local-option-overrides.json` | keep as tracked helper | Carries DeepSeek V4 pricing and `kimi-k2.7-code` billing overrides |
| Image filter patch helper | `patches/patch-image-gen-filter.py` | keep as tracked helper | Retained as legacy replay helper during migration |
| Input channel guard | `scripts/channel_guard.py` | keep as tracked helper | Manages the input route state after upstream daily-limit failures |
| Legacy input budget guard | `scripts/input_budget_guard.py` | keep as tracked helper | Historical guard retained for reference |
| Watchdog shell | `watchdog.sh` | keep as tracked helper; **cron disabled on host 251 since 2026-09-30** | Relay health check script with server-only key file; its relay probe restarted `new-api` every ~20 minutes while upstream channels were unstable, so the cron entry is commented out and only the systemd unit setting `Restart=on-failure` remains |
| Live relay timeout | `/opt/new-api/.env` | document only | `RELAY_TIMEOUT=900` remains server-only runtime config |
| Claude Opus 5 thinking-disable guard | `relaykit/relayconvert/reasoning/claude.go` | commit as source change | `claude-opus-5*` sets `supportsDisable=false`, so `reasoning_effort: "none"` keeps adaptive thinking instead of deriving `thinking.type="disabled"`, which that family rejects with 400 |
| Claude default effort tier | `relaykit/relayconvert/reasoning/claude.go` | commit as source change | Requests that carry no effort signal are accounted at the family default: Opus 5.5 medium, Fable 5.1 high (`claudeDefaultEffort`, mirrors `geminiDefaultEffort`) |
