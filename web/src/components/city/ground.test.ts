import { describe, expect, it } from 'vitest'
import { planCity } from '../../../scripts/lib/city'
import { drawGround, drawLots, drawRoads, groundPixels, offsetSide, roadOutline } from './ground'
import type { GroundContext } from './ground'

/** Records what was asked of a 2D context, so the drawing can be counted. */
function stub() {
  const ops: string[] = []
  const ctx: GroundContext & { ops: string[] } = {
    ops,
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    lineJoin: 'miter',
    lineCap: 'butt',
    globalAlpha: 1,
    save: () => void ops.push('save'),
    restore: () => void ops.push('restore'),
    translate: () => void ops.push('translate'),
    scale: () => void ops.push('scale'),
    beginPath: () => void ops.push('beginPath'),
    moveTo: () => void ops.push('moveTo'),
    lineTo: () => void ops.push('lineTo'),
    closePath: () => void ops.push('closePath'),
    arc: () => void ops.push('arc'),
    rect: () => void ops.push('rect'),
    fillRect: () => void ops.push('fillRect'),
    fill: () => void ops.push('fill'),
    stroke: () => void ops.push('stroke'),
    setLineDash: () => void ops.push('setLineDash'),
  }
  return ctx
}

const count = (ctx: { ops: string[] }, op: string) => ctx.ops.filter((o) => o === op).length

const plan = planCity()

describe('drawRoads', () => {
  it('fills every road exactly once and strokes its kerb', () => {
    const ctx = stub()
    drawRoads(ctx, plan)
    expect(plan.roads.length).toBeGreaterThan(10)
    expect(count(ctx, 'fill')).toBe(plan.roads.length)
    // One kerb per road, plus the dashed centre line on each avenue.
    const avenues = plan.roads.filter((r) => r.tier === 2).length
    expect(count(ctx, 'stroke')).toBe(plan.roads.length + avenues)
    expect(count(ctx, 'setLineDash')).toBe(1)
  })

  it('gives a cul-de-sac its turning circle', () => {
    const ctx = stub()
    drawRoads(ctx, plan)
    expect(count(ctx, 'arc')).toBe(plan.roads.filter((r) => r.turningCircle).length)
  })
})

describe('drawLots', () => {
  it('fills every lot exactly once and outlines it', () => {
    const ctx = stub()
    drawLots(ctx, plan)
    expect(plan.lots.length).toBeGreaterThan(200)
    expect(count(ctx, 'fill')).toBe(plan.lots.length)
    expect(count(ctx, 'stroke')).toBe(plan.lots.length)
  })
})

describe('drawGround', () => {
  it('draws the whole plan against a stub: a fill per road and one per lot', () => {
    const ctx = stub()
    drawGround(ctx, plan, { built: new Set([plan.lots[0].id]), empty: new Set([plan.lots[1].id]) })
    expect(count(ctx, 'fill')).toBeGreaterThanOrEqual(plan.roads.length + plan.lots.length)
    // The ground is drawn in world units, so the context is moved once.
    expect(count(ctx, 'translate')).toBe(1)
    expect(count(ctx, 'scale')).toBe(1)
    expect(ctx.ops[ctx.ops.length - 1]).toBe('restore')
  })

  it('sizes the image from the plan', () => {
    const { width, height } = groundPixels(plan)
    expect(width).toBeGreaterThan(height)
    expect(width / plan.bounds.w).toBeCloseTo(height / plan.bounds.d, 1)
  })
})

describe('road outlines', () => {
  it('offsets a straight road to a band of its own width', () => {
    const side = offsetSide(
      [
        [0, 0],
        [10, 0],
      ],
      2,
      false
    )
    expect(side).toEqual([
      [0, -2],
      [10, -2],
    ])
  })

  it('mitres a right angle instead of pinching it', () => {
    const side = offsetSide(
      [
        [0, 0],
        [10, 0],
        [10, 10],
      ],
      2,
      false
    )
    expect(side[1]).toEqual([12, -2])
  })

  it('closes the ring, so the carriageway has no seam', () => {
    const ring = plan.roads.find((r) => r.id === 'ring')
    expect(ring).toBeDefined()
    const outline = roadOutline(ring!)
    expect(outline.length).toBeGreaterThan(8)
    // Two sides of a closed road, each returning to where it started.
    expect(outline[0]).toEqual(outline[(outline.length / 2) - 1])
  })
})
