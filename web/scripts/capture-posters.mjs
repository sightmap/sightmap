// Photographs the building tour, one still per chapter, for the poster tier.
//
//   pnpm posters                       # all chapters, both viewports
//   pnpm posters -- --variant=mobile   # one viewport
//   pnpm posters -- --chapter=nightfall
//   pnpm posters -- --url=http://localhost:5173/building   # reuse a dev server
//
// Output: web/public/building/poster-<variant>-<nn>-<chapter>.webp, transparent,
// committed to the repo. The build does not run this — a prerender that needed
// a GPU would be a prerender that breaks in CI — so the stills are artwork with
// a reproducible recipe, and this file is the recipe.
//
// REGENERATE THE STILLS whenever the scene's look changes: lighting, tone
// mapping, materials, camera framing, or a chapter's parameters. A poster tier
// whose artwork does not match the live scene is worse than no poster tier.
//
// Three traps this script exists to avoid, all of them met in practice:
//
//   1. --disable-gpu kills the WebGL context, the page never gets a scene, and
//      the capture is a blank frame that looks like a broken build. We run
//      SwiftShader instead, and never pass --disable-gpu.
//   2. The dev SPA intermittently fails to hydrate after a navigation, leaving
//      an empty root. An earlier audit collected 26 silently blank frames that
//      way. Every capture asserts nine .bld-chapter nodes first and reloads if
//      they are missing.
//   3. The scene damps asymptotically and drifts continuously, so "wait a bit
//      then shoot" bakes in half-drawn linework. Capture mode (see
//      src/components/building/capture.ts) freezes the drift and publishes how
//      far the scene still has to travel; we wait on that number.
import { spawn } from 'node:child_process'
import { existsSync, mkdirSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import puppeteer from 'puppeteer-core'

const here = dirname(fileURLToPath(import.meta.url))
const webRoot = resolve(here, '..')

/** Kept in step with src/components/building/posters.ts.
 *
 *  `scale` is the device pixel ratio each still is rendered at, chosen per
 *  viewport by what the artwork needs rather than by a single global number:
 *  the desktop frame is wide enough that 1.5x already resolves the thinnest
 *  label type (measured: 118KB at 1.5x vs 185KB at 2x, no visible difference),
 *  while the mobile frame is small enough that 2x costs little and phones are
 *  overwhelmingly retina. --scale overrides both. */
const VIEWPORTS = {
  desktop: { width: 1440, height: 900, scale: 1.5 },
  mobile: { width: 430, height: 860, scale: 2 },
}
const OUT_DIR = resolve(webRoot, 'public/building')
const DEV_PORT = 5199
const CHAPTERS_EXPECTED = 9
/** Matches SETTLED_DELTA in src/components/building/capture.ts. */
const SETTLED_DELTA = 0.002
const SETTLE_TIMEOUT_MS = 20_000
/** After the scene settles: label fades, and any looping vignette gets past
 *  its opening state rather than being photographed mid-first-move. */
const DWELL_MS = 900
/** A fully transparent webp is about a kilobyte. Anything near that drew nothing. */
const MIN_BYTES = 6_000

const CHROME_CANDIDATES = [
  process.env.CHROME,
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/usr/bin/google-chrome-stable',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium-browser',
  '/usr/bin/chromium',
  '/snap/bin/chromium',
].filter(Boolean)

function findChrome() {
  const found = CHROME_CANDIDATES.find((p) => existsSync(p))
  if (!found) {
    console.error(`no Chrome found. Tried:\n  ${CHROME_CANDIDATES.join('\n  ')}\nSet CHROME=/path/to/chrome.`)
    process.exit(1)
  }
  return found
}

function parseArgs(argv) {
  const args = { url: null, variants: Object.keys(VIEWPORTS), chapter: null, quality: 80, scale: null }
  for (const arg of argv) {
    const [key, value] = arg.replace(/^--/, '').split('=')
    if (key === 'url') args.url = value
    else if (key === 'variant') args.variants = [value]
    else if (key === 'chapter') args.chapter = value
    else if (key === 'quality') args.quality = Number(value)
    else if (key === 'scale') args.scale = Number(value)
    else {
      console.error(`unknown argument: ${arg}`)
      process.exit(1)
    }
  }
  for (const v of args.variants) {
    if (!VIEWPORTS[v]) {
      console.error(`unknown variant "${v}" — expected one of ${Object.keys(VIEWPORTS).join(', ')}`)
      process.exit(1)
    }
  }
  return args
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function waitForServer(url, timeoutMs = 120_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url, { redirect: 'manual' })
      if (res.status < 500) return
    } catch {
      // not listening yet
    }
    await sleep(400)
  }
  throw new Error(`dev server did not answer at ${url} within ${timeoutMs}ms`)
}

/** Starts `pnpm dev` (which also regenerates the blog/atlas manifests) and
 *  returns a stop function. Skipped when --url points at a running server. */
async function startDevServer() {
  const child = spawn('pnpm', ['dev', '--port', String(DEV_PORT), '--strictPort'], {
    cwd: webRoot,
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  child.stdout.on('data', (b) => process.stdout.write(`  [dev] ${b}`))
  child.stderr.on('data', (b) => process.stderr.write(`  [dev] ${b}`))
  const url = `http://localhost:${DEV_PORT}/building`
  await waitForServer(url)
  return { url, stop: () => child.kill('SIGTERM') }
}

/**
 * Loads the page and refuses to go further until it is really hydrated:
 * nine chapter sections and a canvas that has drawn at least one frame.
 * Reloads rather than trusting a page that came up empty.
 */
async function loadHydrated(page, url) {
  for (let attempt = 1; attempt <= 4; attempt++) {
    await page.goto(url, { waitUntil: 'networkidle2', timeout: 60_000 })
    try {
      await page.waitForFunction(
        (expected) =>
          document.querySelectorAll('.bld-chapter').length === expected &&
          !!document.querySelector('.bld-stage canvas'),
        { timeout: 20_000 },
        CHAPTERS_EXPECTED,
      )
      await page.waitForFunction(() => (window.__bldCaptureProgress?.frames ?? 0) > 0, { timeout: 30_000 })
      return
    } catch {
      console.warn(`  hydration incomplete (attempt ${attempt}) — reloading`)
    }
  }
  throw new Error('page never hydrated with nine chapters and a drawing canvas')
}

/** Everything that is not the scene: hidden, but the story sections keep their
 *  layout so scroll positions and the active chapter still resolve. */
const CHROME_CSS = `
  .bld-nav, .bld-hud, .bld-rail, .bld-footer, .bld-tier,
  .consent, [data-component='ConsentBanner'] { display: none !important; }
  .bld-story { visibility: hidden !important; }
  html, body, .bld, .bld-stage { background: none !important; }
  .bld-stage::before, .bld-stage::after { opacity: 0 !important; }
  .bld-poster { display: none !important; }
`

/** Scroll so the page's own measurement lands exactly on chapter `index`.
 *  BuildingExperience samples at `scrollY + vh/2`, so the midpoint of the
 *  section is the position where its progress is a whole number. */
async function goToChapter(page, index, id) {
  await page.evaluate((i) => {
    const el = document.querySelectorAll('.bld-chapter')[i]
    const top = el.offsetTop + el.offsetHeight / 2 - window.innerHeight / 2
    window.scrollTo({ top: Math.max(0, top), behavior: 'instant' })
  }, index)
  await page.waitForFunction(
    (chapter) => document.querySelector('.bld-chapter[data-active="true"]')?.dataset.chapter === chapter,
    { timeout: 10_000 },
    id,
  )
}

/** Waits for the damped scene to arrive where it is going. */
async function waitSettled(page) {
  await page.waitForFunction(
    (limit) => {
      const p = window.__bldCaptureProgress
      return !!p && p.frames > 30 && p.delta < limit
    },
    { timeout: SETTLE_TIMEOUT_MS, polling: 100 },
    SETTLED_DELTA,
  )
  await sleep(DWELL_MS)
}

async function captureVariant(browser, url, variant, args) {
  const { width, height } = VIEWPORTS[variant]
  const scale = args.scale ?? VIEWPORTS[variant].scale
  const page = await browser.newPage()
  await page.setViewport({ width, height, deviceScaleFactor: scale })
  await page.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: 'no-preference' }])
  // Set before any page script runs, so the scene comes up in capture mode
  // rather than being switched into it mid-flight.
  await page.evaluateOnNewDocument(() => {
    window.__bldCapture = true
  })
  page.on('pageerror', (err) => console.warn(`  [page error] ${err.message}`))

  await loadHydrated(page, url)
  await page.addStyleTag({ content: CHROME_CSS })

  const chapters = await page.evaluate(() =>
    [...document.querySelectorAll('.bld-chapter')].map((el) => el.dataset.chapter),
  )
  const written = []
  for (const [index, id] of chapters.entries()) {
    if (args.chapter && args.chapter !== id) continue
    await goToChapter(page, index, id)
    await waitSettled(page)
    const file = resolve(OUT_DIR, `poster-${variant}-${String(index).padStart(2, '0')}-${id}.webp`)
    const buffer = await page.screenshot({ type: 'webp', quality: args.quality, omitBackground: true })
    if (buffer.length < MIN_BYTES) {
      throw new Error(`${id} (${variant}) captured ${buffer.length} bytes — that is a blank frame, not a scene`)
    }
    writeFileSync(file, buffer)
    written.push({ id, file, bytes: buffer.length })
    console.log(`  ${variant} ${String(index).padStart(2, '0')} ${id.padEnd(13)} ${(buffer.length / 1024).toFixed(0)}KB`)
  }
  await page.close()
  return written
}

async function main() {
  const args = parseArgs(process.argv.slice(2))
  mkdirSync(OUT_DIR, { recursive: true })
  const chrome = findChrome()

  let server = null
  let url = args.url
  if (!url) {
    console.log(`starting dev server on :${DEV_PORT}`)
    server = await startDevServer()
    url = server.url
  } else {
    await waitForServer(url)
  }

  const browser = await puppeteer.launch({
    executablePath: chrome,
    headless: true,
    args: [
      '--no-sandbox',
      '--disable-dev-shm-usage',
      '--hide-scrollbars',
      // SwiftShader, NOT --disable-gpu: the software rasteriser still gives the
      // page a real WebGL context, which is the whole point.
      '--use-gl=angle',
      '--use-angle=swiftshader',
      '--enable-unsafe-swiftshader',
    ],
  })

  try {
    const written = []
    for (const variant of args.variants) {
      const { width, height } = VIEWPORTS[variant]
      console.log(`capturing ${variant} (${width}x${height} @${args.scale ?? VIEWPORTS[variant].scale}x)`)
      written.push(...(await captureVariant(browser, url, variant, args)))
    }
    const expected = (args.chapter ? 1 : CHAPTERS_EXPECTED) * args.variants.length
    if (written.length !== expected) {
      throw new Error(`captured ${written.length} stills, expected ${expected}`)
    }
    const total = written.reduce((sum, w) => sum + w.bytes, 0)
    console.log(`\nwrote ${written.length} stills to ${OUT_DIR} (${(total / 1024).toFixed(0)}KB total)`)
  } finally {
    await browser.close()
    server?.stop()
  }
}

main().catch((err) => {
  console.error(`\ncapture failed: ${err.message}`)
  process.exit(1)
})
