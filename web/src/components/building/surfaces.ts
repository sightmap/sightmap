// Loading the baked surface atlases, and pointing a material at one tile.
//
// The atlases are three KTX2/Basis files baked by scripts/build-building-textures.ts.
// Each holds four tiling surfaces in a 2×2 grid, packed as a roughness/metalness
// map and a tangent-space normal map. What this module adds is the part the
// bake cannot: getting a material to sample *one* cell of an atlas while the
// surface tiles across it many times over.
import * as THREE from 'three'
import { KTX2Loader } from 'three-stdlib'
import { tileTransform, type AtlasLayout, type Tile } from './texture-atlas'

export type AtlasName = 'arch' | 'furniture' | 'soft'

export interface SurfaceAtlas {
  /** R unused · G roughness · B metalness, all as multipliers on the material. */
  orm: THREE.CompressedTexture
  normal: THREE.CompressedTexture
}

export type SurfaceAtlases = Record<AtlasName, SurfaceAtlas>

const ATLAS_NAMES: AtlasName[] = ['arch', 'furniture', 'soft']

/**
 * The decoded atlases, shared by every canvas on the page.
 *
 * Cached at module scope like the skylight HDR next door, and for the same
 * reason: the tour and the homepage billboard are two renderers that should
 * fetch and transcode one set of files between them, not two.
 *
 * Unlike the HDR, the *result* is shareable too — a transcoded compressed
 * texture is plain image data with no renderer-owned render target behind it.
 */
let loading: Promise<SurfaceAtlases> | null = null

function loadOne(loader: KTX2Loader, url: string): Promise<THREE.CompressedTexture> {
  return new Promise((resolve, reject) => {
    loader.load(
      url,
      (texture) => resolve(texture as THREE.CompressedTexture),
      undefined,
      (err) => reject(err instanceof Error ? err : new Error(`failed to load ${url}`))
    )
  })
}

/**
 * Fetch and transcode all six atlas images.
 *
 * `renderer` is needed once, to detect which compressed formats this device
 * supports — Basis is a supercompressed intermediate, and what it becomes
 * (ETC, BC, ASTC) depends on the GPU. It is not retained.
 */
export function loadSurfaceAtlases(renderer: THREE.WebGLRenderer): Promise<SurfaceAtlases> {
  loading ??= (async () => {
    const loader = new KTX2Loader().setTranscoderPath('/basis/').detectSupport(renderer)
    try {
      const loaded = await Promise.all(
        ATLAS_NAMES.map(async (name) => {
          const [orm, normal] = await Promise.all([
            loadOne(loader, `/building/textures/${name}-orm.ktx2`),
            loadOne(loader, `/building/textures/${name}-normal.ktx2`),
          ])
          for (const t of [orm, normal]) {
            // The atlas is addressed by arithmetic in the shader, never by the
            // sampler's wrap mode — clamping is what stops a rounding error at
            // a tile edge from wrapping to the far side of the image.
            t.wrapS = THREE.ClampToEdgeWrapping
            t.wrapT = THREE.ClampToEdgeWrapping
            t.anisotropy = 4
            // Non-colour data. Transcoded Basis carries its own transfer
            // function, but say it here too so a future format change cannot
            // quietly put an sRGB curve on a roughness value.
            t.colorSpace = THREE.NoColorSpace
          }
          return [name, { orm, normal }] as const
        })
      )
      return Object.fromEntries(loaded) as SurfaceAtlases
    } finally {
      loader.dispose()
    }
  })()
  return loading
}

/** Reset the module cache. Tests only — the app wants exactly one load. */
export function resetSurfaceAtlasCache(): void {
  loading = null
}

export interface TiledSurface {
  atlas: SurfaceAtlas
  layout: AtlasLayout
  tile: Tile
  /** How many times the surface repeats across the geometry's UV range. */
  repeat: readonly [number, number]
  /** Give the material a metalnessMap too. Only worth it for actual metal. */
  metalness?: boolean
  /** Normal map strength. Grain wants a light touch. */
  normalScale?: number
}

/**
 * Point `material` at one tile of an atlas.
 *
 * three transforms each map slot's UV in the vertex shader, which can offset
 * and scale but cannot wrap *within* a sub-rectangle — set `repeat` above 1 on
 * an atlased texture and the surface samples straight into the neighbouring
 * tile. The wrap has to happen per-fragment, after interpolation.
 *
 * So the patch computes the tile coordinate in the fragment shader and then
 * redirects three's own UV varyings at it with two `#define`s. Placing the
 * defines *after* the line that reads the original varying is what makes this
 * work: the preprocessor only rewrites the lines below, so the declaration and
 * the vertex-side assignment are untouched and every sampler in the chunks
 * below picks up the tile coordinate instead. Nothing in three's shader chunks
 * is transcribed, which is what keeps this from breaking on a three upgrade.
 *
 * The tile rectangle arrives as uniforms rather than inlined constants so that
 * every tiled material in the scene shares one compiled program.
 */
export function applyTiledSurface(material: THREE.MeshStandardMaterial, s: TiledSurface): void {
  const { offset, scale } = tileTransform(s.layout, s.tile)

  material.roughnessMap = s.atlas.orm
  material.normalMap = s.atlas.normal
  material.normalScale = new THREE.Vector2(s.normalScale ?? 0.5, s.normalScale ?? 0.5)
  if (s.metalness) material.metalnessMap = s.atlas.orm

  const previous = material.onBeforeCompile
  material.onBeforeCompile = (shader, renderer) => {
    previous?.call(material, shader, renderer)
    shader.uniforms.uTileRect = { value: new THREE.Vector4(offset[0], offset[1], scale, scale) }
    shader.uniforms.uTileRepeat = { value: new THREE.Vector2(s.repeat[0], s.repeat[1]) }
    shader.fragmentShader = shader.fragmentShader
      .replace('void main() {', 'uniform vec4 uTileRect;\nuniform vec2 uTileRepeat;\nvoid main() {')
      .replace(
        // Anchored at the first of the three chunks that sample these maps.
        // three's fragment main runs roughness, then metalness, then the
        // normal chunks, so injecting here covers all of them; anchoring at
        // the normal chunk would silently leave roughness on the raw UV.
        '#include <roughnessmap_fragment>',
        [
          // vRoughnessMapUv is three's own varying and equals the raw uv here,
          // because the atlas textures carry no offset/repeat of their own.
          '\tvec2 sTileUv = uTileRect.xy + fract( vRoughnessMapUv * uTileRepeat ) * uTileRect.zw;',
          '\t#define vRoughnessMapUv sTileUv',
          '\t#define vNormalMapUv sTileUv',
          '\t#define vMetalnessMapUv sTileUv',
          '#include <roughnessmap_fragment>',
        ].join('\n')
      )
  }

  // Every tiled material compiles the same program and differs only by
  // uniform, so one key covers them all — but it must differ from an
  // unpatched material's, or three hands this one that program instead.
  const inherited = material.customProgramCacheKey?.()
  material.customProgramCacheKey = () => `tiledSurface${inherited ? `|${inherited}` : ''}`

  material.needsUpdate = true
}
