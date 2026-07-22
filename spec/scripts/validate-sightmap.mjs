#!/usr/bin/env node
// Validate YAML files against the Sightmap JSON Schema.
//
// Usage:
//   node scripts/validate-sightmap.mjs <glob-or-dir> [<glob-or-dir> ...]
//
// Examples:
//   node scripts/validate-sightmap.mjs .sightmap
//   node scripts/validate-sightmap.mjs spec/v1/examples
//
// Conformance fixtures whose `expected.json` declares the input as
// expected-to-fail (via a `fmt.schema-invalid` or `fmt.parse-error`
// diagnostic) are skipped — they are intentionally invalid inputs
// designed to test the formatter's refuse-to-rewrite behavior.
//
// Exits non-zero on any unexpected validation or parse error.

import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join, extname, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import yaml from "js-yaml";
import Ajv2020 from "ajv/dist/2020.js";

const __dirname = fileURLToPath(new URL(".", import.meta.url));
const repoRoot = resolve(__dirname, "..");
const schemaPath = join(repoRoot, "v1/sightmap.schema.json");

const schema = JSON.parse(readFileSync(schemaPath, "utf8"));
const ajv = new Ajv2020({ allErrors: true, strict: false });
const validate = ajv.compile(schema);

const roots = process.argv.slice(2);
if (roots.length === 0) {
  console.error("usage: validate-sightmap.mjs <dir> [<dir> ...]");
  process.exit(2);
}

function collect(p) {
  const files = [];
  const st = statSync(p);
  if (st.isDirectory()) {
    for (const entry of readdirSync(p)) {
      files.push(...collect(join(p, entry)));
    }
  } else if (st.isFile()) {
    const ext = extname(p);
    if (ext === ".yaml" || ext === ".yml") files.push(p);
  }
  return files;
}

// Fixture-aware skip: when a YAML lives under a fixture's `sightmap/`
// directory and the fixture's `expected.json` declares the input as
// expected-to-fail (via `fmt.schema-invalid` or `fmt.parse-error`), skip
// schema validation. `.expected/` outputs are always validated — they
// are canonical and must conform.
function shouldSkipInputAsExpectedInvalid(file) {
  const parts = file.split(sep);
  const sightmapIdx = parts.lastIndexOf("sightmap");
  if (sightmapIdx <= 0) return false;
  if (parts.slice(sightmapIdx + 1).includes(".expected")) return false;
  const fixtureDir = parts.slice(0, sightmapIdx).join(sep);
  const expectedJsonPath = join(fixtureDir, "expected.json");
  if (!existsSync(expectedJsonPath)) return false;
  let cases;
  try {
    cases = JSON.parse(readFileSync(expectedJsonPath, "utf8")).cases ?? [];
  } catch {
    return false;
  }
  for (const tc of cases) {
    const diags = tc?.expected?.diagnostics ?? [];
    for (const d of diags) {
      if (d?.code === "fmt.schema-invalid" || d?.code === "fmt.parse-error") {
        return true;
      }
    }
  }
  return false;
}

let failures = 0;
let checked = 0;
let skipped = 0;

for (const root of roots) {
  const absRoot = resolve(root);
  for (const file of collect(absRoot)) {
    const rel = relative(repoRoot, file);

    if (shouldSkipInputAsExpectedInvalid(file)) {
      skipped++;
      console.log(`skip  ${rel}  (fixture expects schema-invalid input)`);
      continue;
    }

    let doc;
    try {
      doc = yaml.load(readFileSync(file, "utf8"));
    } catch (err) {
      failures++;
      console.error(`FAIL  ${rel}  (YAML parse error)`);
      console.error(`      ${err.message}`);
      continue;
    }

    checked++;
    const ok = validate(doc);
    if (!ok) {
      failures++;
      console.error(`FAIL  ${rel}`);
      for (const e of validate.errors ?? []) {
        const where = e.instancePath || "(root)";
        console.error(`      ${where}  ${e.message}`);
      }
    } else {
      console.log(`ok    ${rel}`);
    }
  }
}

console.log("");
console.log(`${checked} file(s) checked, ${skipped} skipped, ${failures} failure(s).`);
process.exit(failures > 0 ? 1 : 0);
