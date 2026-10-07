import { useFrame } from '@react-three/fiber'
import { Html, Line } from '@react-three/drei'
import { useMemo, useRef, type ComponentRef } from 'react'
import { TRAVELLER_COLORS, type Journey } from './model'
import { buildPath } from './geometry'
import { useBuildingModel } from './context'
import { useShared } from './state'

// Chapter 06. The BookFlight journey as a named route: a ribbon through the
// building with a numbered pin at every stop.
export default function Trajectory() {
  const s = useShared()
  const model = useBuildingModel()
  // A building whose scan found no journeys has no route to highlight.
  const journey: Journey | undefined = model.journeys[0]
  const path = useMemo(() => (journey ? buildPath(model, journey, 0.16) : null), [model, journey])
  const line = useRef<ComponentRef<typeof Line>>(null)
  const pins = useRef<(HTMLDivElement | null)[]>([])

  useFrame(() => {
    const o = s.cur.trajectory
    if (line.current) {
      const m = line.current.material as unknown as { opacity: number }
      m.opacity = o * 0.95
      line.current.visible = o > 0.02
    }
    pins.current.forEach((el, k) => {
      if (!el) return
      const po = Math.min(1, Math.max(0, o * 1.6 - k * 0.07))
      el.style.opacity = po.toFixed(2)
      el.style.visibility = po > 0.02 ? 'visible' : 'hidden'
      el.style.transform = `scale(${(0.6 + 0.4 * po).toFixed(3)})`
    })
  })

  if (!journey || !path) return null
  const color = TRAVELLER_COLORS[journey.who]

  return (
    <>
      <Line
        ref={line}
        points={path.points}
        color={color}
        lineWidth={3}
        transparent
        opacity={0}
        depthWrite={false}
      />
      {journey.stops.map(([, name], k) => {
        const p = path.points[path.stops[k]]
        return (
          <Html key={name + k} position={[p.x, p.y + 0.15, p.z]} center zIndexRange={[7, 0]} style={{ pointerEvents: 'none' }}>
            <div
              ref={(el) => {
                pins.current[k] = el
              }}
              className="bld-pin"
              style={{ opacity: 0, visibility: 'hidden' }}
            >
              <b>{k + 1}</b>
              <span>{name}</span>
            </div>
          </Html>
        )
      })}
    </>
  )
}
