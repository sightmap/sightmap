// Pushes the source maps emitted by `vite build` to Replay
// (https://replay.io) so recordings of sightmap.org open on the original TSX
// rather than the minified bundle.
//
// Replay resolves maps two ways: it fetches them from the deployed site, or it
// reads a copy uploaded ahead of time and keyed by group. We do both. Fetching
// works today because vite.config.ts sets `sourcemap: true` and the maps ship
// with the deploy, but a recording outlives the deploy that produced it — once
// Netlify replaces dist/, the hashed asset URLs 404 and every older recording
// loses its mapping. The uploaded copy is what keeps those readable, so the
// --group is the commit, not the release: it has to match the exact bundle a
// given recording captured.
//
// Requires REPLAY_API_KEY (Replay's Team Settings → API keys). Set it in the
// Netlify site's build environment; without it this is a no-op, which is what
// keeps local `pnpm build` from needing Replay credentials at all.
import { execFileSync, spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const distDir = path.join(webDir, 'dist')

// Pinned to a minor range: an upload that silently changes format between
// deploys would strand recordings, and this runs unattended on Netlify.
const REPLAY_CLI = 'replayio@^1.8.2'

// Netlify exposes COMMIT_REF, GitHub Actions exposes GITHUB_SHA; neither is
// set for a local build, so fall back to git and finally to a literal so the
// upload is still well-formed when run from a dirty tree.
function resolveGroup(): string {
  const fromEnv =
    process.env.REPLAY_SOURCEMAP_GROUP ||
    process.env.COMMIT_REF ||
    process.env.GITHUB_SHA
  if (fromEnv) return fromEnv
  try {
    return execFileSync('git', ['rev-parse', 'HEAD'], {
      cwd: webDir,
      encoding: 'utf8',
    }).trim()
  } catch {
    return 'local'
  }
}

function main(): void {
  if (!process.env.REPLAY_API_KEY) {
    console.log(
      'upload-sourcemaps: REPLAY_API_KEY not set — skipping Replay upload.',
    )
    return
  }
  if (!fs.existsSync(distDir)) {
    console.warn(
      `upload-sourcemaps: ${distDir} does not exist — run \`vite build\` first.`,
    )
    return
  }

  const group = resolveGroup()
  console.log(`upload-sourcemaps: uploading dist/ to Replay as group ${group}`)

  // spawnSync, not execFileSync: `replayio upload-source-maps` exits 0 even
  // when the upload fails (verified against 1.8.2 with a bad key — it prints
  // "✘ Source maps upload failed" and still returns 0). Exit code alone would
  // report a rejected REPLAY_API_KEY as success, so we scan the output too.
  // Matching on a string is brittle, but a reworded message only puts us back
  // to trusting the exit code, which is where we'd be anyway.
  const res = spawnSync(
    'npx',
    ['--yes', REPLAY_CLI, 'upload-source-maps', '--group', group, 'dist'],
    { cwd: webDir, encoding: 'utf8' },
  )
  const output = `${res.stdout ?? ''}${res.stderr ?? ''}`
  if (output) process.stdout.write(output)

  const failed =
    res.error != null || res.status !== 0 || /upload failed/i.test(output)
  if (failed) {
    // Never fail the deploy over this. The maps still ship with dist/, so a
    // failed upload degrades old recordings, not the live site — and a
    // marketing page going dark because Replay's API blipped is the worse
    // trade. Loud enough to grep out of the Netlify build log.
    console.warn(
      `upload-sourcemaps: WARNING — Replay upload failed for group ${group}, continuing build.${
        res.error ? ` ${res.error.message}` : ''
      }`,
    )
    return
  }
  console.log(`upload-sourcemaps: uploaded group ${group} to Replay.`)
}

main()
