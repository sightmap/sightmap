// The image-based light for the model.
//
// Before this, glass at roughness 0.1 and steel at metalness 0.2 had nothing
// to reflect: a metal with no environment resolves to something close to
// black, which is why metalness read as darker paint rather than metal. A
// small skylight HDR gives every specular surface something to return, and
// pays for the ambient and hemisphere fill this scene used to need instead.
//
// The HDR ships in the bundle (Vite fingerprints it like any other asset) —
// deliberately not drei's <Environment preset>, which fetches multi-megabyte
// HDRs from a CDN at runtime.
import { useFrame, useThree } from '@react-three/fiber'
import { useEffect } from 'react'
import * as THREE from 'three'
import { RGBELoader } from 'three-stdlib'
import { useShared } from './state'
import skylightUrl from './skylight.hdr?url'

/**
 * Scales the whole IBL.
 *
 * The map is authored as plausible radiance (mean ~1.2 over the sphere), so at
 * 1.0 it puts back more flat fill than the ambient and hemisphere cuts take
 * out — measured on the day scene that read as *less* interior contrast, the
 * opposite of the point. At 0.34 the environment contributes ~0.42 of diffuse
 * against the old fill's ~1.42: the sun keeps the contrast, and the map is
 * here to be reflected by glass and steel rather than to light the model.
 */
export const ENVIRONMENT_INTENSITY = 0.34

/**
 * The same daylit sky, dimmed to a memory of itself at nightfall.
 *
 * There is one map and it is a day map. Left at full strength after dark it
 * lit the model with cool daylight the sun had already left, which washed the
 * warm emissive out of the lit windows — nightfall is the chapter the rest of
 * the tour is measured against, and it lost its glow before this was added.
 * Dimming the environment as night falls is also just what happens outdoors.
 */
export const ENVIRONMENT_INTENSITY_NIGHT = 0.1

/**
 * The decoded equirect texture, shared by every canvas on the page.
 *
 * The decode is renderer-independent, so the tour and the homepage billboard
 * fetch and parse one file between them. The PMREM *result* is not — it lives
 * in a render target owned by one renderer — so that part is per-canvas below.
 */
let decoding: Promise<THREE.DataTexture> | null = null

function loadSkylight(): Promise<THREE.DataTexture> {
  decoding ??= new Promise((resolve, reject) => {
    new RGBELoader().load(skylightUrl, resolve, undefined, (err) =>
      reject(err instanceof Error ? err : new Error(`failed to load ${skylightUrl}`))
    )
  })
  return decoding
}

/**
 * Pre-filters the skylight into this canvas's `scene.environment`.
 *
 * Lives inside the shared SceneContent, so the tour and the billboard get the
 * same lighting without either canvas configuring it for itself.
 */
export default function BakedEnvironment() {
  const { gl, scene, invalidate } = useThree()
  const shared = useShared()

  // Tracks the same night factor the lights do. Cheap: a scalar write per
  // frame, and only on frames that were going to be drawn anyway.
  useFrame(() => {
    if (!scene.environment) return
    scene.environmentIntensity = THREE.MathUtils.lerp(
      ENVIRONMENT_INTENSITY,
      ENVIRONMENT_INTENSITY_NIGHT,
      shared.cur.night
    )
  })

  useEffect(() => {
    let target: THREE.WebGLRenderTarget | null = null
    let live = true

    loadSkylight()
      .then((equirect) => {
        if (!live) return
        const pmrem = new THREE.PMREMGenerator(gl)
        target = pmrem.fromEquirectangular(equirect)
        pmrem.dispose()
        scene.environment = target.texture
        scene.environmentIntensity = ENVIRONMENT_INTENSITY
        // Under reduced motion the tour renders on demand and is very likely
        // already settled by the time the HDR lands. Without this the scene
        // keeps the frame it drew before the environment existed.
        invalidate()
      })
      .catch((err) => {
        // A missing environment is a visible downgrade, not a crash: the
        // lights still light the scene. Say so rather than swallowing it.
        console.error('[building] environment map failed to load; falling back to lights only', err)
      })

    return () => {
      live = false
      scene.environment = null
      target?.dispose()
    }
  }, [gl, scene, invalidate])

  return null
}
