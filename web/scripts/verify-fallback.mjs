// Proves the poster tier actually tells the nine-beat story.
//
//   pnpm posters:verify                    # against a fresh dev server
//   pnpm posters:verify -- --url=http://localhost:5173/building
//   pnpm posters:verify -- --out=/tmp/fallback   # keep the frames
//
// This is the acceptance test for the fallback: with WebGL denied, scroll the
// whole page and assert that all nine chapters show a DIFFERENT still. The
// failure it exists to catch is the one that shipped before — every chapter
// rendering the same unchanging image, so nine screens of copy had one picture
// between them, and the page looked functional while the visual argument was
// missing entirely.
//
// It asserts three separate things, because each has its own way of lying:
//   - nine .bld-chapter nodes, before trusting any frame (the dev SPA sometimes
//     fails to hydrate after a navigation and leaves an empty root; blank
//     frames and poster frames look alike);
//   - the still the page believes is active matches the chapter that is active;
//   - the rendered frames are pixel-distinct from each other, which is the only
//     check that survives the wiring being right and the artwork being wrong.
import { spawn } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import puppeteer from 'puppeteer-core'

const here = dirname(fileURLToPath(import.meta.url))
const webRoot = resolve(here, '..')
const DEV_PORT = 5198
const CHAPTERS_EXPECTED = 9

const CHROME_CANDIDATES = [
  process.env.CHROME,
  '/usr/bin/chromium',
  '/usr/bin/chromium-browser',
  '/usr/bin/google-chrome-stable',
  '/usr/bin/google-chrome',
].filter(Boolean)

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

function parseArgs(argv) {
  const args = { url: null, out: null }
  for (const arg of argv) {
    const [key, value] = arg.replace(/^--/, '').split('=')
    if (key === 'url') args.url = value
    else if (key === 'out') args.out = value
    else {
      console.error(`unknown argument: ${arg}`)
      process.exit(1)
    }
  }
  return args
}

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
  throw new Error(`no server at ${url}`)
}

async function startDevServer() {
  const child = spawn('pnpm', ['dev', '--port', String(DEV_PORT), '--strictPort'], {
    cwd: webRoot,
    stdio: ['ignore', 'ignore', 'inherit'],
  })
  const url = `http://localhost:${DEV_PORT}/building`
  await waitForServer(url)
  return { url, stop: () => child.kill('SIGTERM') }
}

async function main() {
  const args = parseArgs(process.argv.slice(2))
  const outDir = args.out ? resolve(args.out) : null
  if (outDir) mkdirSync(outDir, { recursive: true })

  const chrome = CHROME_CANDIDATES.find((p) => existsSync(p))
  if (!chrome) throw new Error('no Chrome found; set CHROME=/path/to/chrome')

  let server = null
  let url = args.url
  if (!url) {
    server = await startDevServer()
    url = server.url
  } else {
    // A bare origin means "the dev server is over there", not "verify the home
    // page". Without this, --url=http://localhost:5173 checks the wrong route
    // and fails as a hydration error, which is a maddening thing to debug.
    if (new URL(url).pathname === '/') url = new URL('/building', url).href
    await waitForServer(url)
  }

  const browser = await puppeteer.launch({
    executablePath: chrome,
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage', '--hide-scrollbars'],
  })

  try {
    const page = await browser.newPage()
    await page.setViewport({ width: 1440, height: 900, deviceScaleFactor: 1 })
    // Deny WebGL the way a device without it would: the context request comes
    // back null. NOT --disable-gpu, which breaks the page in a different way
    // and would make this test pass against a scene that never rendered.
    await page.evaluateOnNewDocument(() => {
      const real = HTMLCanvasElement.prototype.getContext
      HTMLCanvasElement.prototype.getContext = function (type, ...rest) {
        if (typeof type === 'string' && type.includes('webgl')) return null
        return real.call(this, type, ...rest)
      }
    })
    // The dev SPA intermittently comes up with an empty root, and a failure to
    // hydrate looks exactly like the bug this script hunts: no chapters, no
    // stills, a blank frame. Reload rather than report a false negative — but
    // give up loudly rather than loop, because a page that never hydrates in
    // four tries is a real failure and not a flake.
    // Note this waits on the poster, not on a canvas: WebGL is denied here, so
    // there is no canvas to wait for and its absence is the point.
    let chapterCount = 0
    for (let attempt = 1; attempt <= 4; attempt++) {
      await page.goto(url, { waitUntil: 'networkidle2', timeout: 60_000 })
      try {
        await page.waitForFunction(
          (expected) =>
            document.querySelectorAll('.bld-chapter').length === expected &&
            !!document.querySelector('.bld-poster__still[data-active="true"]'),
          { timeout: 20_000 },
          CHAPTERS_EXPECTED,
        )
        chapterCount = CHAPTERS_EXPECTED
        break
      } catch {
        chapterCount = await page.evaluate(() => document.querySelectorAll('.bld-chapter').length)
        console.warn(`  hydration incomplete (attempt ${attempt}: ${chapterCount} chapters) — reloading`)
      }
    }
    if (chapterCount !== CHAPTERS_EXPECTED) {
      throw new Error(
        `page never hydrated: ${chapterCount} chapters after 4 attempts, expected ${CHAPTERS_EXPECTED}`,
      )
    }
    const hasCanvas = await page.evaluate(() => !!document.querySelector('.bld-stage canvas'))
    if (hasCanvas) throw new Error('a canvas exists — WebGL was not actually denied, so this proves nothing')

    const chapters = await page.evaluate(() =>
      [...document.querySelectorAll('.bld-chapter')].map((el) => el.dataset.chapter),
    )

    const seen = new Map()
    for (const [index, id] of chapters.entries()) {
      await page.evaluate((i) => {
        const el = document.querySelectorAll('.bld-chapter')[i]
        window.scrollTo({ top: Math.max(0, el.offsetTop + el.offsetHeight / 2 - window.innerHeight / 2) })
      }, index)
      // The active still must be this chapter's, and it must have actually
      // decoded — an <img> that 404s is still "active" and still blank.
      await page.waitForFunction(
        (chapter) => {
          const img = document.querySelector('.bld-poster__still[data-active="true"]')
          return !!img && img.dataset.chapter === chapter && img.complete && img.naturalWidth > 0
        },
        { timeout: 15_000 },
        id,
      )
      await sleep(700) // cross-fade
      const frame = await page.screenshot({ type: 'png' })
      const hash = createHash('sha256').update(frame).digest('hex').slice(0, 12)
      if (outDir) writeFileSync(join(outDir, `fallback-${String(index).padStart(2, '0')}-${id}.png`), frame)
      const clash = seen.get(hash)
      if (clash) throw new Error(`chapter "${id}" renders identically to "${clash}" — the stills are not swapping`)
      seen.set(hash, id)
      console.log(`  ok  ${String(index).padStart(2, '0')} ${id.padEnd(14)} ${(frame.length / 1024).toFixed(0)}KB  ${hash}`)
    }

    console.log(`\n${seen.size}/${CHAPTERS_EXPECTED} chapters render distinct stills with WebGL denied`)
    if (outDir) console.log(`frames written to ${outDir}`)
  } finally {
    await browser.close()
    server?.stop()
  }
}

main().catch((err) => {
  console.error(`\nfallback verification failed: ${err.message}`)
  process.exit(1)
})
