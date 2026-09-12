import { useFrame, useThree } from '@react-three/fiber'
import { Html } from '@react-three/drei'
import { useMemo, useRef } from 'react'
import * as THREE from 'three'
import { FLOOR_D, FLOOR_W, SLAB_T, floorY, roomTop, type BuildingModel } from './model'
import { useBuildingModel } from './context'

// The room names, which on a derived building are the tool names the scan
// found. Same pattern as the wayfinding tags on the /building page: plain
// HTML anchored in 3D, so it uses the site's fonts.
//
// A floor of ten tools has ten labels that would sit on top of each other, so
// a label is only drawn when there is room for it on screen: the nearer the
// camera, the more of them fit and the more names appear.

/** Long tool names are truncated rather than wrapped: a label is a tag. */
export function truncate(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max)}…` : text
}

export const ROOM_NAME_MAX = 22
export const FLOOR_NAME_MAX = 40

interface Anchor {
  pos: THREE.Vector3
  text: string
  /** `sign` reserves the board's space on screen and draws nothing itself. */
  kind: 'room' | 'floor' | 'sign'
  /** Half-width and half-height of the label on screen, in px. */
  hw: number
  hh: number
}

function anchorsFor(model: BuildingModel): Anchor[] {
  const out: Anchor[] = []
  const sign = model.facade?.sign
  if (sign) {
    // The sign and its plate own their patch of sky; a tool name over them
    // would read as part of the name on the board.
    out.push({
      kind: 'sign',
      text: '',
      pos: new THREE.Vector3(0, floorY(model.floors.length) + SLAB_T + 1.0, FLOOR_D / 2 + 0.45),
      hw: Math.max(sign.length * 3.6, 70),
      hh: 34,
    })
  }
  model.floors.forEach((floor, i) => {
    const name = truncate(floor.name, FLOOR_NAME_MAX)
    out.push({
      kind: 'floor',
      text: name,
      // Hung off the back-right corner, where the floor directory hangs on
      // the /building page: screen-right is (+X, -Z), so a long page title
      // runs out into the sky rather than across the building or the sign.
      pos: new THREE.Vector3(FLOOR_W / 2 + 0.4, floorY(i) + SLAB_T + 0.5, -FLOOR_D / 2 - 0.4),
      hw: name.length * 3.2 + 12,
      hh: 13,
    })
    for (const room of floor.rooms) {
      const text = truncate(room.name, ROOM_NAME_MAX)
      out.push({
        kind: 'room',
        text,
        pos: new THREE.Vector3(room.x, floorY(i) + SLAB_T + roomTop(room) + 0.18, room.z),
        hw: text.length * 2.9 + 9,
        hh: 12,
      })
    }
  })
  return out
}

export default function RoomLabels() {
  const model = useBuildingModel()
  const { camera, size } = useThree()
  const anchors = useMemo(() => anchorsFor(model), [model])
  const els = useRef<(HTMLDivElement | null)[]>([])
  const v = useMemo(() => new THREE.Vector3(), [])
  const frame = useRef(0)
  const shown = useRef<{ x: number; y: number; hw: number; hh: number }[]>([])

  useFrame(() => {
    // Nothing here moves on its own; a third of the frames is plenty to keep
    // up with a drag.
    if (frame.current++ % 3 !== 0) return
    const placed = shown.current
    placed.length = 0
    const screen = anchors.map((a, k) => {
      v.copy(a.pos).project(camera)
      return { k, a, x: ((v.x + 1) / 2) * size.width, y: ((1 - v.y) / 2) * size.height, z: v.z }
    })
    // The floor directory and the sign are always drawn and claim their space
    // first; the tool names take what is left, nearest first, so moving the
    // camera closer is what makes more of them appear.
    for (const p of screen) {
      if (p.a.kind === 'room') continue
      placed.push({ x: p.x, y: p.y, hw: p.a.hw, hh: p.a.hh })
      const el = els.current[p.k]
      if (el) {
        el.style.opacity = '1'
        el.style.visibility = 'visible'
      }
    }
    for (const p of screen.filter((q) => q.a.kind === 'room').sort((a, b) => a.z - b.z)) {
      const el = els.current[p.k]
      if (!el) continue
      const clear = !placed.some(
        (o) => Math.abs(o.x - p.x) < o.hw + p.a.hw && Math.abs(o.y - p.y) < o.hh + p.a.hh
      )
      if (clear) placed.push({ x: p.x, y: p.y, hw: p.a.hw, hh: p.a.hh })
      el.style.opacity = clear ? '1' : '0'
      el.style.visibility = clear ? 'visible' : 'hidden'
    }
  })

  return (
    <>
      {anchors.map((a, k) =>
        a.kind === 'sign' ? null : (
        <Html
          key={k}
          position={a.pos}
          center={a.kind === 'room'}
          zIndexRange={[5, 0]}
          style={{ pointerEvents: 'none' }}
          wrapperClass="bld-room-anchor"
        >
          <div
            ref={(el) => {
              els.current[k] = el
            }}
            className={`bld-room bld-room--${a.kind}`}
            style={{ opacity: 0, visibility: 'hidden' }}
            aria-hidden="true"
          >
            {a.text}
          </div>
        </Html>
        )
      )}
    </>
  )
}
