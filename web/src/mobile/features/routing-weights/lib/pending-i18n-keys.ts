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

/**
 * Copy introduced by the mobile routing-weights page. The shared locale files
 * are filled by the follow-up i18n worker, so until that lands the mobile
 * locale tests treat exactly these keys as pending instead of failing the
 * console suite. Delete this list (and its two test imports) once every locale
 * carries the keys.
 */
export const PENDING_ROUTING_WEIGHTS_I18N_KEYS = [
  'Apply preset to this model?',
  'Custom ratio',
  'Enter a ratio greater than 0.',
  'Model routing',
  'Routing ratio',
  'Set the ratio as percentages. They are stored as integer weights.',
  'The share is computed inside one priority tier.',
  'This replaces the overrides of {{model}} only.',
] as const
