// The scanner proper: opens a submitted site in a fresh `sightmap browser`
// session, records the WebMCP tools each page registers, follows a couple of
// same-origin links, and writes one scan report. Discovery only — no tool is
// ever executed, no form is ever submitted, and no account exists in the
// profile it uses.
//
// The browser is the sightmap CLI's own: `browser start` launches Chrome for
// Testing behind a CDP daemon, `browser inject --persist` installs the
// recorder on every new document, `browser navigate` / `browser eval` drive
// and read the page, and `browser mcp list --json` cross-checks what the
// product's own WebMCP enumeration sees. Every step is a subprocess call, so
// a transcript of what the scanner did is one `sightmap browser` command per
// line — reviewable, and replayable by hand.
//
// Runs wherever the sightmap CLI and a Chrome for Testing build are: a
// Netlify Agent Runner, a GitHub Actions job, a maintainer's laptop.
import { execFile } from 'node:child_process'
import fs from 'node:fs'
import net from 'node:net'
import os from 'node:os'
import path from 'node:path'
import type { ScanFormHint, ScanPage, ScanReport, ScanSurface, ScanStatus, ScanTool } from '../../src/types/directory'
import { preflightUrl, resolvePublic } from './preflight'
import { COLLECT_SCRIPT, COUNT_SCRIPT, RECORDER_SCRIPT } from './scan-recorder'
import { classifyTool, countTools, runChecks, toolWarnings } from './tool-risk'

export const SCANNER_NAME = 'sightmap-atlas-scan'
export const SCANNER_VERSION = '1.0.0'

export interface ScanOptions {
  url: string
  /** Pages to visit including the first. Hard-capped at MAX_PAGES. */
  maxPages?: number
  /** What the submitter said an agent should be able to do. Stored, not acted on. */
  intent?: string | null
  /** Extra same-origin paths the submitter or maintainer wants checked first. */
  paths?: string[]
  /** Loopback fixtures only — never set from a submission. */
  allowLocal?: boolean
  /** The sightmap CLI binary; defaults to $SIGHTMAP_BIN, then `sightmap` on PATH. */
  sightmapBin?: string
  log?: (line: string) => void
}

export const MAX_PAGES = 3
// Client-side registration is not tied to `load`: an SDK may register only
// after its own fetch resolves, and a page that swaps the surface out from
// under the recorder is only visible once it is enumerated. So instead of one
// fixed sleep the driver polls a cheap "how many tools can you see" script and
// stops as soon as the answer holds still, or at SETTLE_MAX_MS.
const SETTLE_POLL_MS = 500
const SETTLE_STABLE_MS = 1000
const SETTLE_MAX_MS = 6000
const MAX_TOOLS = 200
const MAX_SCHEMA_BYTES = 20_000
/** Per-page navigation timeout. */
const PAGE_TIMEOUT_MS = 20_000
/** Whole-scan budget. */
const TOTAL_TIMEOUT_MS = 120_000

// Chrome flags that turn on native WebMCP where the build supports it; the
// same three `sightmap browser mcp list` names when it reports `absent`.
export const WEBMCP_CHROME_FLAGS = [
  '--enable-blink-features=ModelContext,ModelContextTesting',
  '--enable-features=DevToolsWebMCPSupport',
]

// Links a discovery pass must not follow: they log out, mutate, or leave.
const SKIP_LINK = /(logout|log-out|signout|sign-out|delete|remove|unsubscribe|cancel|checkout|cart|pay|purchase|download|\.(pdf|zip|dmg|exe|tar|gz)$)/i

interface Collected {
  records: {
    api: 'navigator' | 'document'
    via: string
    name: string
    description: string
    inputSchema: unknown
    hasExecute: boolean
  }[]
  native: { navigator: boolean; document: boolean }
  errors: string[]
  declarative: { name: string; description: string; inputSchema: unknown }[]
  links: string[]
  forms: { action: string; method: string; fields: string[] }[]
  title: string
  description: string
  url: string
  status: number | null
}

const pathOf = (url: string): string => {
  try {
    const u = new URL(url)
    return `${u.pathname}${u.search}` || '/'
  } catch {
    return url
  }
}

function truncateSchema(schema: unknown): unknown {
  if (schema === undefined || schema === null) return schema
  try {
    const s = JSON.stringify(schema)
    if (s.length <= MAX_SCHEMA_BYTES) return schema
    return { _truncated: true, _bytes: s.length, type: 'object' }
  } catch {
    return null
  }
}

/** Chooses which same-origin links to visit after the first page. */
export function pickLinks(origin: string, startPath: string, hrefs: string[], want: number, preferred: string[] = []): string[] {
  const seen = new Set<string>([startPath])
  const out: string[] = []
  const consider = (href: string) => {
    if (out.length >= want) return
    let u: URL
    try {
      u = new URL(href, origin)
    } catch {
      return
    }
    if (u.origin !== origin) return
    if (u.protocol !== 'https:' && u.protocol !== 'http:') return
    u.hash = ''
    const p = `${u.pathname}${u.search}`
    if (seen.has(p) || SKIP_LINK.test(p)) return
    seen.add(p)
    out.push(u.toString())
  }
  for (const p of preferred) consider(p)
  for (const h of hrefs) consider(h)
  return out
}

/**
 * Two origins are the same site when they differ only by a leading `www.`
 * and both are https (or both http, for a loopback fixture). Anything else —
 * another subdomain, another port, another scheme — is off-origin.
 */
export function sameSite(a: string, b: string): boolean {
  try {
    const ua = new URL(a)
    const ub = new URL(b)
    if (ua.protocol !== ub.protocol || ua.port !== ub.port) return false
    const strip = (h: string) => h.toLowerCase().replace(/^www\./, '')
    return strip(ua.hostname) === strip(ub.hostname)
  } catch {
    return false
  }
}

// Titles that bot-mitigation interstitials use. Matched against the page
// title only after the page settled, so a site that merely mentions one of
// these phrases in its copy is not affected.
const CHALLENGE_TITLE = /^\s*(just a moment|attention required|access denied|checking your browser|verify you are human|please verify|one more step|are you a robot)\b/i

export function isChallengePage(title: string): boolean {
  return CHALLENGE_TITLE.test(title)
}

interface Exec {
  code: number
  stdout: string
  stderr: string
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

// Concrete ports, not --port 0: sightmap CLI <= 0.31.2 writes serverPort: 0
// into its session file for an auto-allocated port, and every later daemon
// command then fails with "no running session". Fixed in the CLI; harmless
// once every runner has that build.
function reservePort(exclude: number[] = []): Promise<number> {
  return new Promise((resolve, reject) => {
    const probe = net.createServer()
    probe.on('error', reject)
    probe.listen(0, '127.0.0.1', () => {
      const addr = probe.address()
      const port = addr && typeof addr === 'object' ? addr.port : 0
      probe.close(() => {
        if (port && !exclude.includes(port)) resolve(port)
        else reservePort(exclude).then(resolve, reject)
      })
    })
  })
}

/**
 * One `sightmap browser …` session in a throwaway directory. The session file
 * the CLI keeps is keyed by `--sightmap-dir`, so a private dir per scan means
 * two scans on one machine never talk to each other's Chrome, and stopping
 * the session removes the profile with the directory.
 */
export class SightmapSession {
  readonly dir: string
  readonly sightmapDir: string
  readonly profile: string
  readonly logFile: string
  readonly transcript: string[] = []
  private started = false
  private tab = ''

  constructor(
    private readonly bin: string,
    private readonly log: (line: string) => void
  ) {
    this.dir = fs.mkdtempSync(path.join(os.tmpdir(), 'atlas-scan-'))
    this.sightmapDir = path.join(this.dir, '.sightmap')
    this.profile = path.join(this.dir, 'profile')
    this.logFile = path.join(this.dir, 'daemon.log')
    fs.mkdirSync(this.sightmapDir, { recursive: true })
  }

  /** Transcript, with an immediate repeat collapsed to `cmd ×n`. */
  private record(cmd: string): void {
    const i = this.transcript.length - 1
    if (i >= 0 && this.transcript[i].replace(/ ×\d+$/, '') === cmd) {
      const seen = Number(this.transcript[i].match(/ ×(\d+)$/)?.[1] ?? 1)
      this.transcript[i] = `${cmd} ×${seen + 1}`
      return
    }
    this.transcript.push(cmd)
  }

  exec(args: string[], timeoutMs: number): Promise<Exec> {
    const shown = args.map((a) => (a.length > 60 ? `${a.slice(0, 57)}…` : a))
    this.record(`sightmap ${shown.join(' ')}`)
    return new Promise((resolve) => {
      execFile(
        this.bin,
        args,
        { timeout: timeoutMs, maxBuffer: 16 << 20, cwd: this.dir, env: process.env },
        (err, stdout, stderr) => {
          const code = err && typeof (err as NodeJS.ErrnoException & { code?: unknown }).code === 'number'
            ? ((err as { code: number }).code as number)
            : err
              ? 1
              : 0
          resolve({ code, stdout: String(stdout), stderr: String(stderr) + (err && !stderr ? `\n${err.message}` : '') })
        }
      )
    })
  }

  private browser(args: string[], timeoutMs: number): Promise<Exec> {
    return this.exec(['browser', ...args, '--sightmap-dir', this.sightmapDir], timeoutMs)
  }

  /**
   * A page-affecting command, pinned to the tab this session owns. Without
   * `--tab` the CLI auto-selects only when exactly one content tab is open, so
   * a page that opens a popup would make every later command ambiguous.
   */
  private page(args: string[], timeoutMs: number): Promise<Exec> {
    return this.browser(this.tab ? [...args, '--tab', this.tab] : args, timeoutMs)
  }

  /** The tail of the daemon log, for an error message that can be acted on. */
  private logTail(): string {
    try {
      const text = fs.readFileSync(this.logFile, 'utf8').trimEnd()
      return text ? `\n--- ${this.logFile}\n${text.slice(-2000)}` : ''
    } catch {
      return ''
    }
  }

  async start(chromeBinary: string | undefined, extraChromeFlags: string[]): Promise<void> {
    const serverPort = await reservePort()
    const cdpPort = await reservePort([serverPort])
    const args = [
      'start',
      '--detach',
      '--headless',
      '--port',
      String(serverPort),
      '--cdp-port',
      String(cdpPort),
      '--profile',
      this.profile,
      // Chrome's own first tab is chrome://newtab, which the CLI does not count
      // as a content tab — with only that open, every page command fails with
      // "no content tab open". Landing on about:blank gives the session a tab
      // to drive before the recorder is installed on it.
      '--url',
      'about:blank',
      '--log-file',
      this.logFile,
      ...WEBMCP_CHROME_FLAGS.map((f) => `--chrome-flag=${f}`),
      ...extraChromeFlags.map((f) => `--chrome-flag=${f}`),
    ]
    if (chromeBinary) args.push('--chrome-binary', chromeBinary)
    const r = await this.browser(args, 90_000)
    if (r.code !== 0) throw new Error(`sightmap browser start failed:\n${r.stderr.trim()}${this.logTail()}`)
    this.started = true
    this.tab = await this.waitForTab(30_000)
  }

  /**
   * `--detach` returns as soon as CDP and the daemon's HTTP server answer,
   * which is before the daemon has navigated its tab to about:blank — so the
   * scanner waits for the tab rather than assuming one is there.
   */
  private async waitForTab(timeoutMs: number): Promise<string> {
    const deadline = Date.now() + timeoutMs
    let last = ''
    for (;;) {
      const r = await this.browser(['tabs', 'list'], 15_000)
      const id = r.stdout.split('\n').find((l) => l.trim())?.split('\t')[0]?.trim()
      if (r.code === 0 && id) return id
      last = r.stderr.trim().split('\n').pop() ?? ''
      if (Date.now() > deadline) {
        throw new Error(`sightmap browser start: no content tab appeared${last ? ` (${last})` : ''}${this.logTail()}`)
      }
      await sleep(250)
    }
  }

  async injectPersist(script: string): Promise<void> {
    const file = path.join(this.dir, 'recorder.js')
    fs.writeFileSync(file, script)
    const r = await this.page(['inject', '--persist', '--file', file], 30_000)
    if (r.code !== 0) throw new Error(`sightmap browser inject failed:\n${r.stderr.trim()}${this.logTail()}`)
    await this.waitForPersistedScript(20_000)
  }

  /**
   * `inject --persist` returns as soon as the daemon holds the script, but the
   * daemon can only put it on a tab its collector has attached to, and Chrome
   * runs an on-new-document script only in documents created after that. A
   * navigation in the gap would load the first page with no recorder on it —
   * every tool registration lost, silently — so wait for the script to land.
   */
  private async waitForPersistedScript(timeoutMs: number): Promise<void> {
    const deadline = Date.now() + timeoutMs
    for (;;) {
      const r = await this.browser(['inject', '--list'], 15_000)
      const tabs = r.stdout.match(/\b(\d+) tab\(s\)/)
      if (r.code === 0 && tabs && Number(tabs[1]) > 0) return
      if (Date.now() > deadline) {
        throw new Error(`sightmap browser inject --persist: the recorder never reached a tab${this.logTail()}`)
      }
      await sleep(200)
    }
  }

  /** Navigates and returns the URL the page settled on (server or client redirects included). */
  async navigate(url: string, timeoutMs: number): Promise<string> {
    const r = await this.page(['navigate', url], timeoutMs)
    if (r.code !== 0) throw new Error(r.stderr.trim().split('\n').pop() || `navigate ${url} failed`)
    const redirected = r.stderr.match(/\(redirected to (\S+)\)/)
    return redirected ? redirected[1] : url
  }

  async eval<T>(script: string, timeoutMs = 35_000): Promise<T> {
    const r = await this.page(['eval', script], timeoutMs)
    if (r.code !== 0) throw new Error(`sightmap browser eval failed:\n${r.stderr.trim()}`)
    return JSON.parse(r.stdout) as T
  }

  /**
   * Waits for the page's tool set to stop changing. Polls COUNT_SCRIPT, which
   * also reconciles the recorder with any surface the page installed over it,
   * and returns once the count has been unchanged for SETTLE_STABLE_MS or the
   * budget runs out.
   */
  async settle(budgetMs = SETTLE_MAX_MS): Promise<void> {
    const start = Date.now()
    let last = Number.NaN
    let stableSince = start
    for (;;) {
      await sleep(SETTLE_POLL_MS)
      let n: number
      try {
        n = await this.eval<number>(COUNT_SCRIPT, 15_000)
      } catch {
        return
      }
      const now = Date.now()
      if (n !== last) {
        last = n
        stableSince = now
      } else if (now - stableSince >= SETTLE_STABLE_MS) {
        return
      }
      if (now - start >= budgetMs) return
    }
  }

  /** `browser mcp list --json`: the product's own enumeration, as a cross-check. */
  async mcpList(): Promise<{ present: boolean; names: string[] }> {
    const r = await this.page(['mcp', 'list', '--json'], 35_000)
    if (r.code !== 0) return { present: false, names: [] }
    try {
      const tools = JSON.parse(r.stdout) as { name?: string }[]
      return { present: true, names: tools.map((t) => String(t.name ?? '')) }
    } catch {
      return { present: true, names: [] }
    }
  }

  async stop(): Promise<void> {
    if (this.started) {
      const r = await this.browser(['stop'], 30_000)
      if (r.code !== 0) this.log(`  ! sightmap browser stop: ${r.stderr.trim().split('\n').pop()}`)
    }
    fs.rmSync(this.dir, { recursive: true, force: true })
  }
}

export function sightmapBinary(explicit?: string): string {
  return explicit || process.env.SIGHTMAP_BIN || 'sightmap'
}

function surfaceOf(c: Collected): ScanSurface {
  const imperative = c.records.length > 0
  if (imperative && (c.native.navigator || c.native.document)) return 'native'
  if (imperative) return 'polyfilled'
  if (c.declarative.length > 0) return 'declarative'
  return 'absent'
}

/**
 * Folds one page's tools into the scan-wide map: a name already recorded just
 * gains this page, a new one is classified and warned about, and past
 * MAX_TOOLS the rest are dropped with a note.
 */
function collectTools(toolMap: Map<string, ScanTool>, page: ScanPage, collected: Collected, notes: string[]): void {
  const seenHere = new Set<string>()
  const add = (name: string, desc: string, schema: unknown, impl: ScanTool['impl'], api: ScanTool['api']) => {
    if (!name || seenHere.has(name)) return
    seenHere.add(name)
    const existing = toolMap.get(name)
    if (existing) {
      if (!existing.pages.includes(page.path)) existing.pages.push(page.path)
      return
    }
    if (toolMap.size >= MAX_TOOLS) {
      if (!notes.includes('tool cap reached')) notes.push('tool cap reached')
      return
    }
    const { risk, reason } = classifyTool(name, desc)
    const tool: ScanTool = {
      name,
      description: desc,
      inputSchema: truncateSchema(schema),
      page: page.path,
      pages: [page.path],
      impl,
      api,
      risk,
      riskReason: reason,
      warnings: [],
    }
    tool.warnings = toolWarnings(tool)
    toolMap.set(name, tool)
  }
  for (const r of collected.records) add(r.name, r.description, r.inputSchema, 'imperative', r.api)
  for (const d of collected.declarative) add(d.name, d.description, d.inputSchema, 'declarative', 'dom')
}

export async function scanSite(opts: ScanOptions): Promise<ScanReport> {
  const log = opts.log ?? (() => {})
  const startedAt = new Date()
  const maxPages = Math.max(1, Math.min(MAX_PAGES, opts.maxPages ?? MAX_PAGES))
  const deadline = Date.now() + TOTAL_TIMEOUT_MS

  const pre = preflightUrl(opts.url, { allowLocal: opts.allowLocal })
  if (!pre.ok) throw new Error(`preflight: ${pre.reason}`)
  if (!opts.allowLocal) {
    const dnsCheck = await resolvePublic(pre.host)
    if (!dnsCheck.ok) throw new Error(`preflight: ${dnsCheck.reason}`)
  }

  const notes: string[] = []
  const pages: ScanPage[] = []
  const toolMap = new Map<string, ScanTool>()
  const forms: ScanFormHint[] = []
  let links: string[] = []
  let title = ''
  let description = ''
  let finalUrl = pre.url
  let blocked = false
  let cliVersion = 'unknown'

  const session = new SightmapSession(sightmapBinary(opts.sightmapBin), log)
  try {
    const v = await session.exec(['version'], 10_000)
    if (v.code !== 0) {
      throw new Error(
        `the sightmap CLI is not available (${sightmapBinary(opts.sightmapBin)}): install it with npm i -g @sightmap/sightmap`
      )
    }
    cliVersion = v.stdout.trim().replace(/^sightmap version\s*/, '') || 'unknown'

    // A runner behind an egress proxy sets HTTPS_PROXY for its tools; Chrome
    // does not read it on its own, so pass it through.
    const proxy = process.env.HTTPS_PROXY || process.env.https_proxy
    const extra: string[] = []
    if (proxy) {
      extra.push(`--proxy-server=${proxy}`)
      // Chrome bypasses loopback for a proxy implicitly, but the scanner's own
      // fixtures live there and a `<-loopback>` in someone's NO_PROXY would
      // turn that off, so say it out loud.
      extra.push('--proxy-bypass-list=localhost;127.0.0.1;[::1]')
    }
    // No permission prompts of any kind: a page asking for camera, location,
    // or notifications gets a denial, not a prompt that hangs the scan.
    extra.push('--deny-permission-prompts')
    // Operator input for the runner's own network environment, never from a submission.
    const operatorFlags = (process.env.ATLAS_CHROME_FLAGS ?? '').split(/\s+/).filter(Boolean)
    if (operatorFlags.length > 0) {
      extra.push(...operatorFlags)
      notes.push(`operator Chrome flags: ${operatorFlags.join(' ')}`)
    }
    await session.start(process.env.ATLAS_CHROME_PATH || undefined, extra)
    await session.injectPersist(RECORDER_SCRIPT)

    let origin = new URL(pre.url).origin
    const queue: string[] = [pre.url]
    const preferred = (opts.paths ?? []).map((p) => new URL(p, origin).toString())
    let first = true

    while (queue.length > 0 && pages.length < maxPages) {
      if (Date.now() > deadline) {
        notes.push('scan budget exhausted before every page was checked')
        break
      }
      const url = queue.shift()!
      const reqPath = pathOf(url)
      log(`  → ${reqPath}`)

      let landed: string
      try {
        landed = await session.navigate(url, PAGE_TIMEOUT_MS)
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err)
        log(`  ! ${reqPath}: ${msg}`)
        pages.push({ url, path: reqPath, title: '', status: null, surface: 'absent', tools: [], error: msg })
        first = false
        continue
      }

      if (first) {
        finalUrl = landed
        const landedOrigin = new URL(landed).origin
        if (landedOrigin !== origin && sameSite(origin, landedOrigin)) {
          // `example.com` → `www.example.com` (or back) is the same site
          // wearing its canonical host. Preflight the destination like a
          // fresh submission and carry on there; the listing's host is the
          // one the site actually answers on.
          const re = preflightUrl(landed, { allowLocal: opts.allowLocal })
          const okThere = re.ok && (opts.allowLocal || (await resolvePublic(re.host)).ok)
          if (okThere) {
            notes.push(`first page redirected to ${landedOrigin}; continuing there`)
            origin = landedOrigin
          }
        }
        if (landedOrigin !== origin) {
          // The scanner never follows the page off-origin. Re-run preflight
          // on the destination so the note says whether resubmitting it
          // would even be allowed, then stop.
          const re = preflightUrl(landed, { allowLocal: opts.allowLocal })
          const okThere = re.ok && (opts.allowLocal || (await resolvePublic(re.host)).ok)
          notes.push(`first page redirected off-origin to ${landedOrigin}`)
          blocked = true
          pages.push({
            url,
            path: reqPath,
            title: '',
            status: null,
            surface: 'absent',
            tools: [],
            error: okThere ? `redirected to ${landedOrigin}; resubmit that URL` : `redirected to ${landedOrigin}, which failed preflight`,
          })
          break
        }
      }

      await session.settle(Math.max(SETTLE_POLL_MS, Math.min(SETTLE_MAX_MS, deadline - Date.now())))
      let collected: Collected
      try {
        collected = await session.eval<Collected>(COLLECT_SCRIPT)
      } catch (err) {
        const msg = err instanceof Error ? err.message.split('\n').pop() ?? err.message : String(err)
        log(`  ! ${reqPath}: ${msg}`)
        pages.push({ url: landed, path: pathOf(landed), title: '', status: null, surface: 'absent', tools: [], error: msg })
        first = false
        continue
      }

      // Chrome answers a failed navigation with its own error document, which
      // `browser navigate` reports as a success. Left alone it reads as a real
      // page that simply has no WebMCP surface — the most misleading result
      // the scanner can produce — so name it for what it is.
      if (/^chrome-error:/i.test(collected.url || landed)) {
        const why = 'the browser could not load this page (it showed an error page instead)'
        log(`  ! ${reqPath}: ${why}`)
        pages.push({ url: landed, path: reqPath, title: '', status: null, surface: 'absent', tools: [], error: why })
        for (const e of collected.errors) notes.push(`${reqPath}: recorder: ${e}`)
        first = false
        continue
      }

      if (isChallengePage(collected.title)) {
        // A bot-challenge interstitial (Cloudflare and the like) is not the
        // site: recording "no tools" here would list a product as having no
        // WebMCP surface because a robot check got in the way. The scanner
        // never solves challenges; a maintainer can rescan from a network the
        // site does not challenge, or ask the owner to allowlist the scanner's
        // user agent.
        const msg = `bot-challenge page ("${collected.title.trim()}") instead of the site; the scanner does not solve challenges`
        log(`  ! ${reqPath}: ${msg}`)
        pages.push({ url: collected.url || landed, path: pathOf(collected.url || landed), title: collected.title, status: collected.status, surface: 'absent', tools: [], error: msg })
        if (first) {
          blocked = true
          break
        }
        continue
      }
      const surface = surfaceOf(collected)
      const names = [...new Set([...collected.records.map((r) => r.name), ...collected.declarative.map((d) => d.name)])]
      const page: ScanPage = {
        url: collected.url || landed,
        path: pathOf(collected.url || landed),
        title: collected.title,
        status: collected.status,
        surface,
        tools: names,
      }
      pages.push(page)

      if (first) {
        title = collected.title
        description = collected.description
        links = collected.links
        for (const next of pickLinks(origin, page.path, collected.links, maxPages - 1, preferred)) queue.push(next)
        // The product's own enumeration, for the record. It reads
        // getTools() on document.modelContext, which the recorder answers,
        // so the two should agree on this page's imperative tools.
        const cli = await session.mcpList()
        const recorded = new Set(collected.records.map((r) => r.name))
        const missing = cli.names.filter((n) => n && !recorded.has(n))
        notes.push(
          cli.present
            ? `sightmap browser mcp list saw ${cli.names.length} tool(s) on ${page.path}${missing.length ? ` (not recorded: ${missing.join(', ')})` : ''}`
            : `sightmap browser mcp list reported no WebMCP surface on ${page.path}`
        )
      }
      first = false

      for (const f of collected.forms) forms.push({ page: page.path, ...f })
      for (const e of collected.errors) notes.push(`${page.path}: recorder: ${e}`)

      collectTools(toolMap, page, collected, notes)
      log(`    ${page.surface}: ${page.tools.length} tool(s)`)
    }
  } finally {
    await session.stop()
  }

  const tools = [...toolMap.values()]
  const surfaces = pages.map((p) => p.surface)
  const surface: ScanSurface = surfaces.includes('native')
    ? 'native'
    : surfaces.includes('polyfilled')
      ? 'polyfilled'
      : surfaces.includes('declarative')
        ? 'declarative'
        : 'absent'

  let status: ScanStatus
  if (blocked) status = 'blocked'
  else if (pages.length > 0 && pages.every((p) => p.error)) status = 'load-error'
  else if (tools.length > 0) status = 'tools-found'
  else if (surface === 'absent') status = 'api-absent'
  else status = 'api-empty'
  if (status === 'tools-found' && tools.some((t) => t.warnings.some((w) => w.startsWith('metadata matches')))) {
    status = 'needs-review'
  }

  return {
    version: 1,
    url: pre.url,
    finalUrl,
    host: pre.host,
    scannedAt: startedAt.toISOString(),
    scanner: { name: SCANNER_NAME, version: SCANNER_VERSION, browser: `sightmap ${cliVersion}` },
    surface,
    status,
    intent: opts.intent?.trim() || null,
    pages,
    tools,
    checks: runChecks(tools, pages),
    counts: countTools(tools, pages),
    hints: { forms, links: links.slice(0, 40), title, description },
    notes: [...notes, ...session.transcript.map((t) => `$ ${t}`)],
  }
}
