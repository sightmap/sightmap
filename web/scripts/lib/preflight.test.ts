import { describe, expect, it } from 'vitest'
import { isPrivateAddress, preflightUrl, resolvePublic } from './preflight'

describe('preflightUrl', () => {
  it('accepts a public https URL and strips the fragment', () => {
    const r = preflightUrl('https://Example.com/docs?x=1#top')
    expect(r).toEqual({ ok: true, url: 'https://example.com/docs?x=1', host: 'example.com' })
  })

  it('adds https:// to a bare hostname', () => {
    expect(preflightUrl('shop.acme.io').url).toBe('https://shop.acme.io/')
    expect(preflightUrl('my-shop.acme.io/a-b').ok).toBe(true)
  })

  it('rejects http, credentials, IP literals, and reserved hosts', () => {
    expect(preflightUrl('http://acme.io/').reason).toMatch(/https/)
    expect(preflightUrl('https://user:pw@acme.io/').reason).toMatch(/credentials/)
    expect(preflightUrl('https://10.0.0.1/').reason).toMatch(/IP-literal/)
    expect(preflightUrl('https://[::1]/').reason).toMatch(/IP-literal/)
    expect(preflightUrl('https://localhost/').reason).toMatch(/dot|reserved/)
    expect(preflightUrl('https://metadata.internal/').reason).toMatch(/reserved/)
    expect(preflightUrl('https://acme.local/').reason).toMatch(/reserved/)
  })

  it('rejects junk', () => {
    expect(preflightUrl('').ok).toBe(false)
    expect(preflightUrl('not a url').ok).toBe(false)
    expect(preflightUrl('https://exa mple.com').ok).toBe(false)
    expect(preflightUrl('javascript:alert(1)').ok).toBe(false)
    expect(preflightUrl('https://a'.padEnd(3000, 'a')).ok).toBe(false)
  })

  it('lets a test point the scanner at a loopback fixture only when asked', () => {
    expect(preflightUrl('http://127.0.0.1:4173/', { allowLocal: true }).ok).toBe(true)
    expect(preflightUrl('http://127.0.0.1:4173/').ok).toBe(false)
  })
})

describe('isPrivateAddress', () => {
  it('classifies the usual ranges', () => {
    for (const ip of ['127.0.0.1', '10.1.2.3', '172.16.0.1', '192.168.1.1', '169.254.169.254', '100.64.0.1', '0.0.0.0', '::1', 'fe80::1', 'fd00::1', '::ffff:10.0.0.1']) {
      expect(isPrivateAddress(ip), ip).toBe(true)
    }
    for (const ip of ['8.8.8.8', '104.16.0.1', '2606:4700::1111']) {
      expect(isPrivateAddress(ip), ip).toBe(false)
    }
  })
})

describe('resolvePublic', () => {
  it('rejects a host that resolves to a private address', async () => {
    const r = await resolvePublic('internal.acme.io', async () => [{ address: '10.0.0.5' }])
    expect(r.ok).toBe(false)
    expect(r.reason).toMatch(/private/)
  })

  it('accepts a host whose every record is public', async () => {
    const r = await resolvePublic('acme.io', async () => [{ address: '104.16.0.1' }, { address: '2606:4700::1111' }])
    expect(r.ok).toBe(true)
  })

  it('rejects a host that does not resolve', async () => {
    const r = await resolvePublic('nope.acme.io', async () => {
      throw new Error('ENOTFOUND')
    })
    expect(r.ok).toBe(false)
    expect(r.reason).toMatch(/ENOTFOUND/)
  })
})
