#!/usr/bin/env node
// Sync the plugin manifest versions to the sightmap package version.
//
// The release is tag-driven (goreleaser + the npm publish take the version from
// the pushed git tag), but the root plugin manifests carry their own `version`
// fields that the harness UIs display. Those are not touched by the release
// flow, so they drift. This script writes a single source-of-truth version into
// every manifest.
//
// Source of truth: go/npm/package.json's `version` (override with --version).
// Release prep: bump go/npm/package.json, run this, commit, then tag.
//
// Pure Node.js (no deps). Idempotent.

import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');

function arg(name) {
  const i = process.argv.indexOf(`--${name}`);
  return i !== -1 && process.argv[i + 1] ? process.argv[i + 1] : undefined;
}

const metaPkg = JSON.parse(readFileSync(join(REPO_ROOT, 'go', 'npm', 'package.json'), 'utf8'));
const version = (arg('version') ?? metaPkg.version).replace(/^v/, '');

// Manifests whose top-level `version` field mirrors the package version.
const TOP_LEVEL_MANIFESTS = [
  '.claude-plugin/plugin.json',
  '.codex-plugin/plugin.json',
  '.cursor-plugin/plugin.json',
];

let touched = 0;

for (const rel of TOP_LEVEL_MANIFESTS) {
  const path = join(REPO_ROOT, rel);
  if (!existsSync(path)) continue;
  const json = JSON.parse(readFileSync(path, 'utf8'));
  if (json.version === version) continue;
  json.version = version;
  writeFileSync(path, JSON.stringify(json, null, 2) + '\n');
  console.log(`sync: ${rel} → ${version}`);
  touched++;
}

// marketplace.json: version lives under plugins[0].
const marketplacePath = join(REPO_ROOT, '.claude-plugin/marketplace.json');
if (existsSync(marketplacePath)) {
  const marketplace = JSON.parse(readFileSync(marketplacePath, 'utf8'));
  if (marketplace.plugins?.[0] && marketplace.plugins[0].version !== version) {
    marketplace.plugins[0].version = version;
    writeFileSync(marketplacePath, JSON.stringify(marketplace, null, 2) + '\n');
    console.log(`sync: .claude-plugin/marketplace.json (plugins[0]) → ${version}`);
    touched++;
  }
}

if (touched === 0) {
  console.log(`sync: all plugin manifests already at ${version}`);
}
