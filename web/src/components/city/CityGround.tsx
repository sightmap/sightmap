// The ground: one plane, one canvas texture, drawn from the plan at mount.
// Roads, kerbs, markings, lot lines, district tint and the contact shadow
// under every building are all texels, which is why the whole city costs one
// draw call from the knees down.
import { useEffect, useMemo } from 'react'
import * as THREE from 'three'
import type { CityPlan } from '@/types/city'
import { PIXELS_PER_UNIT, drawGround, groundPixels } from './ground'
import { GROUND } from './palette'

export interface CityGroundProps {
  plan: CityPlan
  /** Lots carrying a building, so the drawing knows where to darken. */
  built: ReadonlySet<number>
  empty: ReadonlySet<number>
}

export default function CityGround({ plan, built, empty }: CityGroundProps) {
  const texture = useMemo(() => {
    const { width, height } = groundPixels(plan)
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const ctx = canvas.getContext('2d')
    if (!ctx) return null
    drawGround(ctx, plan, { built, empty, pixelsPerUnit: PIXELS_PER_UNIT })
    const tex = new THREE.CanvasTexture(canvas)
    tex.colorSpace = THREE.SRGBColorSpace
    tex.anisotropy = 8
    tex.needsUpdate = true
    return tex
  }, [plan, built, empty])

  useEffect(() => () => texture?.dispose(), [texture])

  return (
    <group>
      <mesh rotation={[-Math.PI / 2, 0, 0]} position={[plan.bounds.x, 0, plan.bounds.z]} receiveShadow>
        <planeGeometry args={[plan.bounds.w, plan.bounds.d]} />
        {texture ? (
          <meshStandardMaterial map={texture} roughness={1} />
        ) : (
          <meshStandardMaterial color={GROUND.earth} roughness={1} />
        )}
      </mesh>
      {/* Open country under the fog, so the plan does not end in mid-air. */}
      <mesh rotation={[-Math.PI / 2, 0, 0]} position={[0, -0.05, 0]}>
        <planeGeometry args={[plan.bounds.w * 6, plan.bounds.d * 6]} />
        <meshStandardMaterial color={GROUND.grass} roughness={1} />
      </mesh>
    </group>
  )
}
