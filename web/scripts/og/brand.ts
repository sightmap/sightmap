// Node-side copy of the card palette. These mirror the :root tokens in
// src/index.css; the renderer runs in a headless browser with no access to the
// app's stylesheet, so they are duplicated here deliberately.
export const CARD_W = 1200
export const CARD_H = 630

export const BG = '#faf8f6'
export const BG_CODE = '#1a1a2e'
export const TEXT = '#1a1714'
export const MUTED = '#8a8272'
export const ACCENT = '#c9456d'
export const BORDER = '#e5e0da'

export const SANS = "'DM Sans'"
export const MONO = "'JetBrains Mono'"

// Deterministic per-slug value, so a given post always gets the same
// decorative treatment across regenerations.
export function seedOf(slug: string): number {
  let h = 0
  for (let i = 0; i < slug.length; i++) h = (h * 31 + slug.charCodeAt(i)) >>> 0
  return h
}
