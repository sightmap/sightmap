// One sun, one hemisphere fill, and fog the exact colour of the sky. The
// building page's Lights are cut for a drafting table fifteen units across;
// this shadow camera has to cover four hundred, so the city gets its own.
//
// Warm sun against cool shade, and the fog matched to the sky so the plan
// dissolves at its edge instead of stopping in mid-air.
import { useEffect, useMemo, useRef } from 'react'
import { useThree } from '@react-three/fiber'
import * as THREE from 'three'
import type { CityPlan } from '@/types/city'
import { SKY, SUN } from './palette'

export interface CityLightsProps {
  plan: CityPlan
  night: boolean
}

export default function CityLights({ plan, night }: CityLightsProps) {
  const { scene } = useThree()
  const sun = useRef<THREE.DirectionalLight>(null)
  const sky = useMemo(() => ({ day: new THREE.Color(SKY.day), night: new THREE.Color(SKY.night) }), [])
  const fog = useMemo(() => new THREE.Fog(SKY.day, 420, 1500), [])

  useEffect(() => {
    const color = night ? sky.night : sky.day
    scene.background = color
    fog.color.copy(color)
    scene.fog = fog
    return () => {
      scene.fog = null
      scene.background = null
    }
  }, [scene, fog, night, sky])

  // The shadow camera covers the whole plan: the city is static, so one map
  // holds every contact shadow the ground texture does not already carry.
  const reach = Math.max(plan.bounds.w, plan.bounds.d) * 0.6

  useEffect(() => {
    if (sun.current) sun.current.shadow.camera.updateProjectionMatrix()
  }, [reach])

  return (
    <>
      {/* Warm sun against cool shade: the ambient stays low so a wall that
          faces away from the sun is visibly a different plane. */}
      <ambientLight intensity={night ? 0.42 : 0.55} color={night ? '#6f7fbf' : '#eef1f6'} />
      <hemisphereLight args={[night ? '#5a6bb0' : '#dfe7f0', night ? '#232a45' : '#bdb49c', night ? 0.38 : 0.62]} />
      <directionalLight
        ref={sun}
        position={[reach * 0.7, reach * 1.1, -reach * 0.55]}
        intensity={night ? 0.55 : 2.1}
        color={night ? SUN.night : SUN.day}
        castShadow
        shadow-mapSize={[2048, 2048]}
        shadow-bias={-0.0006}
        shadow-normalBias={0.6}
      >
        <orthographicCamera attach="shadow-camera" args={[-reach, reach, reach, -reach, 1, reach * 4]} />
      </directionalLight>
    </>
  )
}
