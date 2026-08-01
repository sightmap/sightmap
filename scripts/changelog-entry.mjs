#!/usr/bin/env node
// Generate the docs-site changelog entry for the version being released.
//
// The published changelog at docs/changelog.mdx is assembled from per-release
// entry files in docs/changelog/entries/ (see docs/changelog/README.md). Those
// entries were written by hand, which meant a release could ship with no entry
// at all — the docs site sat a version behind npm until someone noticed.
//
// This closes that loop at the source. It runs from `npm run version-packages`,
// immediately after `changeset version` has written the new block to
// go/npm/CHANGELOG.md and bumped go/npm/package.json — so the entry lands in
// the same "Version Packages" PR as the bump that caused it, reviewable before
// merge. Same hook, and the same shape, as scripts/sync-manifest-versions.mjs.
//
// The prose is copied from the changesets block verbatim: changesets output is
// already the published wording, so there is nothing to rewrite. Only the
// changesets scaffolding is stripped — the `### Patch Changes` heading and the
// `- <commit>:` prefixes — matching how the entries were formatted by hand.
//
// Pure Node.js (no deps). Idempotent: an entry that already exists is left
// alone, so re-running never clobbers a hand-edited entry.
//
//   node scripts/changelog-entry.mjs                  # entry for the current version
//   node scripts/changelog-entry.mjs --version 0.17.1 # a specific version
//   node scripts/changelog-entry.mjs --date 2026-07-31 # override the date stamp
//   node scripts/changelog-entry.mjs --check          # CI: every release has an entry

import { readFileSync, writeFileSync, readdirSync, existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO_ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const CHANGELOG = join(REPO_ROOT, 'go', 'npm', 'CHANGELOG.md');
const PKG = join(REPO_ROOT, 'go', 'npm', 'package.json');
const ENTRIES_DIR = join(REPO_ROOT, 'docs', 'changelog', 'entries');

function arg(name) {
  const i = process.argv.indexOf(`--${name}`);
  return i !== -1 && process.argv[i + 1] ? process.argv[i + 1] : undefined;
}

// Split CHANGELOG.md into its per-version blocks, newest first.
function releases(text) {
  const out = [];
  const re = /^## (\d+\.\d+\.\d+)\s*$/gm;
  const marks = [...text.matchAll(re)];
  for (let i = 0; i < marks.length; i++) {
    const start = marks[i].index + marks[i][0].length;
    const end = i + 1 < marks.length ? marks[i + 1].index : text.length;
    out.push({ version: marks[i][1], raw: text.slice(start, end).trim() });
  }
  return out;
}

// Strip the changesets scaffolding, leaving the prose as the hand-written
// entries formatted it: one paragraph per changeset, nested bullets dedented
// to column 0, blank line between changesets.
function toEntryBody(raw) {
  const lines = raw
    .split('\n')
    .filter((l) => !/^### (Patch|Minor|Major) Changes\s*$/.test(l));

  // A changeset starts at a column-0 bullet carrying a commit hash. Continuation
  // lines are indented two spaces by changesets; everything else is body.
  const START = /^- [0-9a-f]{7,40}: ?/i;
  const chunks = [];
  let cur = null;
  for (const line of lines) {
    if (START.test(line)) {
      if (cur) chunks.push(cur);
      cur = [line.replace(START, '')];
    } else if (cur) {
      cur.push(line.startsWith('  ') ? line.slice(2) : line);
    } else if (line.trim()) {
      // No hash prefix (a changeset written without `commit`) — keep as-is.
      cur = [line];
    }
  }
  if (cur) chunks.push(cur);

  return chunks.map((c) => c.join('\n').trim()).filter(Boolean).join('\n\n');
}

// cli vs go decides which filter the entry sits under in the changelog's right
// panel. It is a judgment call about audience, not something the prose states,
// so this is a guess the reviewer is expected to confirm in the release PR.
function guessTags(body) {
  return /\bGo library\b|\bgo get\b|\bgo directive\b|\bnested Go module\b/i.test(body)
    ? ['go']
    : ['cli'];
}

// Which versions already have an entry, keyed off the frontmatter label rather
// than the filename — the label is what the build script actually renders.
function entryVersions() {
  if (!existsSync(ENTRIES_DIR)) return new Set();
  const found = new Set();
  for (const f of readdirSync(ENTRIES_DIR)) {
    if (!/\.mdx?$/.test(f)) continue;
    const m = /^label:\s*["']?sightmap\s+(\d+\.\d+\.\d+)/m.exec(
      readFileSync(join(ENTRIES_DIR, f), 'utf8'),
    );
    if (m) found.add(m[1]);
  }
  return found;
}

const all = releases(readFileSync(CHANGELOG, 'utf8'));
const have = entryVersions();

if (process.argv.includes('--check')) {
  const missing = all.filter((r) => !have.has(r.version)).map((r) => r.version);
  if (missing.length) {
    console.error(
      `Released ${missing.length === 1 ? 'version has' : 'versions have'} no changelog entry: ${missing.join(', ')}\n` +
        'The docs site would ship a version behind npm. Generate with:\n' +
        '  node scripts/changelog-entry.mjs --version <X.Y.Z>\n' +
        '  node docs/scripts/build-changelog.mjs\n' +
        'See docs/changelog/README.md.',
    );
    process.exit(1);
  }
  console.log(`All ${all.length} released versions have a changelog entry.`);
  process.exit(0);
}

const version = arg('version') ?? JSON.parse(readFileSync(PKG, 'utf8')).version;
const release = all.find((r) => r.version === version);
if (!release) {
  console.error(`No "## ${version}" block in go/npm/CHANGELOG.md — nothing to generate.`);
  process.exit(1);
}

if (have.has(version)) {
  console.log(`changelog-entry: ${version} already has an entry, leaving it alone.`);
  process.exit(0);
}

const date = arg('date') ?? new Date().toISOString().slice(0, 10);
const body = toEntryBody(release.raw);
const tags = guessTags(body);
const out = join(ENTRIES_DIR, `${date}-sightmap-${version}.mdx`);

writeFileSync(
  out,
  `---\nlabel: "sightmap ${version}"\ndate: "${date}"\ntags: ${JSON.stringify(tags)}\n---\n\n${body}\n`,
);

console.log(`changelog-entry: wrote docs/changelog/entries/${date}-sightmap-${version}.mdx`);
console.log(`  tags: ${JSON.stringify(tags)} — guessed from the prose; confirm before merging.`);
