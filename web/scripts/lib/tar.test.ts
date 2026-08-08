import zlib from 'node:zlib'
import { describe, expect, it } from 'vitest'
import { MAX_MEMBER_NAME_BYTES, tar, tarGz } from './tar'

const BLOCK = 512

interface ReadMember {
  name: string
  mode: number
  mtime: number
  typeflag: string
  body: string
}

/**
 * A literal ustar reader, written out rather than imported so the assertions
 * are against the bytes on the wire and not against the writer's own idea of
 * them. The Go side (go/atlas/install.go) parses these headers for real, and
 * this is the closest a Node test gets to standing in for it.
 */
function readTar(buf: Buffer): ReadMember[] {
  const members: ReadMember[] = []
  let off = 0
  while (off + BLOCK <= buf.length) {
    const header = buf.subarray(off, off + BLOCK)
    if (header.every((b) => b === 0)) break

    const withBlankChecksum = Buffer.from(header)
    withBlankChecksum.fill(0x20, 148, 156)
    let sum = 0
    for (const b of withBlankChecksum) sum += b
    expect(parseInt(header.subarray(148, 154).toString('ascii'), 8)).toBe(sum)
    expect(header.subarray(257, 263).toString('latin1')).toBe('ustar\0')
    expect(header.subarray(263, 265).toString('ascii')).toBe('00')

    const size = parseInt(header.subarray(124, 135).toString('ascii'), 8)
    members.push({
      name: header.subarray(0, 100).toString('utf-8').replace(/\0.*$/, ''),
      mode: parseInt(header.subarray(100, 107).toString('ascii'), 8),
      mtime: parseInt(header.subarray(136, 147).toString('ascii'), 8),
      typeflag: header.subarray(156, 157).toString('ascii'),
      body: buf.subarray(off + BLOCK, off + BLOCK + size).toString('utf-8'),
    })
    off += BLOCK + Math.ceil(size / BLOCK) * BLOCK
  }
  return members
}

const member = (name: string, body: string) => ({ name, body: Buffer.from(body, 'utf-8') })

describe('tar', () => {
  it('round-trips every member, in order', () => {
    const buf = tar([member('.sightmap/config.yaml', 'version: 1\n'), member('.sightmap/home.yaml', 'views: []\n')])
    expect(readTar(buf).map((m) => [m.name, m.body])).toEqual([
      ['.sightmap/config.yaml', 'version: 1\n'],
      ['.sightmap/home.yaml', 'views: []\n'],
    ])
  })

  it('emits plain relative paths as regular files', () => {
    // Every other member type is one `sightmap atlas add` refuses outright, so
    // the writer must be incapable of producing them.
    const [m] = readTar(tar([member('.sightmap/config.yaml', 'version: 1\n')]))
    expect(m.typeflag).toBe('0')
    expect(m.name.startsWith('/')).toBe(false)
    expect(m.name).not.toContain('..')
    expect(m.mode).toBe(0o644)
  })

  it('pins mtime so two builds of one corpus are byte-identical', () => {
    const files = [member('.sightmap/config.yaml', 'version: 1\n')]
    expect(readTar(tar(files))[0].mtime).toBe(0)
    expect(tar(files).equals(tar(files))).toBe(true)
  })

  it('pads bodies to the block size and closes with two zero blocks', () => {
    const buf = tar([member('.sightmap/config.yaml', 'x')])
    expect(buf.length % BLOCK).toBe(0)
    expect(buf.subarray(buf.length - BLOCK * 2).every((b) => b === 0)).toBe(true)
    // One header, one padded body, two end blocks.
    expect(buf.length).toBe(BLOCK * 4)
  })

  it('handles a body that lands exactly on a block boundary', () => {
    const body = 'y'.repeat(BLOCK)
    expect(readTar(tar([member('.sightmap/exact.yaml', body)]))[0].body).toBe(body)
  })

  it('refuses a name ustar cannot represent instead of truncating it', () => {
    const name = `.sightmap/${'a'.repeat(MAX_MEMBER_NAME_BYTES)}.yaml`
    expect(() => tar([member(name, 'x')])).toThrow(/longer than 100 bytes/)
  })
})

describe('tarGz', () => {
  it('gzips a stream that unpacks back to the same members', () => {
    const files = [member('.sightmap/config.yaml', 'version: 1\n')]
    expect(readTar(zlib.gunzipSync(tarGz(files))).map((m) => m.name)).toEqual(['.sightmap/config.yaml'])
  })
})
