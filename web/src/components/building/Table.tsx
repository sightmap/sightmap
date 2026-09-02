import { useFrame } from '@react-three/fiber'
import { Html, Line, RoundedBox } from '@react-three/drei'
import { useMemo, useRef } from 'react'
import * as THREE from 'three'
import { TABLE } from './model'
import { useShared } from './state'

// The drafting table: a blueprint-blue slab with a faint grid, on which the
// sheets lie and the model stands.
export default function Table() {
  const s = useShared()
  const mat = useRef<THREE.MeshStandardMaterial>(null)
  const col = useMemo(() => ({ day: new THREE.Color('#12315f'), night: new THREE.Color('#091a3d') }), [])
  const grid = useMemo(() => {
    const pts: [number, number, number][] = []
    const hw = TABLE.w / 2 - 0.5
    const hd = TABLE.d / 2 - 0.5
    for (let x = -Math.floor(hw); x <= hw; x += 1) pts.push([x, 0.006, -hd], [x, 0.006, hd])
    for (let z = -Math.floor(hd); z <= hd; z += 1) pts.push([-hw, 0.006, z], [hw, 0.006, z])
    return pts
  }, [])
  useFrame(() => {
    if (mat.current) mat.current.color.copy(col.day).lerp(col.night, s.cur.night)
  })
  return (
    <group>
      <RoundedBox
        args={[TABLE.w, TABLE.t, TABLE.d]}
        radius={0.1}
        smoothness={3}
        position={[0, -TABLE.t / 2, 0]}
        receiveShadow
      >
        <meshStandardMaterial ref={mat} color="#12315f" roughness={0.96} />
      </RoundedBox>
      <Line points={grid} segments color="#2f62b8" lineWidth={1} transparent opacity={0.5} depthWrite={false} />
      <Html
        position={[-TABLE.w / 2 + 0.6, 0.02, TABLE.d / 2 - 0.6]}
        zIndexRange={[4, 0]}
        style={{ pointerEvents: 'none' }}
      >
        <div className="bld-titleblock">
          <span>Drawing set A-101</span>
          <span>northwind-air · .sightmap/ · v1</span>
        </div>
      </Html>
    </group>
  )
}
