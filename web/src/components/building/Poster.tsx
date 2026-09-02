// Static isometric blueprint of the building, as inline SVG. This is what the
// prerender ships and what a visitor sees before the WebGL chunk arrives (or
// instead of it, when WebGL is unavailable). The same model data draws it, so
// it is a faithful line drawing of the scene rather than a placeholder.
import { CORE, FLOORS, FLOOR_D, FLOOR_H, FLOOR_W } from './model'

const COS30 = Math.cos(Math.PI / 6)
const SIN30 = 0.5
const S = 26 // px per world unit

function iso(x: number, y: number, z: number): [number, number] {
  return [(x - z) * COS30 * S, (x + z) * SIN30 * S - y * S]
}

const pt = (p: [number, number]) => `${p[0].toFixed(1)},${p[1].toFixed(1)}`

function rect(x: number, z: number, w: number, d: number, y: number): string {
  const a = iso(x - w / 2, y, z - d / 2)
  const b = iso(x + w / 2, y, z - d / 2)
  const c = iso(x + w / 2, y, z + d / 2)
  const e = iso(x - w / 2, y, z + d / 2)
  return `M${pt(a)}L${pt(b)}L${pt(c)}L${pt(e)}Z`
}

function box(x: number, z: number, w: number, d: number, y0: number, h: number): string {
  // Only the three edges of a box that face the camera, plus its top.
  const top = rect(x, z, w, d, y0 + h)
  const fr = iso(x + w / 2, y0, z + d / 2)
  const frT = iso(x + w / 2, y0 + h, z + d / 2)
  const r = iso(x + w / 2, y0, z - d / 2)
  const rT = iso(x + w / 2, y0 + h, z - d / 2)
  const l = iso(x - w / 2, y0, z + d / 2)
  const lT = iso(x - w / 2, y0 + h, z + d / 2)
  return `${top}M${pt(fr)}L${pt(frT)}M${pt(r)}L${pt(rT)}M${pt(l)}L${pt(lT)}M${pt(l)}L${pt(fr)}L${pt(r)}`
}

function build(): { floors: string; rooms: string; core: string; table: string; grid: string } {
  let floors = ''
  let rooms = ''
  for (let i = 0; i < FLOORS.length; i++) {
    const y = i * FLOOR_H
    floors += rect(0, 0, FLOOR_W, FLOOR_D, y)
    for (const r of FLOORS[i].rooms) {
      const blocks = r.blocks ?? [{ x: r.x, z: r.z, w: r.w, d: r.d }]
      for (const b of blocks) rooms += box(b.x, b.z, b.w, b.d, y + 0.18 + (r.base ?? 0), r.h * 1.25)
    }
  }
  const topY = FLOORS.length * FLOOR_H
  floors += rect(0, 0, FLOOR_W, FLOOR_D, topY)
  // Front corner verticals of the shell.
  const corners: [number, number][] = [
    [FLOOR_W / 2, FLOOR_D / 2],
    [FLOOR_W / 2, -FLOOR_D / 2],
    [-FLOOR_W / 2, FLOOR_D / 2],
    [-FLOOR_W / 2, -FLOOR_D / 2],
  ]
  for (const [cx, cz] of corners) floors += `M${pt(iso(cx, 0, cz))}L${pt(iso(cx, topY, cz))}`
  const core = box(CORE.x, CORE.z, CORE.w, CORE.d, 0, topY + 0.6)
  const table = rect(0, 0, 17.5, 14.5, -0.02)
  let grid = ''
  for (let gx = -8; gx <= 8; gx += 2) grid += `M${pt(iso(gx, 0, -7))}L${pt(iso(gx, 0, 7))}`
  for (let gz = -6; gz <= 6; gz += 2) grid += `M${pt(iso(-8.5, 0, gz))}L${pt(iso(8.5, 0, gz))}`
  return { floors, rooms, core, table, grid }
}

const D = build()

export default function Poster({ hidden }: { hidden: boolean }) {
  return (
    <svg
      className="bld-poster"
      data-component="BuildingPoster"
      data-hidden={hidden ? 'true' : 'false'}
      viewBox="-420 -400 840 690"
      preserveAspectRatio="xMidYMid meet"
      aria-hidden="true"
      focusable="false"
    >
      <defs>
        <linearGradient id="bld-poster-table" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#1c4a94" />
          <stop offset="1" stopColor="#12336c" />
        </linearGradient>
        <clipPath id="bld-poster-clip">
          <path d={D.table} />
        </clipPath>
      </defs>
      <g transform="translate(0,60)">
        <path d={D.table} fill="url(#bld-poster-table)" />
        <path d={D.grid} stroke="#3e6fc4" strokeWidth="0.6" fill="none" clipPath="url(#bld-poster-clip)" />
        <path d={D.table} stroke="#8fb0ea" strokeWidth="1.2" fill="none" />
        <path d={D.rooms} stroke="#ffffff" strokeOpacity="0.55" strokeWidth="0.8" fill="none" strokeLinejoin="round" />
        <path d={D.floors} stroke="#ffffff" strokeOpacity="0.9" strokeWidth="1.1" fill="none" strokeLinejoin="round" />
        <path d={D.core} stroke="#9fd1ff" strokeOpacity="0.9" strokeWidth="1" fill="none" />
      </g>
    </svg>
  )
}
