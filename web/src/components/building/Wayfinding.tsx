import { useFrame } from '@react-three/fiber'
import { Html } from '@react-three/drei'
import { useRef } from 'react'
import { FLOORS, FLOOR_D, FLOOR_W, SLAB_T, floorY, roomTop } from './model'
import { smoothstep } from './chapters'
import { useShared } from './state'

// The signage: a floor directory down the right-hand edge, tags over the
// rooms that carry one, and yellow memory notes. Plain HTML anchored in 3D,
// so it uses the site's fonts and stays crisp at any zoom.
interface Anchor {
  pos: [number, number, number]
  kind: 'floor' | 'tag' | 'memory'
  floor: number
  node: React.ReactNode
}

const anchors: Anchor[] = []
FLOORS.forEach((f, i) => {
  anchors.push({
    kind: 'floor',
    floor: i,
    pos: [FLOOR_W / 2 + 0.15, floorY(i) + 0.95, -FLOOR_D / 2 + 0.3],
    node: (
      <>
        <b>{String(i).padStart(2, '0')}</b> {f.name} <span>{f.route}</span>
      </>
    ),
  })
  for (const r of f.rooms) {
    if (r.tag) {
      anchors.push({
        kind: 'tag',
        floor: i,
        pos: [r.x, floorY(i) + SLAB_T + roomTop(r) + 0.15, r.z],
        node: r.name,
      })
    }
    if (r.memory) {
      anchors.push({
        kind: 'memory',
        floor: i,
        pos: [r.x - r.w / 2 - 0.4, floorY(i) + SLAB_T + roomTop(r) + 0.55, r.z + 1.0],
        node: (
          <>
            <em>memory</em>
            {r.memory}
          </>
        ),
      })
    }
  }
})

export default function Wayfinding() {
  const s = useShared()
  const els = useRef<(HTMLDivElement | null)[]>([])
  useFrame(() => {
    const c = s.cur
    const rise = smoothstep(Math.min(1, c.rise * 1.4 - 0.4))
    anchors.forEach((a, k) => {
      const el = els.current[k]
      if (!el) return
      const base = a.kind === 'floor' ? c.labels : c.tags
      // Stagger by floor so the directory lights up bottom to top.
      const o = smoothstep(base * 1.5 - a.floor * 0.08) * rise
      el.style.opacity = o.toFixed(3)
      el.style.visibility = o > 0.02 ? 'visible' : 'hidden'
      el.style.transform = `translateY(${((1 - o) * 10).toFixed(1)}px)`
    })
  })
  return (
    <>
      {anchors.map((a, k) => (
        <Html
          key={k}
          position={a.pos}
          center={a.kind === 'tag'}
          zIndexRange={[6, 0]}
          style={{ pointerEvents: 'none' }}
        >
          <div
            ref={(el) => {
              els.current[k] = el
            }}
            className={`bld-tag bld-tag--${a.kind}`}
            style={{ opacity: 0, visibility: 'hidden' }}
          >
            {a.node}
          </div>
        </Html>
      ))}
    </>
  )
}
