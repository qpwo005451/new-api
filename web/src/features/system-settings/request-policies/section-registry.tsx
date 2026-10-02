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
import { ModelHealthPolicySection } from '../models/model-health-policy-section'
import { ModelWeightSection } from '../models/model-weight-section'
import { UpstreamRateLimitSection } from '../models/upstream-rate-limit-section'
import { VirtualPoolRoutingSection } from '../models/virtual-pool-routing-section'
import { createSectionRegistry } from '../utils/section-registry'
import { ChannelHealthSection } from './channel-health-section'
import type { RequestPolicySettings } from './defaults'
import { RequestChecksSection } from './request-checks-section'
import { RoutingPolicySection } from './routing-section'

const POLICY_SECTIONS = [
  {
    id: 'filtering',
    titleKey: 'Request checks',
    build: (settings: RequestPolicySettings) => (
      <RequestChecksSection
        defaultValues={{
          CheckSensitiveEnabled: settings.CheckSensitiveEnabled,
          CheckSensitiveOnPromptEnabled: settings.CheckSensitiveOnPromptEnabled,
          SensitiveWords: settings.SensitiveWords,
        }}
      />
    ),
  },
  {
    id: 'routing',
    titleKey: 'Sessions and retries',
    build: () => <RoutingPolicySection />,
  },
  {
    id: 'health',
    titleKey: 'Channel health',
    build: (settings: RequestPolicySettings) => (
      <ChannelHealthSection defaultValues={settings} />
    ),
  },
  {
    id: 'upstream-rate-limits',
    titleKey: 'Upstream Request Limits',
    build: (settings: RequestPolicySettings) => (
      <UpstreamRateLimitSection
        defaultValues={{
          'upstream_rate_limit_setting.enabled':
            settings['upstream_rate_limit_setting.enabled'],
          'upstream_rate_limit_setting.rules':
            settings['upstream_rate_limit_setting.rules'],
        }}
      />
    ),
  },
  {
    id: 'virtual-pool-routing',
    titleKey: 'Virtual Pool Routing',
    build: (settings: RequestPolicySettings) => (
      <VirtualPoolRoutingSection
        defaultValues={{
          'model_retry_policy_setting.virtual_model_routes':
            settings['model_retry_policy_setting.virtual_model_routes'],
        }}
      />
    ),
  },
  {
    id: 'model-health-policy',
    titleKey: 'Model Health Policy',
    build: (settings: RequestPolicySettings) => (
      <ModelHealthPolicySection
        defaultValues={{
          'model_health_policy_setting.enabled':
            settings['model_health_policy_setting.enabled'],
          'model_health_policy_setting.rules':
            settings['model_health_policy_setting.rules'],
        }}
      />
    ),
  },
  {
    id: 'model-weights',
    titleKey: 'Model Weights',
    build: (settings: RequestPolicySettings) => (
      <ModelWeightSection
        defaultValues={{
          'model_weight_setting.weights':
            settings['model_weight_setting.weights'],
        }}
      />
    ),
  },
] as const

export type PolicySectionId = (typeof POLICY_SECTIONS)[number]['id']
const registry = createSectionRegistry<PolicySectionId, RequestPolicySettings>({
  sections: POLICY_SECTIONS,
  defaultSection: 'routing',
  basePath: '/system-settings/request-policies',
  urlStyle: 'path',
})
export const POLICY_SECTION_IDS = registry.sectionIds
export const getPolicySectionNavItems = registry.getSectionNavItems
export const getPolicySectionContent = registry.getSectionContent
export const getPolicySectionMeta = registry.getSectionMeta
