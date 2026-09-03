import { useFrame, useThree } from '@react-three/fiber'
import { useMemo, useRef } from 'react'
import * as THREE from 'three'
import { fitShadowFrustum, makeShadowFitScratch, MAX_HALF_EXTENT } from './shadowFrustum'
import { useMobileTier, useShared } from './state'

// Daylight from the front-right, softening to moonlight at nightfall. One
// shadow-casting directional light covers the whole table.
//
// The ambient and hemisphere terms used to be fill — they existed to stop an
// unlit scene going black, because nothing else lit a surface the sun missed.
// The environment map does that job now, and does it directionally, so the
// fill is cut hard: contrast is the whole point of the change, and fill is
// exactly what flattens it. The directional light is untouched and remains
// the only shadow caster.
export default function Lights() {
  const s = useShared()
  const mobile = useMobileTier()
  const { size } = useThree()
  const dir = useRef<THREE.DirectionalLight>(null)
  const amb = useRef<THREE.AmbientLight>(null)
  const hemi = useRef<THREE.HemisphereLight>(null)
  const shadowCam = useRef<THREE.OrthographicCamera>(null)
  const shadowScratch = useMemo(makeShadowFitScratch, [])
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
      // Night keeps almost as much ambient as day even though the sun drops to
      // 0.45, because the environment fades out with it (see Environment.tsx):
      // something has to keep the unlit side of the model off pure black.
      amb.current.intensity = THREE.MathUtils.lerp(0.15, 0.14, n)
      amb.current.color.copy(col.ambDay).lerp(col.ambNight, n)
    }
    if (hemi.current) hemi.current.intensity = THREE.MathUtils.lerp(0.2, 0.12, n)

    // Fit the shadow camera to what's actually on screen this frame. Reads
    // the scroll-blended `s.cur` — not Rig's own damped az/el, which layer
    // idle drift and pointer parallax on top for visual flair a shadow
    // frustum should not chase (see shadowFrustum.ts).
    const cam = shadowCam.current
    if (cam) {
      const bounds = fitShadowFrustum(
        {
          az: s.cur.az,
          el: s.cur.el,
          chapterZoom: s.cur.zoom,
          lookY: s.cur.lookY,
          mobile: s.mobile,
          frame: s.frame,
          width: size.width,
          height: size.height,
        },
        shadowScratch
      )
      if (
        Math.abs(cam.left - bounds.left) > 1e-3 ||
        Math.abs(cam.right - bounds.right) > 1e-3 ||
        Math.abs(cam.top - bounds.top) > 1e-3 ||
        Math.abs(cam.bottom - bounds.bottom) > 1e-3
      ) {
        cam.left = bounds.left
        cam.right = bounds.right
        cam.top = bounds.top
        cam.bottom = bounds.bottom
        cam.updateProjectionMatrix()
      }
    }
  })
  const mapSize = mobile ? 1024 : 2048
  return (
    <>
      <ambientLight ref={amb} intensity={0.15} />
      <hemisphereLight ref={hemi} args={['#ffffff', '#5a7ac9', 0.2]} />
      <directionalLight
        ref={dir}
        position={[13, 15, -7]}
        intensity={1.85}
        castShadow
        shadow-mapSize={[mapSize, mapSize]}
        shadow-bias={-0.0004}
        shadow-normalBias={0.03}
      >
        <orthographicCamera
          ref={shadowCam}
          attach="shadow-camera"
          args={[-MAX_HALF_EXTENT, MAX_HALF_EXTENT, MAX_HALF_EXTENT, -MAX_HALF_EXTENT, 1, 50]}
        />
      </directionalLight>
    </>
  )
}
