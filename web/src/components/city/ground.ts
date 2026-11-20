// The ground, drawn once into a canvas and used as the texture of a single
// plane. Roads, kerbs, markings, lot lines, district tint and the contact
// shadow under every building are pixels rather than geometry, which is what
// keeps a city of three hundred lots inside a handful of draw calls.
//
// Pure: the only thing this module touches is a 2D context, and it only uses
// the part of one a stub can implement, so the drawing is testable without a
// canvas.
import type { CityLot, CityPlan, CityRoad } from '@/types/city'
import { GROUND, PATH_SURFACE, ROADS, accent } from './palette'

/** The slice of a 2D context the ground needs. A stub implements this. */
export interface GroundContext {
  fillStyle: CanvasRenderingContext2D['fillStyle']
  strokeStyle: CanvasRenderingContext2D['strokeStyle']
  lineWidth: number
  lineJoin: CanvasLineJoin
  lineCap: CanvasLineCap
  globalAlpha: number
  save(): void
  restore(): void
  translate(x: number, y: number): void
  scale(x: number, y: number): void
  beginPath(): void
  moveTo(x: number, y: number): void
  lineTo(x: number, y: number): void
  closePath(): void
  arc(x: number, y: number, r: number, a0: number, a1: number, ccw?: boolean): void
  rect(x: number, y: number, w: number, h: number): void
  fillRect(x: number, y: number, w: number, h: number): void
  fill(rule?: CanvasFillRule): void
  stroke(): void
  setLineDash(segments: number[]): void
}

/** Texels per world unit. Enough that a kerb is two pixels wide. */
export const PIXELS_PER_UNIT = 4.5

/** Where the sun is, so every contact shadow leans the same way. */
export const SHADOW_OFFSET: [number, number] = [-1.6, 2.2]

export interface GroundOptions {
  /** Lots carrying a building, which get a contact shadow. */
  built?: ReadonlySet<number>
  /** Lots with nothing on them, which are dirt rather than paving. */
  empty?: ReadonlySet<number>
  pixelsPerUnit?: number
}

export function groundPixels(plan: CityPlan, ppu = PIXELS_PER_UNIT): { width: number; height: number } {
  return { width: Math.round(plan.bounds.w * ppu), height: Math.round(plan.bounds.d * ppu) }
}

type Pt = [number, number]

/** Intersection of two lines given as point + direction. Null when parallel. */
function lineCross(p: Pt, d1: Pt, q: Pt, d2: Pt): Pt | null {
  const det = d1[0] * d2[1] - d1[1] * d2[0]
  if (Math.abs(det) < 1e-6) return null
  const t = ((q[0] - p[0]) * d2[1] - (q[1] - p[1]) * d2[0]) / det
  return [p[0] + d1[0] * t, p[1] + d1[1] * t]
}

const norm = (a: Pt, b: Pt): Pt => {
  const dx = b[0] - a[0]
  const dz = b[1] - a[1]
  const len = Math.hypot(dx, dz) || 1
  return [dz / len, -dx / len]
}

/**
 * One side of a road, offset by `by` with a proper mitre at each corner, so a
 * right angle stays a right angle instead of pinching inwards.
 */
export function offsetSide(points: Pt[], by: number, closed: boolean): Pt[] {
  const pts = closed && points.length > 1 ? points.slice(0, -1) : points
  const n = pts.length
  if (n < 2) return pts.map((p) => [p[0], p[1]] as Pt)
  const out: Pt[] = []
  for (let i = 0; i < n; i++) {
    const hasPrev = closed || i > 0
    const hasNext = closed || i < n - 1
    const prev = pts[(i - 1 + n) % n]
    const next = pts[(i + 1) % n]
    const here = pts[i]
    const nIn = hasPrev ? norm(prev, here) : null
    const nOut = hasNext ? norm(here, next) : null
    if (!nIn || !nOut) {
      const nn = (nIn ?? nOut) as Pt
      out.push([here[0] + nn[0] * by, here[1] + nn[1] * by])
      continue
    }
    const a: Pt = [here[0] + nIn[0] * by, here[1] + nIn[1] * by]
    const b: Pt = [here[0] + nOut[0] * by, here[1] + nOut[1] * by]
    const da: Pt = [here[0] - prev[0], here[1] - prev[1]]
    const db: Pt = [next[0] - here[0], next[1] - here[1]]
    const hit = lineCross(a, da, b, db)
    out.push(hit ?? a)
  }
  if (closed) out.push([out[0][0], out[0][1]])
  return out
}

/** The carriageway as one closed polygon: one side out, the other back. */
export function roadOutline(road: CityRoad): Pt[] {
  const half = road.width / 2
  const left = offsetSide(road.points as Pt[], half, road.closed)
  const right = offsetSide(road.points as Pt[], -half, road.closed).reverse()
  return [...left, ...right]
}

function tracePolygon(ctx: GroundContext, poly: Pt[]): void {
  ctx.moveTo(poly[0][0], poly[0][1])
  for (let i = 1; i < poly.length; i++) ctx.lineTo(poly[i][0], poly[i][1])
  ctx.closePath()
}

/**
 * Every road, kerbs first. One path per road — carriageway plus, for a stub,
 * its turning circle — stroked for the kerb and then filled for the surface,
 * so the kerb shows as a band along the edge for the price of one path.
 */
export function drawRoads(ctx: GroundContext, plan: CityPlan): void {
  const order = [...plan.roads].sort((a, b) => a.tier - b.tier)
  for (const road of order) {
    const tier = ROADS[Math.min(Math.max(road.tier, 0), ROADS.length - 1)]
    ctx.beginPath()
    tracePolygon(ctx, roadOutline(road))
    if (road.turningCircle) {
      const end = road.points[road.points.length - 1]
      ctx.moveTo(end[0] + road.turningCircle, end[1])
      ctx.arc(end[0], end[1], road.turningCircle, 0, Math.PI * 2)
    }
    ctx.lineWidth = road.tier === 2 ? 1.1 : 0.7
    ctx.strokeStyle = tier.kerb
    ctx.stroke()
    ctx.fillStyle = road.kind === 'path' ? PATH_SURFACE : tier.surface
    ctx.fill()
  }

  // Centre dashes down the avenues, the one marking worth a pass of its own.
  ctx.save()
  ctx.setLineDash([3.5, 3.5])
  ctx.lineWidth = 0.5
  for (const road of plan.roads) {
    const tier = ROADS[road.tier]
    if (!tier.mark) continue
    ctx.strokeStyle = tier.mark
    ctx.beginPath()
    ctx.moveTo(road.points[0][0], road.points[0][1])
    for (let i = 1; i < road.points.length; i++) ctx.lineTo(road.points[i][0], road.points[i][1])
    ctx.stroke()
  }
  ctx.restore()
}

/** The blocks, tinted by the district each one belongs to. */
export function drawBlocks(ctx: GroundContext, plan: CityPlan): void {
  for (const block of plan.blocks) {
    ctx.fillStyle = GROUND.earth
    ctx.fillRect(block.x - block.w / 2, block.z - block.d / 2, block.w, block.d)
  }
  // Faint enough to be a hint of neighbourhood rather than a zoning overlay.
  ctx.save()
  ctx.globalAlpha = 0.09
  for (const block of plan.blocks) {
    let best = plan.districts[0]
    let bestD = Infinity
    for (const d of plan.districts) {
      const dist = Math.hypot(block.x - d.x, block.z - d.z)
      if (dist < bestD) {
        bestD = dist
        best = d
      }
    }
    ctx.fillStyle = accent(best.accent)
    ctx.fillRect(block.x - block.w / 2, block.z - block.d / 2, block.w, block.d)
  }
  ctx.restore()
}

/** The plaza's paving and the park's grass, under everything else. */
export function drawLandmarkGround(ctx: GroundContext, plan: CityPlan): void {
  for (const l of plan.landmarks) {
    if (l.kind === 'plaza') {
      const r = l.r ?? 12
      ctx.fillStyle = GROUND.plaza
      ctx.fillRect(l.x - r, l.z - r, r * 2, r * 2)
    } else if (l.kind === 'park') {
      const r = (l.r ?? 8) + 4
      ctx.fillStyle = GROUND.park
      ctx.beginPath()
      ctx.arc(l.x, l.z, r, 0, Math.PI * 2)
      ctx.fill()
    }
  }
}

/** One pad per lot: paving where something stands, dirt where nothing does. */
export function drawLots(ctx: GroundContext, plan: CityPlan, opts: GroundOptions = {}): void {
  const empty = opts.empty
  for (const lot of plan.lots) {
    ctx.fillStyle = empty?.has(lot.id) ? GROUND.dirt : GROUND.lot
    ctx.beginPath()
    ctx.rect(lot.x - lot.w / 2, lot.z - lot.d / 2, lot.w, lot.d)
    ctx.fill()
  }
  ctx.save()
  ctx.globalAlpha = 0.5
  ctx.lineWidth = 0.22
  ctx.strokeStyle = GROUND.lotLine
  for (const lot of plan.lots) {
    ctx.beginPath()
    ctx.rect(lot.x - lot.w / 2, lot.z - lot.d / 2, lot.w, lot.d)
    ctx.stroke()
  }
  ctx.restore()
}

/**
 * Contact darkening under every building. The research calls this the single
 * biggest reason a box reads as mass rather than as a decal, and it costs
 * three rectangles per lot here instead of a shadow-map sample per pixel.
 */
export function drawShadows(ctx: GroundContext, lots: CityLot[], built: ReadonlySet<number>): void {
  ctx.save()
  ctx.fillStyle = GROUND.shadow
  for (const lot of lots) {
    if (!built.has(lot.id)) continue
    const w = Math.min(lot.w, 10.6)
    const d = Math.min(lot.d, 10.6)
    for (const [spread, alpha] of [
      [2.6, 0.16],
      [1.4, 0.2],
      [0.4, 0.26],
    ] as const) {
      ctx.globalAlpha = alpha
      ctx.fillRect(
        lot.x + SHADOW_OFFSET[0] - w / 2 - spread / 2,
        lot.z + SHADOW_OFFSET[1] - d / 2 - spread / 2,
        w + spread,
        d + spread
      )
    }
  }
  ctx.restore()
}

/**
 * The whole ground in one pass, in world units: the caller's context is moved
 * so that the plan's origin is the middle of the image and one unit is
 * `pixelsPerUnit` texels.
 */
export function drawGround(ctx: GroundContext, plan: CityPlan, opts: GroundOptions = {}): void {
  const ppu = opts.pixelsPerUnit ?? PIXELS_PER_UNIT
  const { width, height } = groundPixels(plan, ppu)
  ctx.save()
  ctx.fillStyle = GROUND.grass
  ctx.fillRect(0, 0, width, height)
  ctx.translate(width / 2, height / 2)
  ctx.scale(ppu, ppu)
  ctx.lineJoin = 'round'
  ctx.lineCap = 'round'
  drawBlocks(ctx, plan)
  drawLandmarkGround(ctx, plan)
  drawRoads(ctx, plan)
  drawLots(ctx, plan, opts)
  if (opts.built) drawShadows(ctx, plan.lots, opts.built)
  ctx.restore()
}
