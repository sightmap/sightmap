// Consent region classification, used by the geo-region edge function.
// Kept independent of src/ so the Deno edge bundle has no app dependencies.

export const CONSENT_REQUIRED_COUNTRIES = new Set<string>([
  // EU 27
  'AT',
  'BE',
  'BG',
  'HR',
  'CY',
  'CZ',
  'DK',
  'EE',
  'FI',
  'FR',
  'DE',
  'GR',
  'HU',
  'IE',
  'IT',
  'LV',
  'LT',
  'LU',
  'MT',
  'NL',
  'PL',
  'PT',
  'RO',
  'SK',
  'SI',
  'ES',
  'SE',
  // EEA non-EU
  'IS',
  'LI',
  'NO',
  // UK + Switzerland
  'GB',
  'CH',
])

export type ConsentRegion = 'REQUIRED' | 'OPTIONAL'

/** ISO-3166 alpha-2 country → consent posture. Unknown fails closed to REQUIRED. */
export function classifyRegion(country: string | null | undefined): ConsentRegion {
  if (!country) return 'REQUIRED'
  return CONSENT_REQUIRED_COUNTRIES.has(country.toUpperCase()) ? 'REQUIRED' : 'OPTIONAL'
}
