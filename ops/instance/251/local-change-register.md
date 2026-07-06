# Local Change Register

| Change | Current Production Location | Fork Disposition | Notes |
| --- | --- | --- | --- |
| Input billing alias pricing | `relay/helper/price.go` | commit as source change | Channel 9 bills selected models as `input/*` aliases |
| Input pricing tests | `relay/helper/price_test.go` | commit as source change | Covers alias and tiered billing behavior |
| Stream ping timeout 60s | `relay/helper/stream_scanner.go` | commit as source change | Replaces the 10s timeout for long-running streams |
| Image-generation tool filter | `relay/responses_handler.go` | commit as source change | Prevents unsupported `image_generation` tool relay failures |
| Option override helper | `patches/apply-local-option-overrides.py` | keep as tracked helper | Applies DB-backed local option overrides |
| Option override manifest | `patches/local-option-overrides.json` | keep as tracked helper | Currently carries `kimi-k2.7-code` billing override |
| Image filter patch helper | `patches/patch-image-gen-filter.py` | keep as tracked helper | Retained as legacy replay helper during migration |
| Unified channel guard | `scripts/channel_guard.py` | keep as tracked helper | Manages input and fog channel state |
| Legacy input budget guard | `scripts/input_budget_guard.py` | keep as tracked helper | Historical guard retained for reference |
| Watchdog shell | `watchdog.sh` | keep as tracked helper | Relay health check script with server-only key file |
| Live relay timeout | `/opt/new-api/.env` | document only | `RELAY_TIMEOUT=900` remains server-only runtime config |
