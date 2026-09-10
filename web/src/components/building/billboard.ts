import { CHAPTERS, smoothstep, type SceneParams } from './chapters'

export const BILLBOARD_CYCLE = 44

export function advanceBillboardTime(prev: number, dt: number, reduced: boolean): number {
  return reduced ? prev : prev + Math.min(dt, 0.1)
}

export function billboardStep(t: number, c: SceneParams): boolean {
  Object.assign(c, CHAPTERS[4].scene) // "The people": built, populated
  // Floor directory labels collide with the enter chip on this tight crop.
  c.labels = 0
  c.agents = 1
  c.az = 42 + Math.sin(t * 0.085) * 9
  c.el = 25 + Math.sin(t * 0.05) * 1.5
  c.zoom = 1
  c.lookY = 5.4
  const phase = (Math.sin((t * 2 * Math.PI) / BILLBOARD_CYCLE - Math.PI / 2) + 1) / 2
  c.night = smoothstep((phase - 0.4) / 0.25)
  return c.night > 0.5
}
