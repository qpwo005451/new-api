import { describe, expect, test } from 'vitest'

import { recognizeUserAgent } from './client-info'

describe('recognizeUserAgent', () => {
  test('prefers the admin alias over every other signal', () => {
    expect(
      recognizeUserAgent('OpenAI/JS 6.47.0', {
        aliases: { 'OpenAI/JS 6.47.0': 'Prime Agent' },
        clientTitle: 'Something Else',
      })
    ).toEqual({ source: 'alias', label: 'Prime Agent' })
  })

  test('maps known client user agents to their display names', () => {
    expect(
      recognizeUserAgent('codex_cli_rs/0.42.0 (Ubuntu 22.04; x86_64)')
    ).toEqual({ source: 'known', label: 'Codex CLI' })
    expect(recognizeUserAgent('claude-cli/1.0.83 (external, cli)')).toEqual({
      source: 'known',
      label: 'Claude Code',
    })
    expect(recognizeUserAgent('GeminiCLI/v18.0.0 (linux; x64)')).toEqual({
      source: 'known',
      label: 'Gemini CLI',
    })
    expect(recognizeUserAgent('CherryStudio/1.4.5')).toEqual({
      source: 'known',
      label: 'Cherry Studio',
    })
  })

  test('uses the app-declared title for generic SDK user agents', () => {
    expect(
      recognizeUserAgent('OpenAI/JS 6.47.0', { clientTitle: 'Prime Agent' })
    ).toEqual({ source: 'declared', label: 'Prime Agent' })
  })

  test('labels recognizable SDK and runtime fingerprints', () => {
    expect(recognizeUserAgent('OpenAI/JS 6.47.0')).toEqual({
      source: 'sdk',
      label: 'OpenAI JS SDK 6.47.0',
    })
    expect(recognizeUserAgent('OpenAI/Python 1.2.3')).toEqual({
      source: 'sdk',
      label: 'OpenAI Python SDK 1.2.3',
    })
    expect(recognizeUserAgent('python-httpx/0.27.0')).toEqual({
      source: 'sdk',
      label: 'Python httpx 0.27.0',
    })
    expect(recognizeUserAgent('Go-http-client/1.1')).toEqual({
      source: 'sdk',
      label: 'Go net/http',
    })
    expect(recognizeUserAgent('curl/8.7.1')).toEqual({
      source: 'sdk',
      label: 'curl 8.7.1',
    })
  })

  test('falls back to the raw user agent', () => {
    expect(recognizeUserAgent('SomeRandomAgent/9.9')).toEqual({
      source: 'unknown',
      label: 'SomeRandomAgent/9.9',
    })
    expect(recognizeUserAgent('   ')).toEqual({ source: 'unknown', label: '' })
    expect(recognizeUserAgent(undefined)).toEqual({
      source: 'unknown',
      label: '',
    })
  })
})
