// The closed building: what the city sees from the street. Pure data — the
// palette, the roof's parts and the window grid — so the shapes are testable
// and a later city can instance them without going through React.
//
// Everything here is deliberately cheap: boxes, a cone, a hemisphere, no
// textures and no per-building materials beyond a colour.
import type { Archetype, RoofStyle } from '@/types/blueprint'
import { FLOOR_D, FLOOR_H, FLOOR_W, SLAB_T } from './model'

/**
 * Four looks per archetype, indexed by the blueprint's `palette`. Muted
 * enough that a street of them reads as a city rather than a paint chart,
 * and distinct enough that two neighbours of the same archetype differ.
 */
export const PALETTES: Record<Archetype, [string, string, string, string]> = {
  office: ['#b9bec9', '#9aa4b4', '#cbcfd6', '#8d97a6'],
  storefront: ['#d8a06a', '#c97f5e', '#e0bd8c', '#b86f52'],
  bank: ['#c9c3b0', '#aeb6a8', '#d7d2be', '#98a08f'],
  workshop: ['#8fa0a8', '#7b8c99', '#a6b3b8', '#6d7b87'],
  theatre: ['#a87a97', '#8e6a8c', '#c0929f', '#7a5d7d'],
  terminal: ['#93a9bd', '#7f97ad', '#aabccd', '#6f879d'],
  clinic: ['#cdd8d6', '#b4c6c4', '#dee6e3', '#9fb4b2'],
  library: ['#c2b294', '#ab9c80', '#d5c8ae', '#96876e'],
}

/** Wall colour for a facade. `palette` is wrapped, so a stale index is safe. */
export function paletteColor(archetype: Archetype, palette: number): string {
  const entries = PALETTES[archetype] ?? PALETTES.office
  const i = ((Math.trunc(palette) % entries.length) + entries.length) % entries.length
  return entries[i]
}

// Listed as a Record so the compiler, not a reviewer, is what notices a new
// roof style arriving in the blueprint types.
const ROOF_STYLE_KEYS: Record<RoofStyle, true> = {
  flat: true,
  gable: true,
  hip: true,
  mansard: true,
  sawtooth: true,
  dome: true,
  parapet: true,
}

export const ROOF_STYLES = Object.keys(ROOF_STYLE_KEYS) as RoofStyle[]

export type RoofPart =
  /** A flat deck. */
  | { kind: 'slab'; y: number; w: number; d: number; h: number }
  /** A low wall standing on the edge of the deck below it. */
  | { kind: 'parapet'; y: number; h: number; w: number; d: number }
  /** A triangular prism; `along` is the axis its ridge runs down. */
  | { kind: 'prism'; x: number; z: number; y: number; w: number; d: number; h: number; along: 'x' | 'z' }
  /** A four-sided pyramid, truncated when `topScale` is above 0. */
  | { kind: 'pyramid'; y: number; w: number; d: number; h: number; topScale: number }
  | { kind: 'dome'; y: number; r: number; h: number }

/**
 * The roof of a building `w × d` across whose walls stop at y = 0 (the caller
 * lifts the whole group). Every style starts with the deck the walls carry,
 * so a roof always closes the box even when its shape is unfamiliar.
 */
export function roofParts(style: RoofStyle, w = FLOOR_W, d = FLOOR_D): RoofPart[] {
  const deck: RoofPart = { kind: 'slab', y: 0, w, d, h: 0.18 }
  const top = 0.18
  switch (style) {
    case 'flat':
      return [deck]
    case 'parapet':
      return [deck, { kind: 'parapet', y: top, h: 0.55, w, d }]
    case 'gable':
      return [deck, { kind: 'prism', x: 0, z: 0, y: top, w, d, h: 1.6, along: 'x' }]
    case 'hip':
      return [deck, { kind: 'pyramid', y: top, w: w + 0.3, d: d + 0.3, h: 1.5, topScale: 0 }]
    case 'mansard':
      return [
        deck,
        { kind: 'pyramid', y: top, w: w + 0.3, d: d + 0.3, h: 1.1, topScale: 0.55 },
        { kind: 'slab', y: top + 1.1, w: w * 0.55, d: d * 0.55, h: 0.12 },
      ]
    case 'sawtooth': {
      const n = 4
      const bay = w / n
      const teeth: RoofPart[] = []
      for (let i = 0; i < n; i++) {
        teeth.push({
          kind: 'prism',
          x: -w / 2 + bay * (i + 0.5),
          z: 0,
          y: top,
          w: bay,
          d,
          h: 0.85,
          along: 'z',
        })
      }
      return [deck, ...teeth]
    }
    case 'dome': {
      const r = Math.min(w, d) * 0.32
      return [
        deck,
        { kind: 'slab', y: top, w: r * 2.4, d: r * 2.4, h: 0.35 },
        { kind: 'dome', y: top + 0.35, r, h: r * 0.9 },
      ]
    }
  }
}

export interface Window {
  /** Which face the window sits on. */
  face: 'north' | 'south' | 'east' | 'west'
  x: number
  y: number
  z: number
  /** Lights on behind this one. */
  lit: boolean
}

// mulberry32, the same tiny PRNG the rest of the site seeds its choices with.
function rng(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const WINDOW_INSET = 0.05

/**
 * One window per bay per storey on all four faces, with a seeded scatter of
 * blanks and lit rooms so two buildings of the same size still differ.
 */
export function windowGrid(floors: number, seed: number, w = FLOOR_W, d = FLOOR_D): Window[] {
  const out: Window[] = []
  const r = rng(seed ^ 0x9e3779b9)
  const bays = (len: number) => Math.max(2, Math.round(len / 1.35))
  const nx = bays(w)
  const nz = bays(d)
  const at = (n: number, len: number, i: number) => -len / 2 + (len / n) * (i + 0.5)
  for (let f = 0; f < floors; f++) {
    const y = f * FLOOR_H + SLAB_T + (FLOOR_H - SLAB_T) * 0.52
    for (let i = 0; i < nx; i++) {
      const x = at(nx, w, i)
      for (const face of ['north', 'south'] as const) {
        if (r() < 0.12) continue
        const z = (face === 'south' ? d / 2 : -d / 2) - (face === 'south' ? WINDOW_INSET : -WINDOW_INSET)
        out.push({ face, x, y, z, lit: r() < 0.38 })
      }
    }
    for (let i = 0; i < nz; i++) {
      const z = at(nz, d, i)
      for (const face of ['east', 'west'] as const) {
        if (r() < 0.12) continue
        const x = (face === 'east' ? w / 2 : -w / 2) - (face === 'east' ? WINDOW_INSET : -WINDOW_INSET)
        out.push({ face, x, y, z, lit: r() < 0.38 })
      }
    }
  }
  return out
}

/** Total height of a closed building with this many floors. */
export const shellHeight = (floors: number): number => floors * FLOOR_H + SLAB_T
