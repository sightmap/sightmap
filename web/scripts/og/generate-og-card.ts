// Draft-time generator: renders a post's OG card to a committed PNG and points
// the post's `image` frontmatter at it. Not part of `pnpm build` — cards are
// committed artifacts, so a content-only change never needs a browser.
//
//   pnpm og:card <slug>        # one post (works for drafts)
//   pnpm og:card --all-missing # every published post lacking a card
//   pnpm og:card --all         # regenerate every published post
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { chromium, type Page } from 'playwright'
import { loadPosts } from '../lib/posts'
import { renderCardHtml } from './card-template'
import { CARD_W, CARD_H } from './brand'

const CONTENT_DIR = path.resolve('content/blog')
const OG_DIR = path.resolve('public/blog/og')

// Every distinct family/weight/size the template actually sets, so
// `document.fonts.check()` verifies real availability rather than just
// asking whether the network requests finished (which `fonts.ready` does,
// regardless of whether they succeeded). Kept in sync with card-template.ts.
const REQUIRED_FONTS: { label: string; css: string }[] = [
  { label: "JetBrains Mono 700 (.wordmark/.topic)", css: "700 30px 'JetBrains Mono'" },
  { label: "JetBrains Mono 400 (.foot/.chip)", css: "400 19px 'JetBrains Mono'" },
  { label: "DM Sans 700 (.title)", css: "700 62px 'DM Sans'" },
]

// `document.fonts.ready` resolves whether or not the webfont requests
// actually succeeded, and Playwright's `networkidle` only waits for requests
// to settle, not to succeed — so a blocked, slow, or offline Google Fonts
// fetch silently renders in fallback system fonts and would otherwise
// produce a normal-looking, exit-0 PNG with no signal. These cards are
// committed artifacts a human may never re-open, so fail loudly instead of
// writing one.
//
// `document.fonts.check()` is NOT sufficient here — verified empirically: when
// the stylesheet fails to load, zero FontFace entries are ever registered in
// document.fonts (fonts.size === 0), and per the CSS Font Loading Module spec
// check() optimistically returns true when it has no @font-face declaration
// to contradict it, regardless of whether the named family is actually
// available. `document.fonts.load()` instead performs a real load attempt
// and resolves an empty FontFace[] when it fails, so that is what we use.
async function assertFontsLoaded(page: Page, slug: string): Promise<void> {
  const results = await page.evaluate(
    (specs) => Promise.all(specs.map((s) => document.fonts.load(s.css).then((faces) => faces.length))),
    REQUIRED_FONTS
  )
  const missing = REQUIRED_FONTS.filter((_, i) => results[i] === 0)
  if (missing.length > 0) {
    throw new Error(
      `card for "${slug}" was NOT written: font(s) failed to load — ${missing
        .map((m) => m.label)
        .join(', ')}. The Google Fonts request may be blocked, slow, or offline.`
    )
  }
}

function formatDate(date: string): string {
  return new Date(`${date}T00:00:00`).toLocaleDateString('en-US', {
    month: 'long',
    year: 'numeric',
  })
}

// Insert or replace `image:` inside the post's existing frontmatter block
// without reserializing the YAML, which would reflow the author's quoting.
// Pure string transform (no fs) so the insert/replace logic — including the
// line-ending handling — can be unit tested directly; ensureImageFrontmatter
// below is the thin fs-backed wrapper the CLI entry point calls.
export function patchImageFrontmatter(raw: string, rel: string, filename: string): string | null {
  const m = raw.match(/^(---\r?\n[\s\S]*?)(\r?\n---\r?\n)/)
  if (!m) {
    console.warn(`  ! no frontmatter block in ${filename} — add image: '${rel}' manually`)
    return null
  }
  if (/^image:/m.test(m[1])) {
    return raw.replace(/^image:.*$/m, `image: '${rel}'`)
  }
  // Match the frontmatter block's own line ending instead of hardcoding \n,
  // so a CRLF post doesn't end up with a bare-LF line silently mixed in.
  const eol = m[0].includes('\r\n') ? '\r\n' : '\n'
  return `${m[1]}${eol}image: '${rel}'${m[2]}${raw.slice(m[0].length)}`
}

function ensureImageFrontmatter(file: string, rel: string) {
  const raw = fs.readFileSync(file, 'utf-8')
  const updated = patchImageFrontmatter(raw, rel, path.basename(file))
  if (updated === null) return
  fs.writeFileSync(file, updated)
}

async function main() {
  const mode = process.argv[2]
  if (!mode) {
    console.error('usage: og:card <slug> | --all-missing | --all')
    process.exit(1)
  }
  fs.mkdirSync(OG_DIR, { recursive: true })

  const all = await loadPosts(CONTENT_DIR, { includeDrafts: true })
  let targets = all
  if (mode === '--all-missing') {
    targets = all.filter(
      (p) => !p.frontmatter.draft && !fs.existsSync(path.join(OG_DIR, `${p.frontmatter.slug}.png`))
    )
  } else if (mode === '--all') {
    targets = all.filter((p) => !p.frontmatter.draft)
  } else {
    targets = all.filter((p) => p.frontmatter.slug === mode)
  }

  if (targets.length === 0) {
    console.error(`no posts matched "${mode}"`)
    process.exit(1)
  }

  const browser = await chromium.launch()
  try {
    const page = await browser.newPage({
      viewport: { width: CARD_W, height: CARD_H },
      deviceScaleFactor: 2,
    })
    for (const post of targets) {
      const fm = post.frontmatter
      await page.setContent(
        renderCardHtml(
          { title: fm.title, topic: fm.topic, author: fm.author, date: formatDate(fm.date) },
          fm.slug
        ),
        { waitUntil: 'networkidle' }
      )
      await page.evaluate(() => document.fonts.ready)
      await assertFontsLoaded(page, fm.slug)
      // scale: 'css' keeps the file at exactly 1200x630 despite the 2x device
      // scale factor, so it matches the dimensions declared in og:image:width.
      await page.screenshot({
        path: path.join(OG_DIR, `${fm.slug}.png`),
        clip: { x: 0, y: 0, width: CARD_W, height: CARD_H },
        scale: 'css',
      })
      ensureImageFrontmatter(path.join(CONTENT_DIR, post.file), `/blog/og/${fm.slug}.png`)
      console.log(`  ${fm.slug} -> public/blog/og/${fm.slug}.png`)
    }
  } finally {
    await browser.close()
  }
}

// Only run when invoked directly, so the test can import patchImageFrontmatter
// without triggering a real run (see prerender.tsx for the same pattern).
const entry = process.argv[1]
if (entry && import.meta.url === pathToFileURL(entry).href) {
  main().catch((err) => {
    console.error('OG card generation failed:\n', err instanceof Error ? err.message : err)
    process.exit(1)
  })
}
