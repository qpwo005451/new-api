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
import { describe, expect, test } from 'vitest'

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { OllamaUsageDialog } = await import('../ollama-usage-dialog')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
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
        'Weighted usage': 'Weighted usage',
        'Total tokens:': 'Total tokens:',
        'Usage ratio (upstream ÷ local weighted):':
          'Usage ratio (upstream ÷ local weighted):',
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
        "Ollama does not publish the usage cap; weighted usage estimates upstream usage units as level × (prompt + completion) tokens. The usage ratio (upstream ÷ local weighted) calibrates the estimate while all traffic goes through this channel.":
          "Ollama does not publish the usage cap; weighted usage estimates upstream usage units as level × (prompt + completion) tokens. The usage ratio (upstream ÷ local weighted) calibrates the estimate while all traffic goes through this channel.",
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
  test('renders upstream windows, local per-model estimates, and the calibrated usage ratio', () => {
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
    // Local weighted usage and per-window ratios: 30/6600 = 0.0045, 300/6600 = 0.0455.
    expect(screen.getAllByText('6,600')).toHaveLength(2)
    expect(screen.getAllByText('0.0045')).toHaveLength(1)
    expect(screen.getAllByText('0.0455')).toHaveLength(1)
    // Level badges come from the response level field.
    expect(screen.getAllByText('Low').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('Medium').length).toBeGreaterThanOrEqual(1)
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
})
