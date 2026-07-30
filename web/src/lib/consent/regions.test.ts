import { describe, expect, it } from 'vitest'
import { classifyRegion } from '../../../netlify/lib/regions'

describe('classifyRegion', () => {
  it('treats EEA, UK, and CH as consent-required', () => {
    for (const country of ['DE', 'FR', 'IE', 'NO', 'IS', 'LI', 'GB', 'CH']) {
      expect(classifyRegion(country)).toBe('REQUIRED')
    }
  })
  it('treats everywhere else as opt-out', () => {
    for (const country of ['US', 'CA', 'BR', 'JP', 'AU', 'IN']) {
      expect(classifyRegion(country)).toBe('OPTIONAL')
    }
  })
  it('is case-insensitive', () => {
    expect(classifyRegion('de')).toBe('REQUIRED')
    expect(classifyRegion('us')).toBe('OPTIONAL')
  })
  it('fails closed when the country is unknown', () => {
    expect(classifyRegion(undefined)).toBe('REQUIRED')
    expect(classifyRegion(null)).toBe('REQUIRED')
    expect(classifyRegion('')).toBe('REQUIRED')
  })
})
