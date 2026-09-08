# Channel Samples

Use this fixed matrix for `standard` verification. Do not auto-pick substitute channels or models.

| Sample Id | Channel Label | Required In Standard | Model | Endpoint | Expected Result |
| --- | --- | --- | --- | --- | --- |
| deepseek-baseline | deepseek | yes | `deepseek-v4-flash` | `/v1/chat/completions` | Request succeeds through the dedicated deepseek route |
| ollama-baseline | ollama | yes | `gpt-oss:20b` | `/v1/chat/completions` | Request succeeds through the ollama cloud channel |

## Rules
- Keep this matrix fixed until a human explicitly changes it.
- Fail the check if a required sample cannot be executed.
- Report the sample ID, model, endpoint, and observed failure mode.
- Do not silently swap `deepseek-v4-flash` or `gpt-oss:20b` for a different model.
