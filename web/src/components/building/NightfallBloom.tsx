// Nightfall-only bloom on the emissive window glass, desktop-gated.
//
// Global luminance thresholding can't isolate the windows: at night the desk
// monitors and screens (Tower.tsx's furniture materials) ramp to a brighter
// emissive output than the curtain-wall glass does, so any single threshold
// either blooms every screen in the building along with the windows, or
// blooms nothing at all. Instead the glass mesh carries its own Three.js
// layer (BLOOM_LAYER, see bloom.ts, tagged in Tower.tsx) and only that layer
// is rendered into the bloom source texture — bloom never sees the rest of
// the scene, independent of how bright it is.
//
// Mechanically this is the standard "selective bloom" recipe: a small
// bloomComposer renders only BLOOM_LAYER (cheap — a handful of merged glass
// meshes, not the floors/furniture/people the main pass draws) and blurs it
// at half resolution; a finalComposer renders the normal full scene and
// additively composites the blurred bloom texture on top. Both RenderPasses
// share the renderer's own AgX tone mapping, so the two textures are already
// in the same space before they're added — only OutputPass, last in the
// chain, converts to display colour space, matching what the renderer would
// have done in one step without a composer at all.
//
// This mounts only on desktop (see the `!mobile` gate where this is used)
// and only takes over rendering (the positive useFrame priority below,
// which suppresses React Three Fiber's own auto-render for the frame) once
// nightfall's emissive ramp actually has something to bloom — outside that
// window this falls back to the exact `gl.render(scene, camera)` call R3F
// would have made itself, at the same cost.
//
// The cost that doesn't go away: any EffectComposer render target is a
// plain, non-multisampled texture, so nightfall on desktop forfeits the
// canvas's own `antialias: true` MSAA for as long as the composer is active.
// That tradeoff could not be measured visually in this sandbox — WebGL runs
// on SwiftShader here with no real MSAA hardware path — so it is flagged in
// the PR for human review rather than assumed away.
import { useFrame, useThree } from '@react-three/fiber'
import { useEffect, useMemo } from 'react'
import * as THREE from 'three'
import { EffectComposer } from 'three/addons/postprocessing/EffectComposer.js'
import { OutputPass } from 'three/addons/postprocessing/OutputPass.js'
import { RenderPass } from 'three/addons/postprocessing/RenderPass.js'
import { ShaderPass } from 'three/addons/postprocessing/ShaderPass.js'
import { UnrealBloomPass } from 'three/addons/postprocessing/UnrealBloomPass.js'
import { BLOOM_LAYER, BLOOM_NIGHT_THRESHOLD } from './bloom'
import { useShared } from './state'

/**
 * Additively composites the blurred bloom texture over the normally
 * rendered base scene. Both inputs already went through the renderer's own
 * tone mapping (see file header), so this only ever adds light on top — it
 * never re-maps it. Alpha is taken from the base pass alone: the bloom
 * target clears to the same transparent black the canvas does, so summing
 * alpha too would bleed a faint opacity halo past the building's silhouette
 * into the transparent page background around it.
 */
const COMBINE_SHADER = {
  uniforms: {
    baseTexture: { value: null as THREE.Texture | null },
    bloomTexture: { value: null as THREE.Texture | null },
  },
  vertexShader: /* glsl */ `
    varying vec2 vUv;
    void main() {
      vUv = uv;
      gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
    }
  `,
  fragmentShader: /* glsl */ `
    uniform sampler2D baseTexture;
    uniform sampler2D bloomTexture;
    varying vec2 vUv;
    void main() {
      vec4 base = texture2D(baseTexture, vUv);
      vec3 bloom = texture2D(bloomTexture, vUv).rgb;
      gl_FragColor = vec4(base.rgb + bloom, base.a);
    }
  `,
}

/** Renders only the mesh(es) on BLOOM_LAYER into a half-resolution bloom
 *  texture. `renderToScreen` stays false: this composer's sole output is
 *  the texture `finalComposer`'s combine pass reads, never the canvas. */
function createBloomComposer(gl: THREE.WebGLRenderer, scene: THREE.Scene, camera: THREE.Camera) {
  const composer = new EffectComposer(gl, new THREE.WebGLRenderTarget(1, 1))
  composer.renderToScreen = false
  composer.addPass(new RenderPass(scene, camera))
  // The curtain-wall glass is one uniform MeshStandardMaterial across every
  // floor of both glazed sides (Tower.tsx ramps its emissiveIntensity to
  // n * 0.9 as a single value, not per-window) — the bloom SOURCE is the
  // whole two-sided facade, not a scatter of individual window rects, so
  // there is no brightness variance for `threshold` to discriminate on and
  // no blur `radius` small enough to shrink the glow down to "just the
  // windows": radius only extends the fringe past the source's own edges,
  // and the source edges already span the full building height. The only
  // remaining lever is `strength` — turned down far enough to read as a
  // warm accent glow at the glazing rather than a page-wide wash. strength
  // 0.85 and even 0.22 both washed out the whole silhouette (see PR
  // screenshots); this value is deliberately conservative for the same
  // reason — favoring a restrained result over a dramatic but overblown one.
  composer.addPass(new UnrealBloomPass(new THREE.Vector2(1, 1), 0.05, 0.08, 0.4))
  return composer
}

/** Renders the full scene normally, then blends in whatever the bloom
 *  composer produced this frame, then converts to display colour space. */
function createFinalComposer(gl: THREE.WebGLRenderer, scene: THREE.Scene, camera: THREE.Camera) {
  const composer = new EffectComposer(gl)
  composer.addPass(new RenderPass(scene, camera))
  const mix = new ShaderPass(
    new THREE.ShaderMaterial({
      uniforms: THREE.UniformsUtils.clone(COMBINE_SHADER.uniforms),
      vertexShader: COMBINE_SHADER.vertexShader,
      fragmentShader: COMBINE_SHADER.fragmentShader,
    }),
    'baseTexture'
  )
  composer.addPass(mix)
  composer.addPass(new OutputPass())
  return { composer, mix }
}

export default function NightfallBloom() {
  const s = useShared()
  const { gl, scene, camera, size } = useThree()

  const bloomComposer = useMemo(
    () => createBloomComposer(gl, scene, camera),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [gl]
  )
  const final = useMemo(
    () => createFinalComposer(gl, scene, camera),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [gl]
  )

  // These composers own GPU render targets React doesn't know about; they
  // need an explicit teardown when this (desktop-only) component unmounts.
  useEffect(() => {
    return () => {
      bloomComposer.dispose()
      final.composer.dispose()
    }
  }, [bloomComposer, final])

  // Both composers issue several renderer.render() calls per frame (the
  // bloom-layer render, UnrealBloomPass's internal blur chain, the final
  // scene render, the combine pass, OutputPass). WebGLRenderer resets
  // info.render.calls/triangles at the start of *each* of those by default,
  // so Stats() (Scene.tsx) would otherwise read whatever the very last
  // full-screen pass drew — a full-screen triangle, every nightfall frame —
  // instead of the frame's real draw-call count. Taking over the reset
  // ourselves, once per frame (see useFrame below), keeps that telemetry
  // meaningful for exactly the chapter this feature touches.
  useEffect(() => {
    gl.info.autoReset = false
    return () => {
      gl.info.autoReset = true
    }
  }, [gl])

  useEffect(() => {
    const dpr = gl.getPixelRatio()
    bloomComposer.setPixelRatio(dpr)
    // Half-resolution: the bloom source render and its blur chain are the
    // most expensive part of this, and a window's glow reads the same once
    // blurred and added back in, whether its own texture was full-res or not.
    bloomComposer.setSize(size.width / 2, size.height / 2)
    final.composer.setPixelRatio(dpr)
    final.composer.setSize(size.width, size.height)
  }, [gl, bloomComposer, final, size.width, size.height])

  useFrame(() => {
    // Paired with the autoReset=false effect above: one manual reset per
    // frame, however many renderer.render() calls this frame ends up
    // issuing, so gl.info.render.* always reflects this frame's total.
    gl.info.reset()
    const night = s.cur.night
    if (night < BLOOM_NIGHT_THRESHOLD) {
      // Outside nightfall: skip both composers entirely and render exactly
      // the way the rest of the tour does, so there is no cost and no AA
      // change here.
      gl.render(scene, camera)
      return
    }
    const savedMask = camera.layers.mask
    camera.layers.set(BLOOM_LAYER)
    bloomComposer.render()
    camera.layers.mask = savedMask
    final.mix.uniforms.bloomTexture.value = bloomComposer.readBuffer.texture
    final.composer.render()
  }, 1)

  return null
}
