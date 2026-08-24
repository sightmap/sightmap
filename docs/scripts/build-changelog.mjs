#!/usr/bin/env node
// Assemble changelog/entries/*.mdx into changelog.mdx.
//
// Each entry file is one release note. Frontmatter drives a Mintlify <Update>
// block; the body is the prose shown inside it. Entries are ordered newest
// first by `date`, then by filename as a tiebreak.
//
// Zero dependencies on purpose, so the script runs under a bare `node`. Run it
// after adding or editing an entry, and commit the regenerated changelog.mdx
// (Mintlify has no build step, so the rendered file is what ships).
// See changelog/README.md.
//
//   node scripts/build-changelog.mjs           # regenerate
//   node scripts/build-changelog.mjs --check   # exit 1 if out of date (CI)

import { readFileSync, writeFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const ENTRIES_DIR = join(ROOT, 'changelog', 'entries');
const OUT = join(ROOT, 'changelog.mdx');

const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December'];

// Parse an ISO YYYY-MM-DD without Date() to avoid timezone drift.
function formatDate(iso) {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  if (!m) throw new Error(`date must be YYYY-MM-DD, got: ${iso}`);
  return `${MONTHS[+m[2] - 1]} ${+m[3]}, ${m[1]}`;
}

// Minimal frontmatter reader for our own fixed schema. Values are either a
// quoted string, a JSON array (tags), or a bare string.
function parseEntry(file, text) {
  const m = /^---\n([\s\S]*?)\n---\n?([\s\S]*)$/.exec(text);
  if (!m) throw new Error(`${file}: missing frontmatter`);
  const meta = {};
  for (const line of m[1].split('\n')) {
    if (!line.trim()) continue;
    const kv = /^(\w+):\s*(.*)$/.exec(line);
    if (!kv) throw new Error(`${file}: bad frontmatter line: ${line}`);
    const [, key, rawVal] = kv;
    let val = rawVal.trim();
    if (val.startsWith('[')) val = JSON.parse(val);
    else if (/^["'].*["']$/.test(val)) val = val.slice(1, -1);
    meta[key] = val;
  }
  if (!meta.label) throw new Error(`${file}: 'label' is required`);
  if (!meta.date) throw new Error(`${file}: 'date' is required`);
  return { file, meta, body: m[2].trim() };
}

function renderUpdate({ meta, body }) {
  const desc = meta.description
    ? `Released ${formatDate(meta.date)} — ${meta.description}`
    : `Released ${formatDate(meta.date)}`;
  const attrs = [`label="${meta.label}"`, `description="${desc}"`];
  if (Array.isArray(meta.tags) && meta.tags.length) {
    attrs.push(`tags={${JSON.stringify(meta.tags)}}`);
  }
  // Blank lines around the body so Mintlify parses lists/paragraphs as
  // markdown block content rather than JSX children.
  return `<Update ${attrs.join(' ')}>\n\n${body}\n\n</Update>`;
}

// Reject MDX-unsafe raw tags in an entry body. changelog.mdx is MDX, so a bare
// `<view>` in prose is parsed as an unclosed JSX/HTML element and fails
// `mint validate` ("Expected a closing tag for `<view>`"). Placeholders belong in
// backticks — `<view>` — which every entry already does; this guard keeps it that
// way. Code spans and fenced blocks are stripped first, so backticked tags pass.
// Lowercase-only by design: real Mintlify components are capitalized, and the
// recurring hazard is a lowercase placeholder copied verbatim from a changeset.
function lintBody(file, body) {
  const prose = body
    .replace(/```[\s\S]*?```/g, '') // fenced code blocks
    .replace(/`[^`]*`/g, '');        // inline code
  const tags = [...new Set(prose.match(/<\/?[a-z][\w-]*\/?>?/g) || [])];
  if (!tags.length) return null;
  return `${file}: raw tag(s) in prose — ${tags.join(', ')} — MDX reads these as JSX `
    + `elements and \`mint validate\` will fail. Wrap angle-bracket placeholders in `
    + `backticks, e.g. \`<view>\`.`;
}

function build() {
  let files;
  try {
    files = readdirSync(ENTRIES_DIR).filter((f) => f.endsWith('.mdx') || f.endsWith('.md'));
  } catch {
    files = [];
  }
  const entries = files
    .map((f) => parseEntry(f, readFileSync(join(ENTRIES_DIR, f), 'utf8')))
    .sort((a, b) =>
      a.meta.date === b.meta.date
        ? b.file.localeCompare(a.file)
        : b.meta.date.localeCompare(a.meta.date));

  const problems = entries.map((e) => lintBody(e.file, e.body)).filter(Boolean);
  if (problems.length) {
    console.error('changelog: MDX-unsafe entry content:\n'
      + problems.map((p) => '  - ' + p).join('\n'));
    process.exit(1);
  }

  const header = `---
title: "Changelog"
description: "New features, improvements, and fixes in the sightmap CLI, library, and spec."
rss: true
---

{/* GENERATED FILE — do not edit by hand.
    Add an entry in changelog/entries/, then run: node scripts/build-changelog.mjs
    Docs: changelog/README.md */}

`;

  const body = entries.length
    ? entries.map(renderUpdate).join('\n\n') + '\n'
    : 'No entries yet.\n';

  return header + body;
}

const out = build();
const check = process.argv.includes('--check');

if (check) {
  let current = '';
  try { current = readFileSync(OUT, 'utf8'); } catch {}
  if (current !== out) {
    console.error('changelog.mdx is out of date. Run: node scripts/build-changelog.mjs');
    process.exit(1);
  }
  console.log('changelog.mdx is up to date.');
} else {
  writeFileSync(OUT, out);
  const n = (out.match(/<Update /g) || []).length;
  console.log(`Wrote changelog.mdx (${n} ${n === 1 ? 'entry' : 'entries'}).`);
}
