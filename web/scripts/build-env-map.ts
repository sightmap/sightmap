// Generates the building's environment map: a small procedural sky, written
// as a Radiance (.hdr) file and committed next to the scene that loads it.
//
// Why generate rather than download: the scene needs something for glass at
// roughness 0.1 and steel at metalness 0.2 to reflect, not a photograph. A
// studio sky — warm zenith, cool horizon, a broad sun agreeing with the
// directional light, a dull ground bounce, and two soft bands that give the
// glass visible structure — does that job in a fraction of the bytes any real
// HDR would cost, and it ships in the bundle instead of coming off a CDN.
//
// Format note: 256x128 RGBE written flat (no RLE). 128 KiB of payload, well
// inside the 512 KB budget, and no run-length encoder to get subtly wrong.
// The source is deliberately low-resolution: PMREM blurs it into irradiance
// and roughness mips at load, and a small source cannot alias under a
// scroll-driven camera.
//
// Run on demand — this is NOT part of `pnpm build`. The committed .hdr is the
// artifact; regenerate and commit it when you change the constants below:
//   cd web && pnpm tsx scripts/build-env-map.ts
import { writeFileSync } from 'node:fs'
import { resolve } from 'node:path'

const WIDTH = 256
const HEIGHT = 128
const OUT = resolve(import.meta.dirname, '../src/components/building/skylight.hdr')

/** Matches Lights.tsx's directionalLight position, so speculars agree with shadows. */
const SUN_DIRECTION = norm([13, 15, -7])
/** Broad and soft: a hard disc would put a mirror-ball dot on every steel edge. */
const SUN_ANGULAR_RADIUS = Math.PI / 5
const SUN_RADIANCE: RGB = [11, 9.4, 7.6]

const ZENITH: RGB = [0.62, 0.68, 0.82]
const HORIZON: RGB = [0.78, 0.79, 0.83]
/** The drafting table and the room it sits in: dull, warm, and not black. */
const GROUND: RGB = [0.19, 0.17, 0.15]
/** Two soft overhead bands — the structure glass actually reflects. */
const BAND_RADIANCE = 2.1

type RGB = [number, number, number]

function norm([x, y, z]: number[]): RGB {
  const l = Math.hypot(x, y, z)
  return [x / l, y / l, z / l]
}

function smoothstep(t: number): number {
  const c = Math.min(1, Math.max(0, t))
  return c * c * (3 - 2 * c)
}

function mix(a: RGB, b: RGB, t: number): RGB {
  return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, a[2] + (b[2] - a[2]) * t]
}

/**
 * Radiance for one direction.
 *
 * `dir` is a unit vector in the same space the renderer samples the equirect
 * map in, so `dir[1]` is up and the sun lands where Lights.tsx puts it.
 */
function radiance(dir: RGB): RGB {
  const up = dir[1]

  // Sky above, ground below, with a soft wrap through the horizon so the
  // irradiance has no seam for a flat roof to catch.
  const sky = mix(HORIZON, ZENITH, smoothstep(up * 1.35))
  let out = mix(GROUND, sky, smoothstep((up + 0.06) / 0.16))

  // Two broad bands of brighter sky, one high and one just above the horizon.
  // Curved surfaces read these as moving highlights as the camera drifts.
  for (const [centre, width] of [
    [0.62, 0.16],
    [0.2, 0.1],
  ]) {
    const band = Math.exp(-((up - centre) ** 2) / (2 * width * width))
    const lit = up > 0 ? band * BAND_RADIANCE : 0
    out = [out[0] + lit * 0.9, out[1] + lit * 0.95, out[2] + lit]
  }

  // The sun: a wide cosine falloff rather than a disc.
  const cos = dir[0] * SUN_DIRECTION[0] + dir[1] * SUN_DIRECTION[1] + dir[2] * SUN_DIRECTION[2]
  const angle = Math.acos(Math.min(1, Math.max(-1, cos)))
  if (angle < SUN_ANGULAR_RADIUS) {
    const f = smoothstep(1 - angle / SUN_ANGULAR_RADIUS) ** 2
    out = [out[0] + SUN_RADIANCE[0] * f, out[1] + SUN_RADIANCE[1] * f, out[2] + SUN_RADIANCE[2] * f]
  }

  return out
}

/**
 * Direction for a pixel, in three.js's equirectangular convention:
 *   u = atan2(dir.z, dir.x) / 2pi + 0.5,  v = asin(dir.y) / pi + 0.5
 * with image row 0 at the top (v = 1).
 */
function direction(x: number, y: number): RGB {
  const u = (x + 0.5) / WIDTH
  const v = 1 - (y + 0.5) / HEIGHT
  const dy = Math.sin((v - 0.5) * Math.PI)
  const r = Math.cos((v - 0.5) * Math.PI)
  const a = (u - 0.5) * 2 * Math.PI
  return [r * Math.cos(a), dy, r * Math.sin(a)]
}

/** Shared-exponent RGBE encoding, as Radiance defines it. */
function encodeRGBE([r, g, b]: RGB, out: Uint8Array, at: number): void {
  const peak = Math.max(r, g, b)
  if (peak < 1e-32) {
    out[at] = out[at + 1] = out[at + 2] = out[at + 3] = 0
    return
  }
  const e = Math.ceil(Math.log2(peak))
  const scale = 255.999 / 2 ** e
  out[at] = Math.min(255, Math.floor(r * scale))
  out[at + 1] = Math.min(255, Math.floor(g * scale))
  out[at + 2] = Math.min(255, Math.floor(b * scale))
  out[at + 3] = e + 128
}

const pixels = new Uint8Array(WIDTH * HEIGHT * 4)
let sum: RGB = [0, 0, 0]
for (let y = 0; y < HEIGHT; y++) {
  // Solid-angle weight, so the reported average is irradiance-like rather
  // than dominated by the squashed poles.
  const weight = Math.sin(((y + 0.5) / HEIGHT) * Math.PI)
  for (let x = 0; x < WIDTH; x++) {
    const c = radiance(direction(x, y))
    encodeRGBE(c, pixels, (y * WIDTH + x) * 4)
    sum = [sum[0] + c[0] * weight, sum[1] + c[1] * weight, sum[2] + c[2] * weight]
  }
}

const totalWeight = WIDTH * Array.from({ length: HEIGHT }, (_, y) => Math.sin(((y + 0.5) / HEIGHT) * Math.PI)).reduce((a, b) => a + b, 0)
const header = Buffer.from(`#?RADIANCE\n# Generated by web/scripts/build-env-map.ts - do not hand-edit\nFORMAT=32-bit_rle_rgbe\n\n-Y ${HEIGHT} +X ${WIDTH}\n`, 'ascii')
const file = Buffer.concat([header, Buffer.from(pixels)])
writeFileSync(OUT, file)

const avg = sum.map((v) => v / totalWeight)
console.log(`wrote ${OUT}`)
console.log(`  ${WIDTH}x${HEIGHT} RGBE, ${(file.byteLength / 1024).toFixed(1)} KiB`)
console.log(`  mean radiance: ${avg.map((v) => v.toFixed(3)).join(', ')}`)
