// A minimal ustar writer, enough to publish one atlas entry's corpus as
// `/atlas/<slug>.tar.gz`.
//
// Hand-rolled rather than pulled from npm because the archive the CLI will
// accept is a narrow shape: plain relative paths, regular files, nothing else.
// A general-purpose tar library's job is to reproduce whatever it is given —
// modes, ownership, symlinks, PAX records, per-file mtimes — and every one of
// those is either a member type `sightmap atlas add` refuses outright or a byte
// that changes between builds for no reason. Sixty lines that can only emit the
// accepted shape is a smaller thing to audit than a dependency plus the config
// that constrains it. Also keeps `web/`'s dependency list where it is; the site
// ships very little.
//
// Every field that could vary is pinned: mode 0644, uid/gid 0, mtime 0, no
// owner names. Two builds of the same corpus produce the same bytes, so a
// deploy that changes nothing publishes an identical archive.
import zlib from 'node:zlib'

export interface TarMember {
  /** Archive member name, slash-separated and relative. */
  name: string
  body: Buffer
}

const BLOCK = 512

/** ustar's longest representable name without the prefix field. */
export const MAX_MEMBER_NAME_BYTES = 100

/** Zero-padded octal in a fixed-width field, NUL-terminated as ustar wants. */
function octal(value: number, width: number): string {
  return value.toString(8).padStart(width - 1, '0') + '\0'
}

function header(name: string, size: number): Buffer {
  const nameBytes = Buffer.from(name, 'utf-8')
  if (nameBytes.length > MAX_MEMBER_NAME_BYTES) {
    throw new Error(`archive member name is longer than ${MAX_MEMBER_NAME_BYTES} bytes: ${name}`)
  }
  const buf = Buffer.alloc(BLOCK)
  nameBytes.copy(buf, 0)
  buf.write(octal(0o644, 8), 100, 'ascii') // mode
  buf.write(octal(0, 8), 108, 'ascii') // uid
  buf.write(octal(0, 8), 116, 'ascii') // gid
  buf.write(octal(size, 12), 124, 'ascii') // size
  buf.write(octal(0, 12), 136, 'ascii') // mtime
  // The checksum is computed over the header with its own field read as
  // spaces, so the placeholder goes in before the sum and is overwritten after.
  buf.write(' '.repeat(8), 148, 'ascii')
  buf.write('0', 156, 'ascii') // typeflag: regular file
  buf.write('ustar\0', 257, 'ascii') // magic
  buf.write('00', 263, 'ascii') // version

  let sum = 0
  for (const byte of buf) sum += byte
  buf.write(octal(sum, 7), 148, 'ascii')
  buf.write(' ', 155, 'ascii')
  return buf
}

/** Pads to the next 512-byte boundary. Returns an empty buffer when aligned. */
function padding(size: number): Buffer {
  const over = size % BLOCK
  return over === 0 ? Buffer.alloc(0) : Buffer.alloc(BLOCK - over)
}

/** Builds an uncompressed tar stream. Members are written in the given order. */
export function tar(members: TarMember[]): Buffer {
  const parts: Buffer[] = []
  for (const member of members) {
    parts.push(header(member.name, member.body.length), member.body, padding(member.body.length))
  }
  // Two zero blocks are the end-of-archive marker.
  parts.push(Buffer.alloc(BLOCK * 2))
  return Buffer.concat(parts)
}

export function tarGz(members: TarMember[]): Buffer {
  return zlib.gzipSync(tar(members), { level: zlib.constants.Z_BEST_COMPRESSION })
}
