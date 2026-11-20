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

// ---------------------------------------------------------------------------
// Street level and roofline. Data, not meshes: Shell.tsx is the reference
// consumer, and the city instances the same parts a few hundred times.

/**
 * Which material a part is drawn in. Keeping this to a short list is what
 * lets a whole city be drawn in a handful of instanced meshes.
 */
export type PartTone = 'wall' | 'roof' | 'trim' | 'dark' | 'glass' | 'lit' | 'accent'

export type FacadePart =
  /** A box, optionally tilted about X (a canopy, a sloped panel). */
  | { kind: 'box'; x: number; y: number; z: number; w: number; h: number; d: number; tilt?: number; tone: PartTone }
  /** A cylinder standing on `axis` (a column, a mast, a tank). */
  | { kind: 'cyl'; x: number; y: number; z: number; r: number; h: number; axis: 'x' | 'y' | 'z'; tone: PartTone }

/**
 * One saturated colour per archetype, for awnings, marquees and kerbside
 * props. Never for walls: a street of saturated walls stops reading as a
 * street.
 */
export const ACCENTS: Record<Archetype, string> = {
  office: '#5a6472',
  storefront: '#c9456d',
  bank: '#b8860b',
  workshop: '#2f7d5e',
  theatre: '#8e2740',
  terminal: '#2f6b9b',
  clinic: '#3f8f6a',
  library: '#7a4f2a',
}

const v3 = (variant: number): 0 | 1 | 2 => (((Math.trunc(variant) % 3) + 3) % 3) as 0 | 1 | 2

/** Height of the storey the street sees. */
const GROUND_H = FLOOR_H

/**
 * What stands at the foot of the building on its street face: the one part
 * of a facade a passer-by is close enough to read. Local to the building,
 * y = 0 at the pavement, +Z the street.
 */
export function groundFloorParts(
  archetype: Archetype,
  variant: number,
  w = FLOOR_W,
  d = FLOOR_D
): FacadePart[] {
  const v = v3(variant)
  const out: FacadePart[] = []
  const front = d / 2
  const door = (width = 1.8): void => {
    out.push({ kind: 'box', x: 0, y: 0.95, z: front + 0.02, w: width, h: 1.7, d: 0.14, tone: 'dark' })
    out.push({ kind: 'box', x: 0, y: 1.86, z: front + 0.06, w: width + 0.6, h: 0.16, d: 0.3, tone: 'trim' })
  }
  /** A glazed band across the street face, with piers at each end. */
  const shopfront = (height: number, inset: number): void => {
    out.push({ kind: 'box', x: 0, y: 0.12 + height / 2, z: front - 0.02, w: w - inset * 2, h: height, d: 0.16, tone: 'glass' })
    for (const side of [-1, 1]) {
      out.push({ kind: 'box', x: (side * (w - inset)) / 2, y: GROUND_H / 2, z: front - 0.04, w: inset, h: GROUND_H, d: 0.2, tone: 'trim' })
    }
    out.push({ kind: 'box', x: 0, y: 0.12, z: front, w, h: 0.24, d: 0.3, tone: 'trim' })
  }
  /** A canopy hung over the door, sloping down toward the street. */
  const canopy = (width: number, y: number, tone: PartTone): void => {
    out.push({ kind: 'box', x: 0, y, z: front + 0.7, w: width, h: 0.12, d: 1.5, tilt: -0.34, tone })
    out.push({ kind: 'box', x: 0, y: y - 0.3, z: front + 1.4, w: width, h: 0.3, d: 0.08, tone })
  }

  switch (archetype) {
    case 'storefront':
      shopfront(1.35, 0.5)
      door(1.5)
      canopy(w * 0.8, 2.0 + v * 0.08, 'accent')
      break
    case 'bank': {
      const n = [4, 6, 8][v]
      const span = w - 1.6
      for (let i = 0; i < n; i++) {
        const x = -span / 2 + (span / (n - 1)) * i
        out.push({ kind: 'cyl', x, y: GROUND_H / 2 + 0.2, z: front - 0.5, r: 0.24, h: GROUND_H - 0.4, axis: 'y', tone: 'trim' })
        out.push({ kind: 'box', x, y: 0.12, z: front - 0.5, w: 0.62, h: 0.24, d: 0.62, tone: 'trim' })
      }
      out.push({ kind: 'box', x: 0, y: GROUND_H + 0.05, z: front - 0.5, w: span + 1.0, h: 0.34, d: 1.1, tone: 'trim' })
      out.push({ kind: 'box', x: 0, y: 0.06, z: front + 0.55, w: span + 1.2, h: 0.12, d: 1.1, tone: 'trim' })
      door(1.6)
      break
    }
    case 'workshop': {
      // A roller door wide enough to drive into, and a wicket beside it.
      const bays = [1, 2, 2][v]
      const bw = (w - 2.2) / bays
      for (let i = 0; i < bays; i++) {
        const x = -(w - 2.2) / 2 + bw * (i + 0.5)
        out.push({ kind: 'box', x, y: 1.15, z: front + 0.04, w: bw - 0.2, h: 2.1, d: 0.16, tone: 'dark' })
        for (let k = 0; k < 5; k++) {
          out.push({ kind: 'box', x, y: 0.35 + k * 0.42, z: front + 0.13, w: bw - 0.24, h: 0.06, d: 0.05, tone: 'trim' })
        }
      }
      out.push({ kind: 'box', x: w / 2 - 0.8, y: 0.9, z: front + 0.04, w: 0.9, h: 1.8, d: 0.14, tone: 'glass' })
      out.push({ kind: 'box', x: 0, y: 2.34, z: front + 0.08, w, h: 0.2, d: 0.34, tone: 'accent' })
      break
    }
    case 'theatre': {
      // The marquee, with its lamps.
      const mw = w * 0.66
      out.push({ kind: 'box', x: 0, y: 2.2, z: front + 0.6, w: mw, h: 0.26, d: 1.5, tone: 'dark' })
      out.push({ kind: 'box', x: 0, y: 2.46, z: front + 0.6, w: mw * 0.9, h: 0.22, d: 1.3, tone: 'accent' })
      const lamps = [8, 10, 12][v]
      for (let i = 0; i < lamps; i++) {
        const x = -mw / 2 + (mw / (lamps - 1)) * i
        out.push({ kind: 'box', x, y: 2.0, z: front + 1.32, w: 0.16, h: 0.16, d: 0.08, tone: 'lit' })
      }
      shopfront(1.2, 0.8)
      door(2.0)
      break
    }
    case 'terminal':
      // A glazed lobby the whole width, under a flat canopy.
      shopfront(GROUND_H - 0.5, 0.35)
      canopy(w * [0.8, 0.9, 1.0][v], 2.36, 'trim')
      door(2.0 + v * 0.3)
      break
    case 'clinic':
      shopfront(1.3, 0.7)
      canopy(w * [0.46, 0.55, 0.64][v], 2.1, 'accent')
      door(1.8)
      // The one mark a clinic is known by.
      out.push({ kind: 'box', x: 0, y: 1.76, z: front + 1.46, w: 0.5, h: 0.16, d: 0.05, tone: 'trim' })
      out.push({ kind: 'box', x: 0, y: 1.76, z: front + 1.46, w: 0.16, h: 0.5, d: 0.05, tone: 'trim' })
      break
    case 'library': {
      // A portico: two columns and a pediment over a flight of steps.
      for (const side of [-1, 1]) {
        out.push({ kind: 'cyl', x: side * 1.5, y: GROUND_H / 2 + 0.2, z: front - 0.4, r: 0.22, h: GROUND_H - 0.4, axis: 'y', tone: 'trim' })
      }
      out.push({ kind: 'box', x: 0, y: GROUND_H + 0.05, z: front - 0.4, w: 4.0, h: 0.3, d: 1.0, tone: 'trim' })
      for (let k = 0; k < 2 + v; k++) {
        out.push({ kind: 'box', x: 0, y: 0.05 + k * 0.1, z: front + 0.75 - k * 0.2, w: 4.4 - k * 0.4, h: 0.1, d: 0.5, tone: 'trim' })
      }
      door(1.7)
      break
    }
    case 'office':
      shopfront(1.3 + v * 0.22, 0.6)
      door(1.6 + v * 0.2)
      break
  }
  return out
}

export type RooftopKind = 'tank' | 'antenna' | 'solar' | 'garden'

/**
 * The one thing on the roof. Chosen by archetype and variant so that two
 * neighbours wearing the same look still differ from the street; the parts
 * are local to the top of the building, y = 0 at the roof deck.
 */
const ROOFTOP_KINDS: Record<Archetype, [RooftopKind, RooftopKind, RooftopKind]> = {
  office: ['tank', 'antenna', 'solar'],
  storefront: ['antenna', 'tank', 'garden'],
  bank: ['antenna', 'solar', 'tank'],
  workshop: ['tank', 'solar', 'antenna'],
  theatre: ['antenna', 'tank', 'solar'],
  terminal: ['antenna', 'solar', 'tank'],
  clinic: ['solar', 'garden', 'tank'],
  library: ['garden', 'tank', 'solar'],
}

export function rooftopItem(
  archetype: Archetype,
  variant: number,
  w = FLOOR_W,
  d = FLOOR_D
): { kind: RooftopKind; parts: FacadePart[] } {
  const kind = (ROOFTOP_KINDS[archetype] ?? ROOFTOP_KINDS.office)[v3(variant)]
  const parts: FacadePart[] = []
  const x0 = -w / 2 + 1.6
  const z0 = -d / 2 + 1.4
  switch (kind) {
    case 'tank':
      for (const side of [-1, 1]) {
        for (const front of [-1, 1]) {
          parts.push({ kind: 'box', x: x0 + side * 0.55, y: 0.3, z: z0 + front * 0.55, w: 0.14, h: 0.6, d: 0.14, tone: 'dark' })
        }
      }
      parts.push({ kind: 'cyl', x: x0, y: 1.15, z: z0, r: 0.72, h: 1.1, axis: 'y', tone: 'trim' })
      parts.push({ kind: 'cyl', x: x0, y: 1.76, z: z0, r: 0.78, h: 0.12, axis: 'y', tone: 'dark' })
      break
    case 'antenna':
      parts.push({ kind: 'box', x: x0, y: 0.15, z: z0, w: 1.2, h: 0.3, d: 1.2, tone: 'dark' })
      parts.push({ kind: 'cyl', x: x0, y: 1.9, z: z0, r: 0.07, h: 3.2, axis: 'y', tone: 'dark' })
      for (const y of [1.2, 2.2]) {
        parts.push({ kind: 'box', x: x0, y, z: z0, w: 0.9, h: 0.06, d: 0.06, tone: 'dark' })
      }
      parts.push({ kind: 'box', x: x0, y: 3.55, z: z0, w: 0.16, h: 0.16, d: 0.16, tone: 'lit' })
      break
    case 'solar':
      for (let i = 0; i < 3; i++) {
        for (let j = 0; j < 2; j++) {
          const x = -w / 2 + 1.4 + i * 1.4
          const z = -d / 2 + 1.2 + j * 1.2
          parts.push({ kind: 'box', x, y: 0.34, z, w: 1.2, h: 0.06, d: 0.95, tilt: -0.34, tone: 'dark' })
          parts.push({ kind: 'box', x, y: 0.14, z: z + 0.3, w: 0.07, h: 0.28, d: 0.07, tone: 'trim' })
        }
      }
      break
    case 'garden':
      for (let i = 0; i < 3; i++) {
        const x = -w / 2 + 1.6 + i * 1.9
        parts.push({ kind: 'box', x, y: 0.2, z: z0, w: 1.5, h: 0.4, d: 1.2, tone: 'trim' })
        parts.push({ kind: 'cyl', x, y: 0.7, z: z0, r: 0.45, h: 0.6, axis: 'y', tone: 'accent' })
      }
      break
  }
  return { kind, parts }
}

/**
 * The mullions and transoms around the windows the mask actually left
 * standing, rather than a grid drawn over the whole wall: a blank bay gets
 * no frame, so the pattern of lit, dark and bricked-up bays reads from the
 * street. Takes the output of `windowGrid`.
 */
export function windowMullions(windows: Window[], w = FLOOR_W, d = FLOOR_D): FacadePart[] {
  const out: FacadePart[] = []
  const runs = new Map<string, Window[]>()
  for (const win of windows) {
    const key = `${win.face}|${win.y.toFixed(3)}`
    const list = runs.get(key)
    if (list) list.push(win)
    else runs.set(key, [win])
  }
  const along = (win: Window) => (win.face === 'east' || win.face === 'west' ? win.z : win.x)
  const bayOf = (len: number) => len / Math.max(2, Math.round(len / 1.35))
  for (const list of runs.values()) {
    const face = list[0].face
    const sideways = face === 'east' || face === 'west'
    const bay = bayOf(sideways ? d : w)
    const sorted = [...list].sort((a, b) => along(a) - along(b))
    const y = sorted[0].y
    const at = (u: number, thick: number, height: number, depth: number): FacadePart =>
      sideways
        ? { kind: 'box', x: sorted[0].x, y, z: u, w: depth, h: height, d: thick, tone: 'dark' }
        : { kind: 'box', x: u, y, z: sorted[0].z, w: thick, h: height, d: depth, tone: 'dark' }
    const edges = new Set<number>()
    for (const win of sorted) {
      edges.add(Math.round((along(win) - bay / 2) * 1000) / 1000)
      edges.add(Math.round((along(win) + bay / 2) * 1000) / 1000)
    }
    for (const u of edges) out.push(at(u, 0.08, 1.0, 0.1))
    // A transom over and under each unbroken run of windows.
    let start = 0
    for (let i = 1; i <= sorted.length; i++) {
      const broken = i === sorted.length || along(sorted[i]) - along(sorted[i - 1]) > bay * 1.5
      if (!broken) continue
      const a = along(sorted[start]) - bay / 2
      const b = along(sorted[i - 1]) + bay / 2
      const mid = (a + b) / 2
      for (const dy of [-0.5, 0.5]) {
        const part = at(mid, b - a, 0.09, 0.1)
        part.y += dy
        out.push(part)
      }
      start = i
    }
  }
  return out
}
