// Every sign in the city on one canvas, and one instanced quad per building
// reading its own region of it. The listing page can afford an HTML label over
// its single building; a street cannot, so the text becomes texels here.
import * as THREE from 'three'
import { drawSignAtlas, packSigns, type SignAtlas, type SignItem } from './signs'

const PLATE_H = 56
const PAD_X = 18
const FONT = '600 30px "JetBrains Mono", ui-monospace, monospace'

export interface SignTexture {
  atlas: SignAtlas
  texture: THREE.CanvasTexture
}

/** Measures, packs and draws the signs. One texture for the whole city. */
export function buildSignTexture(texts: string[]): SignTexture | null {
  const unique = [...new Set(texts.filter((t) => t.trim().length > 0))]
  if (unique.length === 0) return null
  const canvas = document.createElement('canvas')
  const measure = canvas.getContext('2d')
  if (!measure) return null
  measure.font = FONT
  const items: SignItem[] = unique.map((text) => ({
    text,
    width: measure.measureText(text).width + PAD_X * 2,
    height: PLATE_H,
  }))
  const atlas = packSigns(items, { width: 1024, pad: 2 })
  canvas.width = atlas.width
  canvas.height = atlas.height
  const ctx = canvas.getContext('2d')
  if (!ctx) return null
  drawSignAtlas(ctx, atlas, { font: FONT, plate: '#2b2d33', ink: '#f6f2ea' })
  const texture = new THREE.CanvasTexture(canvas)
  texture.colorSpace = THREE.SRGBColorSpace
  texture.anisotropy = 4
  texture.needsUpdate = true
  return { atlas, texture }
}

/** World height of a sign plate; its width follows the text. */
export const SIGN_HEIGHT = 1.15

export interface SignInstance {
  /** Index into the atlas's regions. */
  region: number
  matrix: THREE.Matrix4
}

/**
 * One quad per sign, each sampling its own region. The shader is hand-written
 * rather than a patched standard material because a sign needs no lighting and
 * because the region has to reach the vertex stage as a per-instance attribute.
 */
export function buildSignMesh(sign: SignTexture, instances: SignInstance[]): THREE.InstancedMesh | null {
  if (instances.length === 0) return null
  const geometry = new THREE.PlaneGeometry(1, 1)
  const regions = new Float32Array(instances.length * 4)
  const matrices = new Float32Array(instances.length * 16)
  instances.forEach((instance, i) => {
    const r = sign.atlas.regions[instance.region]
    regions.set([r.u, r.v, r.uw, r.vh], i * 4)
    matrices.set(instance.matrix.elements, i * 16)
  })
  geometry.setAttribute('aRegion', new THREE.InstancedBufferAttribute(regions, 4))
  const material = new THREE.ShaderMaterial({
    uniforms: { map: { value: sign.texture } },
    transparent: true,
    depthWrite: false,
    vertexShader: `
      attribute vec4 aRegion;
      varying vec2 vSign;
      void main() {
        vSign = uv * aRegion.zw + aRegion.xy;
        gl_Position = projectionMatrix * modelViewMatrix * instanceMatrix * vec4(position, 1.0);
      }
    `,
    fragmentShader: `
      uniform sampler2D map;
      varying vec2 vSign;
      void main() {
        vec4 texel = texture2D(map, vSign);
        if (texel.a < 0.02) discard;
        gl_FragColor = texel;
        #include <tonemapping_fragment>
        #include <colorspace_fragment>
      }
    `,
  })
  const mesh = new THREE.InstancedMesh(geometry, material, instances.length)
  mesh.instanceMatrix.array.set(matrices)
  mesh.instanceMatrix.needsUpdate = true
  mesh.renderOrder = 2
  mesh.computeBoundingSphere()
  return mesh
}
