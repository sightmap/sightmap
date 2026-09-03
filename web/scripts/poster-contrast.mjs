// Measures how legible the poster tier's line work is against the page it sits on.
//
//   pnpm posters:contrast
//
// Why this exists: the fallback the stills replaced drew the building in white
// stroke, and the day stage behind it is cream. White on cream is not a subtle
// contrast problem, it is an invisible drawing — and because the poster is also
// what every visitor sees before the WebGL chunk arrives, it was the site's
// first impression. The replacement has to be checked, not assumed, so the
// check is a committed script that anyone can re-run.
//
// Method. Each still is transparent, so it is composited over the exact day
// backdrop from building.css (.bld-stage::before) before anything is measured.
// Then, over the region the drawing actually occupies:
//   - "line work" is the darkest 2% of composited pixels — the structural
//     strokes, which are what carries the drawing;
//   - "backdrop" is the median luminance of the pixels the drawing does not
//     cover, i.e. the cream it has to stand out from.
// The reported figure is the WCAG 2.1 contrast ratio between those two. 3:1 is
// the WCAG floor for non-text graphics (SC 1.4.11), and that is the bar used
// here: these stills are graphics, not body text.
import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import puppeteer from 'puppeteer-core'

const here = dirname(fileURLToPath(import.meta.url))
const POSTER_DIR = resolve(here, '../public/building')
// The day backdrop is .bld-stage::before:
//   radial-gradient(120% 90% at 72% 8%, #e9eefb 0%, rgba(250,248,246,0) 55%),
//   linear-gradient(180deg, #f7f4ef 0%, #f1ece4 100%)
// Only the linear stops are reproduced below (see the note at the fill).
const WCAG_GRAPHICS_FLOOR = 3

const CHROME_CANDIDATES = [
  process.env.CHROME,
  '/usr/bin/chromium',
  '/usr/bin/chromium-browser',
  '/usr/bin/google-chrome-stable',
  '/usr/bin/google-chrome',
].filter(Boolean)

/** Relative luminance, WCAG 2.1 §relative-luminance. */
function luminance(r, g, b) {
  const channel = (v) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)
}

const ratio = (l1, l2) => (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05)

const hex = ([r, g, b]) => `#${[r, g, b].map((v) => Math.round(v).toString(16).padStart(2, '0')).join('')}`

/**
 * The legacy fallback's numbers need no sampling: it stroked pure white at a
 * known alpha over a known gradient, and alpha compositing is arithmetic.
 * Reported as the range across the backdrop's two extremes.
 */
function legacyBaseline() {
  const strokes = [
    { name: 'floor plates', color: [255, 255, 255], alpha: 0.9 },
    { name: 'room blocks', color: [255, 255, 255], alpha: 0.55 },
  ]
  const stops = [
    { name: 'top of gradient #f7f4ef', rgb: [247, 244, 239] },
    { name: 'foot of gradient #f1ece4', rgb: [241, 236, 228] },
  ]
  const rows = []
  for (const stroke of strokes) {
    for (const stop of stops) {
      const composite = stroke.color.map((c, i) => c * stroke.alpha + stop.rgb[i] * (1 - stroke.alpha))
      rows.push({
        what: `${stroke.name} (white @ ${stroke.alpha})`,
        over: stop.name,
        ink: hex(composite),
        ratio: ratio(luminance(...composite), luminance(...stop.rgb)),
      })
    }
  }
  return rows
}

async function measureStills(files) {
  const chrome = CHROME_CANDIDATES.find((p) => existsSync(p))
  if (!chrome) throw new Error(`no Chrome found; set CHROME=/path/to/chrome`)
  const browser = await puppeteer.launch({
    executablePath: chrome,
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  })
  try {
    const page = await browser.newPage()
    await page.setViewport({ width: 1440, height: 900, deviceScaleFactor: 1 })
    await page.setContent('<body style="margin:0"></body>')

    const results = []
    for (const file of files) {
      const dataUrl = `data:image/webp;base64,${readFileSync(join(POSTER_DIR, file)).toString('base64')}`
      const measured = await page.evaluate(
        async (src) => {
          const img = new Image()
          img.src = src
          await img.decode()
          const canvas = document.createElement('canvas')
          canvas.width = img.naturalWidth
          canvas.height = img.naturalHeight
          const ctx = canvas.getContext('2d', { willReadFrequently: true })
          // Paint the page's own backdrop first, then the transparent still on
          // top of it — exactly the stack a visitor sees.
          // The gradient has to be rasterised to be sampled, and canvas has no
          // CSS-gradient parser, so approximate it with the same two stops: the
          // vertical linear gradient dominates, and the radial tint only
          // lightens the top right, which can only overstate the backdrop's
          // brightness slightly.
          const grad = ctx.createLinearGradient(0, 0, 0, canvas.height)
          grad.addColorStop(0, '#f7f4ef')
          grad.addColorStop(1, '#f1ece4')
          ctx.fillStyle = grad
          ctx.fillRect(0, 0, canvas.width, canvas.height)
          const backdropPixels = ctx.getImageData(0, 0, canvas.width, canvas.height).data
          ctx.drawImage(img, 0, 0)
          const composited = ctx.getImageData(0, 0, canvas.width, canvas.height).data

          const lum = (r, g, b) => {
            const c = (v) => {
              const s = v / 255
              return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
            }
            return 0.2126 * c(r) + 0.7152 * c(g) + 0.0722 * c(b)
          }

          const drawn = []
          const bare = []
          // Every 2nd pixel in each direction: a quarter of the work, and at
          // 2880x1800 that is still ~1.3M samples per still.
          for (let i = 0; i < composited.length; i += 8) {
            const changed =
              composited[i] !== backdropPixels[i] ||
              composited[i + 1] !== backdropPixels[i + 1] ||
              composited[i + 2] !== backdropPixels[i + 2]
            const l = lum(composited[i], composited[i + 1], composited[i + 2])
            if (changed) drawn.push({ l, rgb: [composited[i], composited[i + 1], composited[i + 2]] })
            else bare.push(l)
          }
          if (!drawn.length) return null
          drawn.sort((a, b) => a.l - b.l)
          bare.sort((a, b) => a - b)
          const darkest = drawn[Math.floor(drawn.length * 0.02)]
          const backdropLum = bare.length ? bare[Math.floor(bare.length / 2)] : lum(244, 240, 233)
          return {
            coverage: drawn.length / (drawn.length + bare.length),
            inkRgb: darkest.rgb,
            inkLum: darkest.l,
            backdropLum,
          }
        },
        dataUrl,
      )
      if (!measured) throw new Error(`${file} composited to nothing — that still is blank`)
      results.push({
        file,
        coverage: measured.coverage,
        ink: hex(measured.inkRgb),
        ratio: ratio(measured.inkLum, measured.backdropLum),
      })
    }
    return results
  } finally {
    await browser.close()
  }
}

async function main() {
  const files = readdirSync(POSTER_DIR)
    .filter((f) => f.startsWith('poster-desktop-') && f.endsWith('.webp'))
    .sort()
  if (!files.length) throw new Error(`no stills in ${POSTER_DIR} — run \`pnpm posters\` first`)

  console.log('BEFORE — the SVG fallback these stills replace (arithmetic, not sampled):\n')
  for (const row of legacyBaseline()) {
    console.log(`  ${row.what.padEnd(28)} over ${row.over.padEnd(26)} ${row.ink}  ${row.ratio.toFixed(2)}:1`)
  }

  console.log('\nAFTER — captured stills, composited over the same day backdrop:\n')
  const results = await measureStills(files)
  for (const r of results) {
    const chapter = r.file.replace(/^poster-desktop-\d+-|\.webp$/g, '')
    const flag = r.ratio >= WCAG_GRAPHICS_FLOOR ? 'ok  ' : 'FAIL'
    console.log(
      `  ${flag} ${chapter.padEnd(14)} line work ${r.ink}  ${r.ratio.toFixed(2)}:1` +
        `  (drawing covers ${(r.coverage * 100).toFixed(0)}% of frame)`,
    )
  }

  const worst = results.reduce((a, b) => (a.ratio < b.ratio ? a : b))
  console.log(`\nworst chapter: ${worst.file} at ${worst.ratio.toFixed(2)}:1 (floor is ${WCAG_GRAPHICS_FLOOR}:1)`)
  if (worst.ratio < WCAG_GRAPHICS_FLOOR) process.exitCode = 1
}

main().catch((err) => {
  console.error(`contrast check failed: ${err.message}`)
  process.exit(1)
})
