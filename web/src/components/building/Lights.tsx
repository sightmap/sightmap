import { useFrame } from '@react-three/fiber'
import { useMemo, useRef } from 'react'
import * as THREE from 'three'
import { useShared } from './state'

// Daylight from the front-right, softening to moonlight at nightfall. One
// shadow-casting directional light covers the whole table. A warm unshadowed
// point light comes up at night so the interiors stay readable.
export default function Lights() {
  const s = useShared()
  const dir = useRef<THREE.DirectionalLight>(null)
  const amb = useRef<THREE.AmbientLight>(null)
  const hemi = useRef<THREE.HemisphereLight>(null)
  const fill = useRef<THREE.PointLight>(null)
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
      dir.current.intensity = THREE.MathUtils.lerp(1.7, 0.45, n)
      dir.current.color.copy(col.day).lerp(col.night, n)
    }
    if (amb.current) {
      amb.current.intensity = THREE.MathUtils.lerp(0.78, 0.36, n)
      amb.current.color.copy(col.ambDay).lerp(col.ambNight, n)
    }
    if (hemi.current) hemi.current.intensity = THREE.MathUtils.lerp(0.7, 0.32, n)
    if (fill.current) fill.current.intensity = s.mobile ? 0 : 35 * n
  })
  const mapSize = s.mobile ? 1024 : 2048
  return (
    <>
      <ambientLight ref={amb} intensity={0.78} />
      <hemisphereLight ref={hemi} args={['#ffffff', '#5a7ac9', 0.7]} />
      <directionalLight
        ref={dir}
        position={[13, 15, -7]}
        intensity={1.7}
        castShadow
        shadow-intensity={0.7}
        shadow-mapSize={[mapSize, mapSize]}
        shadow-bias={-0.0004}
        shadow-normalBias={0.03}
      >
        <orthographicCamera attach="shadow-camera" args={[-15, 15, 15, -15, 1, 50]} />
      </directionalLight>
      <pointLight ref={fill} position={[1.2, 4.5, 0.4]} color="#ffd7a0" intensity={0} distance={8} decay={2} />
    </>
  )
}
