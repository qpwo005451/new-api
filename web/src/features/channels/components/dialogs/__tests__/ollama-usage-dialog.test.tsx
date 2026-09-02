/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { render, screen } from '@testing-library/react'
import dayjs from 'dayjs'
import { describe, expect, test } from 'vitest'

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { OllamaUsageDialog } = await import('../ollama-usage-dialog')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  nsSeparator: false,
  resources: {
    en: {
      translation: {
        'Ollama Usage': 'Ollama Usage',
        Close: 'Close',
        Refresh: 'Refresh',
        Copy: 'Copy',
        'Raw JSON': 'Raw JSON',
        'Upstream Usage': 'Upstream Usage',
        'Local Estimate': 'Local Estimate',
        '5-Hour Window': '5-Hour Window',
        'Weekly Window': 'Weekly Window',
        'Reported by Ollama': 'Reported by Ollama',
        'Usage units': 'Usage units',
        '{{count}} requests': '{{count}} requests',
        'No requests in this window': 'No requests in this window',
        "Estimated from this channel's request logs":
          "Estimated from this channel's request logs",
        'Weighted tokens': 'Weighted tokens',
        'Total tokens:': 'Total tokens:',
        'Requests:': 'Requests:',
        'Upstream requests:': 'Upstream requests:',
        'Prompt {{value}} / Completion {{value2}}':
          'Prompt {{value}} / Completion {{value2}}',
        Low: 'Low',
        Medium: 'Medium',
        High: 'High',
        'Extra High': 'Extra High',
        Unknown: 'Unknown',
        '4-week activity cost:': '4-week activity cost:',
        'Fetched at:': 'Fetched at:',
        'Failed to fetch usage': 'Failed to fetch usage',
        'Weighted tokens estimate upstream usage units as model level × (prompt + completion) tokens, because Ollama does not publish the cap or the unit. Upstream and local request counts are directly comparable; mismatches mean some upstream traffic in this window predates the current channel or key.':
          'Weighted tokens estimate upstream usage units as model level × (prompt + completion) tokens, because Ollama does not publish the cap or the unit. Upstream and local request counts are directly comparable; mismatches mean some upstream traffic in this window predates the current channel or key.',
        'Recovery projection (estimate)': 'Recovery projection (estimate)',
        'Earliest release:': 'Earliest release:',
        'Ollama Monitor Snapshot': 'Ollama Monitor Snapshot',
        'From the monitored ollama.com settings page':
          'From the monitored ollama.com settings page',
        Used: 'Used',
        'Resets at:': 'Resets at:',
        'Resets in:': 'Resets in:',
        'Snapshot fetched at:': 'Snapshot fetched at:',
        Stale: 'Stale',
        'Recovery timing and the projection are estimates based on when this channel’s own requests slide out of each window.':
          'Recovery timing and the projection are estimates based on when this channel’s own requests slide out of each window.',
        'Ollama resets usage server-side on its own schedule and does not expose the reset time through the usage API.':
          'Ollama resets usage server-side on its own schedule and does not expose the reset time through the usage API.',
      },
    },
  },
})

function DialogHarness(props: {
  response: Record<string, unknown> | null
  channelId?: number
}) {
  return (
    <I18nextProvider i18n={i18n}>
      <OllamaUsageDialog
        open
        onOpenChange={() => {}}
        channelName='ollama_pro'
        channelId={props.channelId ?? 46}
        response={props.response as never}
      />
    </I18nextProvider>
  )
}

function successfulResponse() {
  return {
    success: true,
    message: '',
    data: {
      channel_id: 46,
      fetched_at: 1788050524,
      upstream: {
        session: {
          usage: 30,
          models: [
            { name: 'gpt-oss:20b', request_count: 2 },
            { name: 'glm-5.3-flash', request_count: 1 },
          ],
        },
        weekly: { usage: 300, models: [] },
        activity: {
          cost: '0.00000',
          period: { type: 'last_4_weeks' },
          models: [],
        },
      },
      local: {
        session: {
          window_seconds: 18000,
          since: 1788033000,
          models: [
            {
              model_name: 'gpt-oss:20b',
              requests: 2,
              prompt_tokens: 500,
              completion_tokens: 100,
              level: 1,
            },
            {
              model_name: 'glm-5.3-flash',
              requests: 1,
              prompt_tokens: 2000,
              completion_tokens: 1000,
              level: 2,
            },
          ],
          total_tokens: 3600,
          weighted_usage: 6600,
        },
        weekly: {
          window_seconds: 604800,
          since: 1787446000,
          models: [],
          total_tokens: 3600,
          weighted_usage: 6600,
        },
      },
    },
  }
}

describe('Ollama usage dialog', () => {
  test('renders upstream windows, local per-model estimates, and request comparisons', () => {
    render(<DialogHarness response={successfulResponse()} />)

    expect(screen.getByText('Upstream Usage')).toBeInTheDocument()
    expect(screen.getByText('Local Estimate')).toBeInTheDocument()
    // Upstream session usage value and per-model request counts
    // (model names and counts appear in both the upstream and local sections).
    expect(screen.getAllByText('30')).toHaveLength(1)
    expect(screen.getAllByText('2 requests').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('gpt-oss:20b').length).toBeGreaterThanOrEqual(2)
    expect(screen.getAllByText('glm-5.3-flash').length).toBeGreaterThanOrEqual(
      2
    )
    // Local weighted tokens, local request total (3), and upstream request
    // total for the same window (2 + 1).
    expect(screen.getAllByText('6,600')).toHaveLength(2)
    expect(screen.getAllByText('Requests:').length).toBe(2)
    expect(screen.getAllByText('3').length).toBeGreaterThanOrEqual(1)
    // The session window has upstream model counts, so it shows the upstream
    // request total next to its own; the upstream weekly window has no model
    // list, so no upstream count is invented for it.
    expect(screen.getAllByText('Upstream requests:').length).toBe(1)
    // The unit-soup upstream-usage ratio is gone.
    expect(screen.queryByText('0.0045')).not.toBeInTheDocument()
    // Level badges come from the response level field.
    expect(screen.getAllByText('Low').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('Medium').length).toBeGreaterThanOrEqual(1)
    // No recovery section when the payload carries no projection data.
    expect(
      screen.queryByText('Recovery projection (estimate)')
    ).not.toBeInTheDocument()
  })

  test('renders the recovery projection and earliest release for windows that carry one', () => {
    const response = successfulResponse()
    const session = (
      response.data.local as { session: Record<string, unknown> }
    ).session
    session.earliest_release_at = 1788050524 + 18000
    session.projection = {
      bucket_seconds: 3600,
      points: [
        { after_seconds: 3600, weighted_usage: 5000, requests: 2 },
        { after_seconds: 7200, weighted_usage: 0, requests: 0 },
      ],
    }

    render(<DialogHarness response={response} />)

    expect(screen.getAllByText('Recovery projection (estimate)')).toHaveLength(
      1
    )
    expect(screen.getByText('Earliest release:')).toBeInTheDocument()
    expect(
      screen.getByText(
        dayjs((1788050524 + 18000) * 1000).format('YYYY-MM-DD HH:mm:ss')
      )
    ).toBeInTheDocument()
    // First hour keeps projected remaining usage, the second hour is empty.
    expect(
      screen.getByTitle(/\+1h · 5,000 Usage units · 2 requests/)
    ).toBeInTheDocument()
    expect(
      screen.getByTitle(/\+2h · 0 Usage units · 0 requests/)
    ).toBeInTheDocument()
  })

  test('shows the upstream error message and no usage sections when the response failed', () => {
    render(
      <DialogHarness
        response={{
          success: false,
          message: '获取 Ollama 用量失败: status code: 401',
        }}
      />
    )

    expect(
      screen.getByText('获取 Ollama 用量失败: status code: 401')
    ).toBeInTheDocument()
    expect(screen.queryByText('Upstream Usage')).not.toBeInTheDocument()
    expect(screen.queryByText('Local Estimate')).not.toBeInTheDocument()
  })

  test('marks models without a known usage level as Unknown', () => {
    const response = successfulResponse()
    const local = response.data.local as {
      session: {
        models: Array<{ level?: number }>
      }
    }
    local.session.models[0].level = 0

    render(<DialogHarness response={response} />)

    expect(screen.getAllByText('Unknown').length).toBeGreaterThanOrEqual(1)
  })

  test('renders the authoritative monitor snapshot with used percent and reset times', () => {
    const response = successfulResponse()
    ;(response.data as Record<string, unknown>).snapshot = {
      ok: true,
      fiveHour: {
        usedPercent: 7.8,
        resetsAt: '2099-01-01T00:00:00Z',
        resetInMinutes: 999999,
      },
      weekly: {
        usedPercent: 43.8,
        resetsAt: '2099-01-08T00:00:00Z',
        resetInMinutes: 9999999,
      },
      fetchedAt: '2026-09-02T12:45:02.233Z',
      stale: true,
      source: 'ollama-settings-html',
    }

    render(<DialogHarness response={response} />)

    expect(screen.getByText('Ollama Monitor Snapshot')).toBeInTheDocument()
    expect(screen.getByText('7.8%')).toBeInTheDocument()
    expect(screen.getByText('43.8%')).toBeInTheDocument()
    expect(screen.getByText('Stale')).toBeInTheDocument()
    expect(screen.getAllByText('Resets at:')).toHaveLength(2)
    expect(screen.getAllByText('Resets in:')).toHaveLength(2)
    // Relative reset countdown comes from the ISO resetsAt (both windows).
    expect(screen.getAllByText(/in \d+ years/)).toHaveLength(2)
    // Snapshot timestamp rendered from the ISO fetchedAt.
    expect(
      screen.getByText(
        dayjs(Date.parse('2026-09-02T12:45:02.233Z')).format(
          'YYYY-MM-DD HH:mm:ss'
        )
      )
    ).toBeInTheDocument()
    // With an authoritative snapshot the local note drops the sentence
    // claiming the reset time is not exposed.
    expect(
      screen.queryByText(
        'Ollama resets usage server-side on its own schedule and does not expose the reset time through the usage API.'
      )
    ).not.toBeInTheDocument()
  })

  test('keeps the full recovery note when no snapshot is available', () => {
    render(<DialogHarness response={successfulResponse()} />)

    expect(
      screen.getByText(
        'Recovery timing and the projection are estimates based on when this channel’s own requests slide out of each window. Ollama resets usage server-side on its own schedule and does not expose the reset time through the usage API.'
      )
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Ollama Monitor Snapshot')
    ).not.toBeInTheDocument()
  })
})
