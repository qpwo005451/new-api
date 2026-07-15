# Channel Samples

Use this fixed matrix for `standard` verification. Do not auto-pick substitute channels or models.

| Sample Id | Channel Label | Required In Standard | Model | Endpoint | Expected Result |
| --- | --- | --- | --- | --- | --- |
| input-baseline | input | yes | `gpt-5.4-mini` | `/v1/chat/completions` | Request succeeds through the primary input-backed OpenAI-compatible path |
| deepseek-baseline | deepseek | yes | `deepseek-v4-flash` | `/v1/chat/completions` | Request succeeds through the dedicated deepseek route |

## Rules
- Keep this matrix fixed until a human explicitly changes it.
- Fail the check if a required sample cannot be executed.
- Report the sample ID, model, endpoint, and observed failure mode.
- Do not silently swap `gpt-5.4-mini` or `deepseek-v4-flash` for a different model.
