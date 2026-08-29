export type ClientIdentitySource =
  | 'alias'
  | 'known'
  | 'declared'
  | 'sdk'
  | 'unknown'

export interface ClientIdentity {
  source: ClientIdentitySource
  label: string
}

export interface ClientIdentityContext {
  aliases?: Record<string, string> | null
  clientTitle?: string
}

// Known client tools that put their own name in the User-Agent. Ordered and
// matched case-insensitively; first hit wins.
const KNOWN_CLIENTS: Array<{ pattern: RegExp; name: string }> = [
  { pattern: /codex_cli_rs/i, name: 'Codex CLI' },
  { pattern: /claude-cli/i, name: 'Claude Code' },
  { pattern: /geminicli/i, name: 'Gemini CLI' },
  { pattern: /cherrystudio/i, name: 'Cherry Studio' },
  { pattern: /lobechat/i, name: 'LobeChat' },
  { pattern: /nextchat/i, name: 'NextChat' },
  { pattern: /librechat/i, name: 'LibreChat' },
  { pattern: /sillytavern/i, name: 'SillyTavern' },
  { pattern: /openwebui/i, name: 'Open WebUI' },
  { pattern: /chatbox/i, name: 'Chatbox' },
  { pattern: /dify/i, name: 'Dify' },
  { pattern: /cline/i, name: 'Cline' },
  { pattern: /roo-?code/i, name: 'Roo Code' },
  { pattern: /kilocode/i, name: 'Kilo Code' },
]

// SDK/runtime fingerprints for user agents that identify the HTTP stack but
// not the application using it.
const SDK_FINGERPRINTS: Array<{
  pattern: RegExp
  label: (match: RegExpMatchArray) => string
}> = [
  {
    pattern: /^OpenAI\/JS[/ ]v?([\d.]+)/i,
    label: (m) => `OpenAI JS SDK ${m[1]}`,
  },
  {
    pattern: /^OpenAI\/Python[/ ]v?([\d.]+)/i,
    label: (m) => `OpenAI Python SDK ${m[1]}`,
  },
  { pattern: /python-httpx\/([\d.]+)/i, label: (m) => `Python httpx ${m[1]}` },
  {
    pattern: /python-requests\/([\d.]+)/i,
    label: (m) => `Python requests ${m[1]}`,
  },
  { pattern: /aiohttp\/([\d.]+)/i, label: (m) => `Python aiohttp ${m[1]}` },
  { pattern: /axios\/([\d.]+)/i, label: (m) => `axios ${m[1]}` },
  { pattern: /okhttp\/([\d.]+)/i, label: (m) => `OkHttp ${m[1]}` },
  { pattern: /Apache-HttpClient/i, label: () => 'Java HttpClient' },
  { pattern: /^Go-http-client/i, label: () => 'Go net/http' },
  { pattern: /^curl\/([\d.]+)/i, label: (m) => `curl ${m[1]}` },
  { pattern: /^Wget/i, label: () => 'Wget' },
]

// resolveClientIdentity classifies a request's client from the strongest
// available signal: an admin-defined alias (see /api/log/client_aliases),
// a well-known client User-Agent, an app-declared title (X-Title), an SDK
// fingerprint, or the raw User-Agent as a last resort.
export function recognizeUserAgent(
  userAgent: string | null | undefined,
  context: ClientIdentityContext = {}
): ClientIdentity {
  const raw = (userAgent ?? '').trim()
  if (!raw) return { source: 'unknown', label: '' }

  const alias = context.aliases?.[raw]
  if (alias) return { source: 'alias', label: alias }

  for (const client of KNOWN_CLIENTS) {
    if (client.pattern.test(raw)) {
      return { source: 'known', label: client.name }
    }
  }

  const declaredTitle = (context.clientTitle ?? '').trim()
  if (declaredTitle) return { source: 'declared', label: declaredTitle }

  for (const fingerprint of SDK_FINGERPRINTS) {
    const match = raw.match(fingerprint.pattern)
    if (match) return { source: 'sdk', label: fingerprint.label(match) }
  }

  return { source: 'unknown', label: raw }
}
