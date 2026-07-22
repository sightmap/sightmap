#!/usr/bin/env node
// Single-source the canonical spec into the docs site.
//
// Reads spec/v1/schema.md (the normative human-readable reference) and writes
// a generated Starlight page at src/content/docs/reference/schema.md. The
// generated file is gitignored — the canonical file under spec/ is the only
// source. Run automatically before `dev` and `build` (see package.json).
//
// Relative links in the canonical file are rewritten to absolute GitHub URLs
// so they resolve on the docs site.

import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = fileURLToPath(new URL(".", import.meta.url));
const repoRoot = resolve(here, "..", "..");
const src = join(repoRoot, "spec/v1/schema.md");
const out = join(repoRoot, "docs/src/content/docs/reference/schema.md");

const BLOB = "https://github.com/sightmap/sightmap/blob/main";

let body = readFileSync(src, "utf8");

// Drop the leading H1 — Starlight renders the title from frontmatter.
body = body.replace(/^#\s+.*\n/, "");

// Rewrite relative links to absolute GitHub URLs (in-page #anchors untouched).
body = body
  .replace(/\[`\.\.\/VERSIONING\.md`\]\(\.\.\/VERSIONING\.md\)/g, `[the versioning policy](/reference/versioning/)`)
  .replace(/\]\(\.\.\/seps\//g, `](${BLOB}/spec/seps/`)
  .replace(/\]\(\.\.\/VERSIONING\.md\)/g, `](/reference/versioning/)`)
  .replace(/\]\(sightmap\.schema\.json\)/g, `](${BLOB}/spec/v1/sightmap.schema.json)`);

const frontmatter = `---
title: "Schema reference"
description: "Exhaustive field-level reference for the Sightmap v1 YAML format."
---

:::note[Generated from the canonical spec]
This page is generated from [\`spec/v1/schema.md\`](${BLOB}/spec/v1/schema.md) in
this repo — do not edit it here. The canonical file and
[\`sightmap.schema.json\`](${BLOB}/spec/v1/sightmap.schema.json) win on any disagreement.
:::
`;

mkdirSync(dirname(out), { recursive: true });
writeFileSync(out, frontmatter + "\n" + body);
console.log(`sync-spec: wrote ${out} from ${src}`);
