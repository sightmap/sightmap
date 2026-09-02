import { useFrame } from '@react-three/fiber'
import { useMemo, useRef } from 'react'
import * as THREE from 'three'
import { useShared } from './state'

// Daylight from the front-right, softening to moonlight at nightfall. One
// shadow-casting directional light covers the whole table.
export default function Lights() {
  const s = useShared()
  const dir = useRef<THREE.DirectionalLight>(null)
  const amb = useRef<THREE.AmbientLight>(null)
  const hemi = useRef<THREE.HemisphereLight>(null)
  const col = useMemo(
    () => ({
      day: new THREE.Color('#fff2dc'),
      night: new THREE.Color('#7d95e6'),
      ambDay: new THREE.Color('#ffffff'),
      ambNight: new THREE.Color('#5468b3'),
    }),
    []
  )
  useFrame(() => {
    const n = s.cur.night
    if (dir.current) {
      dir.current.intensity = THREE.MathUtils.lerp(1.85, 0.45, n)
      dir.current.color.copy(col.day).lerp(col.night, n)
    }
    if (amb.current) {
      amb.current.intensity = THREE.MathUtils.lerp(0.72, 0.32, n)
      amb.current.color.copy(col.ambDay).lerp(col.ambNight, n)
    }
    if (hemi.current) hemi.current.intensity = THREE.MathUtils.lerp(0.7, 0.28, n)
  })
  const mapSize = s.mobile ? 1024 : 2048
  return (
    <>
      <ambientLight ref={amb} intensity={0.72} />
      <hemisphereLight ref={hemi} args={['#ffffff', '#5a7ac9', 0.7]} />
      <directionalLight
        ref={dir}
        position={[13, 15, -7]}
        intensity={1.85}
        castShadow
        shadow-mapSize={[mapSize, mapSize]}
        shadow-bias={-0.0004}
        shadow-normalBias={0.03}
      >
        <orthographicCamera attach="shadow-camera" args={[-15, 15, 15, -15, 1, 50]} />
      </directionalLight>
    </>
  )
}
