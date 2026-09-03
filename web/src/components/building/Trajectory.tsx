import { useFrame } from '@react-three/fiber'
import { Line } from '@react-three/drei'
import Label from './Label'
import { useMemo, useRef, type ComponentRef } from 'react'
import { JOURNEYS, TRAVELLER_COLORS } from './model'
import { buildPath } from './geometry'
import { useShared } from './state'

// Chapter 06. The BookFlight journey as a named route: a ribbon through the
// building with a numbered pin at every stop.
export default function Trajectory() {
  const s = useShared()
  const journey = JOURNEYS[0]
  const path = useMemo(() => buildPath(journey, 0.16), [journey])
  const line = useRef<ComponentRef<typeof Line>>(null)
  const pins = useRef<(HTMLDivElement | null)[]>([])
  const color = TRAVELLER_COLORS[journey.who]

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
          <Label key={name + k} position={[p.x, p.y + 0.15, p.z]} center zIndexRange={[7, 0]}>
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
          </Label>
        )
      })}
    </>
  )
}
