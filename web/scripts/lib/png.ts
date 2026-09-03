// A minimal PNG encoder, so the texture bake scripts can hand `ktx create` a
// real image without adding an image library to the dependency tree.
//
// Same reasoning as build-env-map.ts writing Radiance by hand: the bake writes
// one flat, non-interlaced, 8-bit image and nothing else, and the format for
// exactly that is a header, a zlib stream and a CRC. `ktx` reads it, the CI
// never runs it, and the committed .ktx2 files are the artifact.
import { deflateSync } from 'node:zlib'

/** An 8-bit image in memory: `channels` interleaved bytes per pixel, row-major. */
export interface Raster {
  width: number
  height: number
  /** 3 for RGB, 1 for greyscale. */
  channels: 1 | 3
  data: Uint8Array
}

export function raster(width: number, height: number, channels: 1 | 3): Raster {
  return { width, height, channels, data: new Uint8Array(width * height * channels) }
}

/** Write one pixel. Out-of-range coordinates are ignored, which keeps callers simple. */
export function put(r: Raster, x: number, y: number, values: readonly number[]): void {
  if (x < 0 || y < 0 || x >= r.width || y >= r.height) return
  const at = (y * r.width + x) * r.channels
  for (let c = 0; c < r.channels; c++) {
    r.data[at + c] = Math.max(0, Math.min(255, Math.round(values[c] ?? values[0])))
  }
}

const CRC_TABLE = (() => {
  const t = new Uint32Array(256)
  for (let n = 0; n < 256; n++) {
    let c = n
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    t[n] = c >>> 0
  }
  return t
})()

function crc32(buf: Uint8Array): number {
  let c = 0xffffffff
  for (let i = 0; i < buf.length; i++) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8)
  return (c ^ 0xffffffff) >>> 0
}

function chunk(type: string, body: Uint8Array): Buffer {
  const head = Buffer.alloc(8)
  head.writeUInt32BE(body.length, 0)
  head.write(type, 4, 'ascii')
  const crcInput = Buffer.concat([Buffer.from(type, 'ascii'), Buffer.from(body)])
  const tail = Buffer.alloc(4)
  tail.writeUInt32BE(crc32(crcInput), 0)
  return Buffer.concat([head, Buffer.from(body), tail])
}

/**
 * Encode `r` as a PNG.
 *
 * Every scanline is written with filter type 0 (None). Filtering only buys
 * compression, and these images are an intermediate handed straight to the
 * KTX encoder — the bytes that ship are the .ktx2, not this.
 */
export function encodePng(r: Raster): Buffer {
  const stride = r.width * r.channels
  const rows = Buffer.alloc((stride + 1) * r.height)
  for (let y = 0; y < r.height; y++) {
    rows[y * (stride + 1)] = 0
    Buffer.from(r.data.buffer, r.data.byteOffset + y * stride, stride).copy(rows, y * (stride + 1) + 1)
  }
  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(r.width, 0)
  ihdr.writeUInt32BE(r.height, 4)
  ihdr[8] = 8 // bit depth
  ihdr[9] = r.channels === 3 ? 2 : 0 // colour type: truecolour or greyscale
  ihdr[10] = 0 // deflate
  ihdr[11] = 0 // adaptive filtering
  ihdr[12] = 0 // no interlace
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr),
    chunk('IDAT', deflateSync(rows, { level: 9 })),
    chunk('IEND', new Uint8Array(0)),
  ])
}
