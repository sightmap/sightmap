#!/usr/bin/env node
// Single-source the canonical spec into the docs site.
//
// Reads spec/v1/schema.md (the normative human-readable reference) and writes
// a generated Mintlify page at docs/reference/schema.md. The generated file is
// checked in — Mintlify has no build step — and CI fails when it drifts from
// the canonical file (see .github/workflows/docs.yml). Regenerate with:
//
//   node docs/scripts/sync-spec.mjs
//
// Relative links in the canonical file are rewritten to docs-site or absolute
// GitHub URLs so they resolve on the docs site.

import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = fileURLToPath(new URL(".", import.meta.url));
const repoRoot = resolve(here, "..", "..");
const src = join(repoRoot, "spec/v1/schema.md");
const out = join(repoRoot, "docs/reference/schema.md");

const BLOB = "https://github.com/sightmap/sightmap/blob/main";

let body = readFileSync(src, "utf8");

// Drop the leading H1 — Mintlify renders the title from frontmatter.
body = body.replace(/^#\s+.*\n/, "");

// Rewrite relative links to docs-site or absolute GitHub URLs (in-page
// #anchors untouched).
body = body
  .replace(/\[`\.\.\/VERSIONING\.md`\]\(\.\.\/VERSIONING\.md\)/g, `[the versioning policy](/reference/versioning)`)
  .replace(/\]\(\.\.\/seps\//g, `](${BLOB}/spec/seps/`)
  .replace(/\]\(\.\.\/VERSIONING\.md\)/g, `](/reference/versioning)`)
  .replace(/\]\(sightmap\.schema\.json\)/g, `](${BLOB}/spec/v1/sightmap.schema.json)`);

const frontmatter = `---
# AUTO-GENERATED — DO NOT EDIT BY HAND.
# Generated from spec/v1/schema.md by docs/scripts/sync-spec.mjs.
# Regenerate: node docs/scripts/sync-spec.mjs
title: "Sightmap Schema Reference"
sidebarTitle: "Schema"
description: "Exhaustive field-level reference for the Sightmap v1 YAML format, generated from the canonical spec."
---
`;

mkdirSync(dirname(out), { recursive: true });
writeFileSync(out, frontmatter + "\n" + body);
console.log(`sync-spec: wrote ${out} from ${src}`);
