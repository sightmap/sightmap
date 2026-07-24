#!/usr/bin/env node
// Sync the per-harness manifest versions to the sightmap package version.
//
// The root plugin manifests (.claude-plugin/, .codex-plugin/, .cursor-plugin/)
// and the marketplace listing carry their own `version` fields that the
// harnesses' UIs display. This script writes a single source-of-truth version
// into every manifest.
//
// This is the sightmap counterpart to subtext's scripts/sync-manifest-versions.mjs
// and behaves identically. It differs only where the two repos legitimately
// differ:
//   - Version source: subtext reads the root package.json (bumped by changesets);
//     sightmap's release is tag-driven, so the source of truth is
//     go/npm/package.json's `version` (override with --version, e.g. from a tag).
//   - Manifest set: sightmap ships no Gemini extension (its manifest is MCP-only
//     and has no skills concept), so gemini-extension.json is not in the list.
//
// Release prep: bump go/npm/package.json, run this, commit, then tag.
//
// Pure Node.js (no deps). Idempotent.

import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');

function arg(name) {
  const i = process.argv.indexOf(`--${name}`);
  return i !== -1 && process.argv[i + 1] ? process.argv[i + 1] : undefined;
}

const { version: pkgVersion } = JSON.parse(
  readFileSync(join(REPO_ROOT, 'go', 'npm', 'package.json'), 'utf8'),
);
const version = (arg('version') ?? pkgVersion).replace(/^v/, '');

const PER_HARNESS_MANIFESTS = [
  '.claude-plugin/plugin.json',
  '.codex-plugin/plugin.json',
  '.cursor-plugin/plugin.json',
];

let touched = 0;

for (const rel of PER_HARNESS_MANIFESTS) {
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
  console.log(`sync: all manifests already at ${version}`);
}
