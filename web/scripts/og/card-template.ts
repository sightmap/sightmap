// Builds the HTML document for a blog OG card. Rendered to PNG by
// generate-og-card.ts via Playwright at 1200x630, deviceScaleFactor 2.
import { CARD_W, CARD_H, BG, BG_CODE, TEXT, MUTED, ACCENT, BORDER, SANS, MONO, seedOf } from './brand'

export interface CardMeta {
  title: string
  topic: string
  author: string
  date: string // already formatted, e.g. "July 2026"
}

// The decorative rule under the wordmark. Its width varies per post so cards
// in a feed do not look stamped from one plate.
function accentRule(slug: string): string {
  const width = 180 + (seedOf(slug) % 120)
  return `<div style="width:${width}px;height:4px;background:${ACCENT};border-radius:2px;margin:26px 0 34px"></div>`
}

export function renderCardHtml(meta: CardMeta, slug: string): string {
  return `<!doctype html><html><head><meta charset="utf8">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;700&family=DM+Sans:wght@400;500;700&display=swap" rel="stylesheet">
<style>
  *{margin:0;box-sizing:border-box}
  body{width:${CARD_W}px;height:${CARD_H}px;background:${BG};overflow:hidden}
  .card{position:relative;width:${CARD_W}px;height:${CARD_H}px;padding:72px}
  .wordmark{font-family:${MONO};font-weight:700;font-size:30px;color:${TEXT};letter-spacing:-.02em}
  .wordmark span{color:${ACCENT}}
  .topic{font-family:${MONO};font-weight:700;font-size:17px;letter-spacing:.16em;text-transform:uppercase;color:${ACCENT}}
  .title{font-family:${SANS};font-weight:700;font-size:62px;line-height:1.1;color:${TEXT};margin-top:18px;max-width:840px;letter-spacing:-.03em}
  .foot{position:absolute;left:72px;bottom:60px;font-family:${MONO};font-weight:400;font-size:19px;color:${MUTED}}
  .chip{position:absolute;right:72px;bottom:56px;background:${BG_CODE};border:1px solid ${BORDER};border-radius:10px;padding:16px 20px;font-family:${MONO};font-size:15px;line-height:1.6;color:#c9d1d9}
  .chip .k{color:#7eb6f6}
  .chip .s{color:#7ee8a8}
</style></head>
<body><div class="card">
  <!-- Wordmark is set as type until the Sightmap mark lands. When it does,
       drop the SVG at public/brand/mark/sightmap-mark.svg, read it here the
       way Subtext's card-template.ts does, and delete the .wordmark rule. -->
  <div class="wordmark">.sightmap<span>/</span></div>
  ${accentRule(slug)}
  <div class="topic">${meta.topic}</div>
  <div class="title">${meta.title}</div>
  <div class="foot">${meta.author} &nbsp;·&nbsp; ${meta.date}</div>
  <div class="chip"><span class="k">views:</span><br>&nbsp;&nbsp;- <span class="k">name:</span> <span class="s">Checkout</span></div>
</div></body></html>`
}
