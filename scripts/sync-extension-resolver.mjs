#!/usr/bin/env node
/**
 * Mirror resolver.js's shared block into content.js.
 *
 * The extension's matching core lives in resolver.js as an ES module, which the
 * devtools panel and side panel import. content.js cannot: MV3 declarative
 * content_scripts run as CLASSIC scripts, with no `import` and no "type":
 * "module" key to opt in. So the block is copied in, verbatim apart from the
 * `export` keywords.
 *
 * Copying by hand is how the two drift, and they had: the same name-keyed lookup
 * bug had to be found and fixed in both. So generate it, and let
 * resolver.test.js fail the build on any drift.
 *
 *   node scripts/sync-extension-resolver.mjs           # write
 *   node scripts/sync-extension-resolver.mjs --check   # verify only (CI)
 */
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const EXT = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "go",
  "cmd",
  "sightmap",
  "extension",
);
const START = "// ─── SHARED-WITH-CONTENT:START ───";
const END = "// ─── SHARED-WITH-CONTENT:END ───";

/** The text between the markers, markers excluded. Throws if either is absent. */
export function sharedBlock(source, file) {
  const i = source.indexOf(START);
  const j = source.indexOf(END);
  if (i < 0 || j < 0 || j < i) {
    throw new Error(`${file}: SHARED-WITH-CONTENT markers missing or inverted`);
  }
  return source.slice(source.indexOf("\n", i) + 1, j);
}

/** resolver.js's block as content.js must carry it: no ES module keywords. */
export function toContentForm(block) {
  return block.replace(/^export function/gm, "function");
}

const resolver = readFileSync(join(EXT, "resolver.js"), "utf8");
const content = readFileSync(join(EXT, "content.js"), "utf8");

const want = toContentForm(sharedBlock(resolver, "resolver.js"));
const have = sharedBlock(content, "content.js");

if (want === have) {
  console.log("extension resolver block: in sync");
  process.exit(0);
}

if (process.argv.includes("--check")) {
  console.error(
    "extension resolver block: content.js has DRIFTED from resolver.js.\n" +
      "Edit resolver.js, then run: node scripts/sync-extension-resolver.mjs",
  );
  process.exit(1);
}

const i = content.indexOf(START);
const j = content.indexOf(END);
const updated =
  content.slice(0, content.indexOf("\n", i) + 1) + want + content.slice(j);
writeFileSync(join(EXT, "content.js"), updated);
console.log("extension resolver block: content.js updated from resolver.js");
