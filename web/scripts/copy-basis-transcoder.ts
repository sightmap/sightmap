// Puts three's Basis transcoder where the browser can fetch it.
//
// The building's surface atlases are KTX2/Basis, which the GPU cannot read
// directly: KTX2Loader fetches a WASM transcoder at runtime and turns the
// supercompressed payload into whatever block format the device actually
// supports. The transcoder ships inside the `three` package, and KTX2Loader
// wants a *directory* URL it can append two filenames to — so the two files
// have to sit together under a stable path, which rules out importing them
// with Vite's `?url`.
//
// Copied at build time rather than committed, for the same reason
// `public/atlas/` is generated rather than committed: a vendored copy of a
// node_modules file is a copy that silently drifts from the version of three
// that is actually installed. `public/basis/` is gitignored.
//
// Runs from `pnpm dev` and `pnpm build`, alongside the blog and atlas
// generators.
import { copyFileSync, mkdirSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, resolve } from 'node:path'

const require = createRequire(import.meta.url)
const OUT_DIR = resolve(import.meta.dirname, '../public/basis')

// Resolved through `three`'s own package rather than by path-walking into
// node_modules, so a hoisted or pnpm-linked install finds the same files.
const source = dirname(require.resolve('three/examples/jsm/libs/basis/basis_transcoder.js'))

mkdirSync(OUT_DIR, { recursive: true })
for (const file of ['basis_transcoder.js', 'basis_transcoder.wasm']) {
  copyFileSync(resolve(source, file), resolve(OUT_DIR, file))
}
console.log(`basis transcoder → ${OUT_DIR}`)
