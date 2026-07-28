// Draft-time generator: renders a post's OG card to a committed PNG and points
// the post's `image` frontmatter at it. Not part of `pnpm build` — cards are
// committed artifacts, so a content-only change never needs a browser.
//
//   pnpm og:card <slug>        # one post (works for drafts)
//   pnpm og:card --all-missing # every published post lacking a card
//   pnpm og:card --all         # regenerate every published post
import fs from 'node:fs'
import path from 'node:path'
import { chromium } from 'playwright'
import { loadPosts } from '../lib/posts'
import { renderCardHtml } from './card-template'
import { CARD_W, CARD_H } from './brand'

const CONTENT_DIR = path.resolve('content/blog')
const OG_DIR = path.resolve('public/blog/og')

function formatDate(date: string): string {
  return new Date(`${date}T00:00:00`).toLocaleDateString('en-US', {
    month: 'long',
    year: 'numeric',
  })
}

// Insert or replace `image:` inside the post's existing frontmatter block
// without reserializing the YAML, which would reflow the author's quoting.
function ensureImageFrontmatter(file: string, rel: string) {
  const raw = fs.readFileSync(file, 'utf-8')
  const m = raw.match(/^(---\r?\n[\s\S]*?)(\r?\n---\r?\n)/)
  if (!m) {
    console.warn(`  ! no frontmatter block in ${path.basename(file)} — add image: '${rel}' manually`)
    return
  }
  if (/^image:/m.test(m[1])) {
    fs.writeFileSync(file, raw.replace(/^image:.*$/m, `image: '${rel}'`))
    return
  }
  fs.writeFileSync(file, `${m[1]}\nimage: '${rel}'${m[2]}${raw.slice(m[0].length)}`)
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
      await page.evaluate(() => (document as unknown as { fonts: { ready: Promise<unknown> } }).fonts.ready)
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

main().catch((err) => {
  console.error('OG card generation failed:\n', err instanceof Error ? err.message : err)
  process.exit(1)
})
