// Compiles the site's own WebMCP tool layer (.sightkick/tools.yaml) against the
// site's own sightmap corpus (.sightmap/app.yaml) and drops the two artifacts
// the browser needs into public/atlas/:
//
//   public/atlas/tools.ir.json          the compiled IR — the tools themselves
//   public/atlas/sightkick-runtime.js   the runtime that registers them on
//                                       document.modelContext
//
// src/lib/sightkick-boot.ts fetches both on /atlas pages. That makes the Atlas
// the first page of sightmap.org to expose its own tools, and the reason
// sightmap.org can be listed in its own directory.
//
// Runs immediately AFTER scripts/build-atlas.ts, never before: build-atlas
// wipes public/atlas/ wholesale (it is generated and gitignored), so anything
// written first is deleted.
//
// This layer is optional by construction. `sightkick` is a devDependency, but a
// missing binary, a corpus edit that stops compiling, or any other failure here
// must not cost the site a deploy — so every failure path logs loudly and exits
// 0, having written nothing. A build without the layer serves /atlas exactly as
// it does today; the loader 404s and gives up quietly.
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { spawnSync } from 'node:child_process'

const PUBLIC_DIR = path.resolve('public/atlas')
const IR_NAME = 'tools.ir.json'
const RUNTIME_NAME = 'sightkick-runtime.js'

/** Loud, single-shape warning: this is the only thing a broken layer prints. */
function warn(reason: string): void {
  console.warn(`\n⚠  sightkick: ${reason}`)
  console.warn('⚠  /atlas will ship without its WebMCP tool layer. The rest of the site is unaffected.\n')
}

/** Runs `pnpm exec sightkick …`, returning its stderr/stdout on failure. */
function sightkick(args: string[]): { ok: true } | { ok: false; detail: string } {
  const res = spawnSync('pnpm', ['exec', 'sightkick', ...args], {
    stdio: ['ignore', 'pipe', 'pipe'],
    encoding: 'utf8',
  })
  if (res.error) return { ok: false, detail: res.error.message }
  if (res.status !== 0) {
    const detail = `${res.stderr ?? ''}${res.stdout ?? ''}`.trim() || `exited with status ${res.status}`
    return { ok: false, detail }
  }
  return { ok: true }
}

function main(): void {
  if (!fs.existsSync(path.resolve('.sightkick'))) {
    warn('no .sightkick/ tool layer in this directory')
    return
  }

  // Compile into a scratch directory first, so a failed run cannot leave a
  // half-written IR (or a stale one from a previous build) under public/atlas/.
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'sightkick-'))
  try {
    const ir = path.join(tmp, IR_NAME)
    const runtime = path.join(tmp, RUNTIME_NAME)

    const built = sightkick(['build', '.', '-o', ir])
    if (!built.ok) {
      warn(`build failed —\n${built.detail}`)
      return
    }
    const emitted = sightkick(['runtime', '-o', runtime])
    if (!emitted.ok) {
      warn(`runtime emit failed —\n${emitted.detail}`)
      return
    }
    if (!fs.existsSync(ir) || !fs.existsSync(runtime)) {
      warn('the CLI reported success but wrote no output')
      return
    }

    fs.mkdirSync(PUBLIC_DIR, { recursive: true })
    fs.copyFileSync(ir, path.join(PUBLIC_DIR, IR_NAME))
    fs.copyFileSync(runtime, path.join(PUBLIC_DIR, RUNTIME_NAME))

    const tools = (JSON.parse(fs.readFileSync(ir, 'utf8')) as { tools?: unknown[] }).tools ?? []
    console.log(`sightkick: ${tools.length} tool(s) → public/atlas/${IR_NAME} + ${RUNTIME_NAME}`)
  } catch (err) {
    // Anything unforeseen (a read-only tmpdir, malformed IR) lands here and is
    // still non-fatal, for the same reason.
    warn(`unexpected failure — ${err instanceof Error ? err.message : String(err)}`)
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true })
  }
}

main()
