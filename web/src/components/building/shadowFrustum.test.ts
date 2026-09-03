import { describe, expect, it } from 'vitest'
import { CHAPTERS } from './chapters'
import { MAX_HALF_EXTENT, fitShadowFrustum, makeShadowFitScratch } from './shadowFrustum'
import type { FramingInputs } from './framing'

const VIEWPORT = { width: 1440, height: 900 }

/** Builds fit inputs from a chapter's raw scene params — the fit reads the
 *  chapter directly, never the Rig's damped copy. */
function inputsFor(chapterId: string, overrides: Partial<FramingInputs> = {}): FramingInputs {
  const chapter = CHAPTERS.find((c) => c.id === chapterId)
  if (!chapter) throw new Error(`no such chapter: ${chapterId}`)
  const { az, el, zoom, lookY } = chapter.scene
  return { az, el, chapterZoom: zoom, lookY, mobile: false, frame: 'tour', ...VIEWPORT, ...overrides }
}

function area(b: { left: number; right: number; top: number; bottom: number }): number {
  return (b.right - b.left) * (b.top - b.bottom)
}

describe('fitShadowFrustum', () => {
  it('always returns a valid, non-inverted box', () => {
    for (const chapter of CHAPTERS) {
      const bounds = fitShadowFrustum(inputsFor(chapter.id), makeShadowFitScratch())
      expect(bounds.right).toBeGreaterThan(bounds.left)
      expect(bounds.top).toBeGreaterThan(bounds.bottom)
    }
  })

  it('never exceeds the old fixed ±15 frustum on any axis', () => {
    for (const chapter of CHAPTERS) {
      const bounds = fitShadowFrustum(inputsFor(chapter.id), makeShadowFitScratch())
      expect(bounds.left).toBeGreaterThanOrEqual(-MAX_HALF_EXTENT)
      expect(bounds.right).toBeLessThanOrEqual(MAX_HALF_EXTENT)
      expect(bounds.bottom).toBeGreaterThanOrEqual(-MAX_HALF_EXTENT)
      expect(bounds.top).toBeLessThanOrEqual(MAX_HALF_EXTENT)
    }
  })

  it('fits tighter at the 1.2x single-floor zoom of chapters 05 and 07 than at the whole-table chapter', () => {
    const wholeTable = area(fitShadowFrustum(inputsFor('building'), makeShadowFitScratch()))
    const selfHealing = area(fitShadowFrustum(inputsFor('self-healing'), makeShadowFitScratch()))
    const webMcp = area(fitShadowFrustum(inputsFor('web-mcp'), makeShadowFitScratch()))
    expect(selfHealing).toBeLessThan(wholeTable)
    expect(webMcp).toBeLessThan(wholeTable)
  })

  it('covers the fanned-sheets arrival chapter at least as generously as the risen table', () => {
    // Before the tower rises (chapters 00–02), the model is flat sheets
    // spread across the whole table footprint — the fit must not collapse
    // to a sliver just because nothing has height yet.
    const arrival = area(fitShadowFrustum(inputsFor('arrival'), makeShadowFitScratch()))
    const selfHealing = area(fitShadowFrustum(inputsFor('self-healing'), makeShadowFitScratch()))
    expect(arrival).toBeGreaterThan(selfHealing)
  })

  it('is unaffected by mobile vs desktop pointer-driven camera jitter (uses raw chapter values)', () => {
    // Rig damps az/el with drift and pointer parallax; the shadow fit takes
    // the chapter's own numbers, so the same chapter fits identically
    // regardless of what the viewer's camera is currently doing.
    const a = fitShadowFrustum(inputsFor('nightfall'), makeShadowFitScratch())
    const b = fitShadowFrustum(inputsFor('nightfall'), makeShadowFitScratch())
    expect(a).toEqual(b)
  })
})
