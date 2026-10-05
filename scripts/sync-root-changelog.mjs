#!/usr/bin/env node
// Mirror the release notes to a CHANGELOG.md at the repo root.
//
// changesets writes each package's notes beside its package.json, and the one
// published package lives in go/npm/, so the canonical changelog is
// go/npm/CHANGELOG.md. Readers look for it at the root, as in subtext, so this
// copies it there. go/npm/CHANGELOG.md stays the source of truth: never edit
// the root copy by hand.
//
// Runs from `npm run version-packages`, after `changeset version` has written
// the new block, so the root copy lands in the same "Version Packages" PR.
//
// Pure Node.js (no deps). Idempotent.
//
//   node scripts/sync-root-changelog.mjs          # write CHANGELOG.md
//   node scripts/sync-root-changelog.mjs --check  # CI: fail if it has drifted

import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const SOURCE = join(REPO_ROOT, 'go', 'npm', 'CHANGELOG.md');
const TARGET = join(REPO_ROOT, 'CHANGELOG.md');

const NOTE =
  '<!-- Generated from go/npm/CHANGELOG.md by scripts/sync-root-changelog.mjs. Do not edit. -->\n\n';

const want = NOTE + readFileSync(SOURCE, 'utf8');

if (process.argv.includes('--check')) {
  const have = existsSync(TARGET) ? readFileSync(TARGET, 'utf8') : '';
  if (have !== want) {
    console.error(
      'CHANGELOG.md is out of sync with go/npm/CHANGELOG.md.\n' +
        'Run `node scripts/sync-root-changelog.mjs` and commit CHANGELOG.md.',
    );
    process.exit(1);
  }
  console.log('CHANGELOG.md matches go/npm/CHANGELOG.md.');
  process.exit(0);
}

writeFileSync(TARGET, want);
console.log('sync-root-changelog: wrote CHANGELOG.md');
