// The homepage slice of the building: the same model as /building, driven by
// a clock instead of scroll. The camera orbits gently and the light runs a
// slow day-to-night cycle so the lit windows get their moment. Loaded lazily
// by BuildingBillboard once the frame nears the viewport; the parent pauses
// the frame loop while it is off-screen.
import { Canvas, useFrame } from '@react-three/fiber'
import { useRef } from 'react'
import { CHAPTERS, smoothstep } from './chapters'
import { SharedStateContext, useShared, type SharedState } from './state'
import { CANVAS_PROPS, Ready, Rig, SceneContent } from './Scene'

const CYCLE = 44 // seconds per day

function Driver({ onNight }: { onNight: (night: boolean) => void }) {
  const s = useShared()
  const wasNight = useRef(false)
  useFrame(({ clock }) => {
    const t = s.reduced ? 0 : clock.getElapsedTime()
    const c = s.cur
    Object.assign(c, CHAPTERS[4].scene) // "The people": built, populated
    // Floor directory labels collide with the enter chip on this tight crop.
    c.labels = 0
    c.agents = 1
    c.az = 42 + Math.sin(t * 0.085) * 9
    c.el = 25 + Math.sin(t * 0.05) * 1.5
    c.zoom = 1
    c.lookY = 5.4
    const phase = (Math.sin((t * 2 * Math.PI) / CYCLE - Math.PI / 2) + 1) / 2
    c.night = smoothstep((phase - 0.4) / 0.25)
    const night = c.night > 0.5
    if (night !== wasNight.current) {
      wasNight.current = night
      onNight(night)
    }
  }, -100)
  return null
}

export interface BillboardSceneProps {
  shared: SharedState
  active: boolean
  onReady: () => void
  onNight: (night: boolean) => void
}

export default function BillboardScene({ shared, active, onReady, onNight }: BillboardSceneProps) {
  return (
    <Canvas
      {...CANVAS_PROPS}
      dpr={[1, 1.5]}
      frameloop={active ? 'always' : 'never'}
      onCreated={({ gl }) => gl.setClearColor(0x000000, 0)}
    >
      <SharedStateContext.Provider value={shared}>
        <Driver onNight={onNight} />
        <Ready onReady={onReady} />
        <Rig />
        <SceneContent />
      </SharedStateContext.Provider>
    </Canvas>
  )
}
