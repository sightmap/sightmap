// The homepage slice of the building: the same model as /building, driven by
// a clock instead of scroll. The camera orbits gently and the light runs a
// slow day-to-night cycle so the lit windows get their moment. Loaded lazily
// by BuildingBillboard once the frame nears the viewport; the parent pauses
// the frame loop while it is off-screen.
import { Canvas, useFrame } from '@react-three/fiber'
import { useRef } from 'react'
import { SharedStateContext, useShared, type SharedState } from './state'
import { CANVAS_PROPS, Ready, Rig, SceneContent } from './Scene'
import { advanceBillboardTime, billboardStep } from './billboard'

function Driver({ onNight }: { onNight: (night: boolean) => void }) {
  const s = useShared()
  const wasNight = useRef(false)
  const tRef = useRef(0)
  useFrame((_, dt) => {
    tRef.current = advanceBillboardTime(tRef.current, dt, s.reduced)
    const night = billboardStep(tRef.current, s.cur)
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
