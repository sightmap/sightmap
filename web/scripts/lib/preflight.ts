// URL preflight for anything that will point a browser at a submitted
// address. Two layers:
//
//   preflightUrl   pure: scheme, credentials, hostname shape, literal
//                  private / loopback / link-local addresses. Runs in the
//                  submission function and again in the scanner.
//   resolvePublic  DNS: rejects a hostname whose A/AAAA records land in a
//                  private or reserved range. The literal check above cannot
//                  catch `internal.example.com → 10.0.0.5`; this can.
//
// Neither is a security boundary on its own. They are the cheap layer that
// keeps the scanner from being pointed at a metadata endpoint or a LAN host
// by mistake; the browser sandbox and the maintainer preflight sit behind it.
import dns from 'node:dns'
import net from 'node:net'

export interface Preflight {
  ok: boolean
  url: string
  host: string
  reason?: string
}

const BLOCKED_HOST = /(^|\.)(localhost|local|internal|localdomain|home|lan|corp|intranet|test|example|invalid|onion)$/i

/**
 * Characters we refuse to carry in a path or query string:
 * `$ ` ' " \ | ; < > ( ) { }` plus whitespace and control characters.
 *
 * A submitted URL is echoed into a runner prompt and into a shell command
 * (`netlify/lib/runner.ts`), and `new URL()` percent-encodes almost none of
 * these — `?q=$(id)` survives a round trip verbatim. The command builder
 * single-quotes every value, and this is the other half of that pair: a URL
 * that needs any of them is not a URL we can scan, and no legitimate
 * submission has one. Note that `%24` still passes; that is fine, since it is
 * inert everywhere it is interpolated.
 */
const UNSAFE_URL_CHARS = /[$`'"\\|;<>(){}\s\u0000-\u001f\u007f]/

/** True for loopback, private, link-local, multicast, unspecified, and CGNAT v4 addresses. */
export function isPrivateV4(ip: string): boolean {
  const parts = ip.split('.').map(Number)
  if (parts.length !== 4 || parts.some((p) => Number.isNaN(p) || p < 0 || p > 255)) return true
  const [a, b] = parts
  if (a === 0 || a === 10 || a === 127) return true
  if (a === 100 && b >= 64 && b <= 127) return true
  if (a === 169 && b === 254) return true
  if (a === 172 && b >= 16 && b <= 31) return true
  if (a === 192 && b === 168) return true
  if (a === 192 && b === 0) return true
  if (a === 198 && (b === 18 || b === 19)) return true
  if (a >= 224) return true
  return false
}

/** True for loopback, unspecified, unique-local, link-local, and v4-mapped private v6 addresses. */
export function isPrivateV6(ip: string): boolean {
  const lower = ip.toLowerCase()
  if (lower === '::' || lower === '::1') return true
  if (lower.startsWith('fe8') || lower.startsWith('fe9') || lower.startsWith('fea') || lower.startsWith('feb')) return true
  if (lower.startsWith('fc') || lower.startsWith('fd')) return true
  if (lower.startsWith('ff')) return true
  const mapped = lower.match(/^::ffff:(\d+\.\d+\.\d+\.\d+)$/)
  if (mapped) return isPrivateV4(mapped[1])
  return false
}

export function isPrivateAddress(ip: string): boolean {
  const kind = net.isIP(ip)
  if (kind === 4) return isPrivateV4(ip)
  if (kind === 6) return isPrivateV6(ip)
  return true
}

/**
 * Normalises and vets a submitted URL without touching the network.
 * `allowLocal` exists for the scanner's own tests, which point it at a
 * fixture on 127.0.0.1; the submission function never sets it.
 */
export function preflightUrl(input: string, opts: { allowLocal?: boolean } = {}): Preflight {
  const trimmed = (input ?? '').trim()
  if (!trimmed) return { ok: false, url: '', host: '', reason: 'empty URL' }
  if (trimmed.length > 2048) return { ok: false, url: '', host: '', reason: 'URL is too long' }
  if (/[\s\u0000-\u001f\u007f]/.test(trimmed)) return { ok: false, url: '', host: '', reason: 'URL contains whitespace or control characters' }

  let u: URL
  try {
    u = new URL(/^[a-z][a-z0-9+.-]*:/i.test(trimmed) ? trimmed : `https://${trimmed}`)
  } catch {
    return { ok: false, url: '', host: '', reason: 'not a valid URL' }
  }

  if (u.username || u.password) return { ok: false, url: '', host: '', reason: 'URL must not carry credentials' }
  if (u.protocol !== 'https:') {
    if (!(opts.allowLocal && u.protocol === 'http:')) {
      return { ok: false, url: '', host: '', reason: 'only https:// URLs are scanned' }
    }
  }

  const host = u.hostname.toLowerCase()
  const bare = host.replace(/^\[|\]$/g, '')
  if (net.isIP(bare)) {
    if (!opts.allowLocal || !isPrivateAddress(bare)) {
      return { ok: false, url: '', host, reason: 'IP-literal hosts are not scanned' }
    }
  } else {
    if (!host.includes('.')) return { ok: false, url: '', host, reason: 'hostname has no dot' }
    if (BLOCKED_HOST.test(host) && !opts.allowLocal) {
      return { ok: false, url: '', host, reason: 'hostname is reserved or not public' }
    }
    if (!/^[a-z0-9.-]+$/.test(host) || host.startsWith('-') || host.includes('..')) {
      return { ok: false, url: '', host, reason: 'hostname is malformed' }
    }
  }

  if (UNSAFE_URL_CHARS.test(u.pathname) || UNSAFE_URL_CHARS.test(u.search)) {
    return { ok: false, url: '', host, reason: 'URL contains characters the scanner does not accept' }
  }

  // Fragments never reach the server and a tracking token in the query is not
  // ours to keep. Keep the path; a submitted deep link is a legitimate start.
  u.hash = ''
  return { ok: true, url: u.toString(), host }
}

/**
 * Resolves `host` and rejects it if any address is private or reserved.
 * A resolver failure is a rejection too: a scan of a host that does not
 * resolve would only ever produce a load error.
 */
export async function resolvePublic(
  host: string,
  lookup: (host: string) => Promise<{ address: string }[]> = (h) => dns.promises.lookup(h, { all: true })
): Promise<Preflight> {
  const bare = host.replace(/^\[|\]$/g, '')
  if (net.isIP(bare)) {
    return isPrivateAddress(bare)
      ? { ok: false, url: '', host, reason: 'address is private or reserved' }
      : { ok: true, url: '', host }
  }
  let records: { address: string }[]
  try {
    records = await lookup(bare)
  } catch (err) {
    return { ok: false, url: '', host, reason: `DNS lookup failed: ${err instanceof Error ? err.message : err}` }
  }
  if (records.length === 0) return { ok: false, url: '', host, reason: 'hostname does not resolve' }
  const bad = records.find((r) => isPrivateAddress(r.address))
  if (bad) return { ok: false, url: '', host, reason: `resolves to a private or reserved address (${bad.address})` }
  return { ok: true, url: '', host }
}
