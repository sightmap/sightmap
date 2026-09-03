// Invoking the Khronos `ktx` CLI, which is what turns the baked PNGs into the
// Basis-compressed .ktx2 files the scene actually ships.
//
// Why an external tool rather than a dependency: the encoder is a C++ codec.
// The npm wrappers for it ship prebuilt binaries of their own, so taking one
// as a devDependency buys nothing over asking for the reference tool, and
// costs a binary blob in the lockfile that CI would install on every run for a
// script CI never executes. The bake is on-demand; the committed .ktx2 is the
// artifact.
import { execFileSync } from 'node:child_process'
import { mkdirSync, statSync, writeFileSync } from 'node:fs'
import { dirname } from 'node:path'
import { encodePng, type Raster } from './png'

/** Non-colour maps (roughness, metalness, normals, AO) must not be sRGB-decoded. */
export type Encoding = 'linear' | 'srgb'

const KTX = process.env.KTX_BIN ?? 'ktx'

export function requireKtx(): string {
  try {
    execFileSync(KTX, ['--version'], { stdio: 'pipe' })
    return KTX
  } catch {
    throw new Error(
      `The KTX encoder ("${KTX}") is not on PATH.\n` +
        'Install KTX-Software (https://github.com/KhronosGroup/KTX-Software/releases)\n' +
        'or point KTX_BIN at the binary, then re-run this bake.'
    )
  }
}

export interface EncodeOptions {
  /** Basis ETC1S compression level, 0–5. Higher is slower and smaller. */
  clevel?: number
  /** Basis ETC1S quality, 1–255. Higher is larger and closer to the source. */
  qlevel?: number
}

/**
 * Write `image` to `outPath` as a mipmapped, ETC1S-compressed KTX2 file.
 *
 * ETC1S rather than UASTC throughout. UASTC is the right choice for a
 * high-frequency normal map, but it transcodes to a full byte per texel, and
 * on this scene the whole texture set has to fit a 4 MB mobile budget in
 * *decoded* bytes. These maps are low-amplitude surface grain on an isometric
 * architectural model, not hardware surface detail read at a glancing angle,
 * so the half-byte-per-texel format is what they are worth.
 */
export function writeKtx2(image: Raster, outPath: string, encoding: Encoding, options: EncodeOptions = {}): number {
  const ktx = requireKtx()
  mkdirSync(dirname(outPath), { recursive: true })
  const pngPath = `${outPath}.src.png`
  writeFileSync(pngPath, encodePng(image))
  const format = image.channels === 3 ? 'R8G8B8_UNORM' : 'R8_UNORM'
  execFileSync(
    ktx,
    [
      'create',
      '--format',
      format,
      // The PNG carries no colour-space chunk, so state the intent rather than
      // letting the encoder assume sRGB and silently re-curve a roughness map.
      '--assign-tf',
      encoding === 'srgb' ? 'srgb' : 'linear',
      '--generate-mipmap',
      '--encode',
      'basis-lz',
      '--clevel',
      String(options.clevel ?? 4),
      '--qlevel',
      String(options.qlevel ?? 160),
      pngPath,
      outPath,
    ],
    { stdio: 'pipe' }
  )
  return statSync(outPath).size
}
