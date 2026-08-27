#!/usr/bin/env node
// sightmap-webmcp — CLI for the sightmap → WebMCP codegen adapter.
//
//   sightmap-webmcp validate  [--tools FILE] [--sightmap-dir DIR]
//   sightmap-webmcp generate  [--tools FILE] [--sightmap-dir DIR]
//                             [--format snippet|module|userscript|all]
//                             [--out FILE | --out-dir DIR] [--check]
//   sightmap-webmcp init      --site SLUG --base-url URL
//                             [--sightmap-dir DIR] [--out FILE]
//
// The tools manifest defaults to ./webmcp.tools.yaml; the corpus defaults to
// the manifest's `sightmap:` field (relative to the manifest), then to a
// .sightmap/ directory next to the manifest.

const fs = require("fs");
const path = require("path");
const { loadCorpus } = require("../src/corpus");
const { loadManifest } = require("../src/manifest");
const { compile } = require("../src/compile");
const { emit, FORMATS, defaultFileName, corpusHash } = require("../src/emit");
const { scaffold } = require("../src/scaffold");

const GENERATOR_VERSION = require("../package.json").version;

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith("--")) {
      const key = a.slice(2);
      if (key === "check") args.check = true;
      else args[key] = argv[++i];
    } else {
      args._.push(a);
    }
  }
  return args;
}

function fail(msg) {
  process.stderr.write(`error: ${msg}\n`);
  process.exit(1);
}

function resolveInputs(args) {
  const toolsFile = path.resolve(args.tools || "webmcp.tools.yaml");
  if (!fs.existsSync(toolsFile)) {
    fail(
      `tools manifest not found: ${toolsFile} (pass --tools, or run "sightmap-webmcp init" to scaffold one)`,
    );
  }
  const { manifest, errors, warnings } = loadManifest(toolsFile);
  for (const w of warnings) process.stderr.write(`warning: ${w}\n`);
  if (errors.length > 0) {
    for (const e of errors) process.stderr.write(`error: ${e}\n`);
    process.exit(1);
  }
  let sightmapDir;
  if (args["sightmap-dir"]) sightmapDir = path.resolve(args["sightmap-dir"]);
  else if (manifest.sightmap)
    sightmapDir = path.resolve(path.dirname(toolsFile), manifest.sightmap);
  else sightmapDir = path.join(path.dirname(toolsFile), ".sightmap");
  let corpus;
  try {
    corpus = loadCorpus(sightmapDir);
  } catch (e) {
    fail(e.message);
  }
  return { toolsFile, manifest, corpus };
}

function compileOrExit(manifest, corpus) {
  const { ir, errors, warnings } = compile(corpus, manifest);
  for (const w of warnings) process.stderr.write(`warning: ${w}\n`);
  if (errors.length > 0) {
    for (const e of errors) process.stderr.write(`error: ${e}\n`);
    process.exit(1);
  }
  return ir;
}

function cmdValidate(args) {
  const { manifest, corpus } = resolveInputs(args);
  const ir = compileOrExit(manifest, corpus);
  process.stdout.write(
    `✓ ${ir.tools.length} tool(s) compile against ${corpus.files.length} corpus file(s): ${ir.tools.map((t) => t.name).join(", ")}\n`,
  );
}

function cmdGenerate(args) {
  const { toolsFile, manifest, corpus } = resolveInputs(args);
  const ir = compileOrExit(manifest, corpus);

  const formatArg = args.format || "snippet";
  const formats = formatArg === "all" ? FORMATS : [formatArg];
  for (const f of formats) {
    if (!FORMATS.includes(f))
      fail(`unknown format "${f}" (snippet|module|userscript|all)`);
  }
  if (args.out && formats.length > 1)
    fail("--out only works with a single --format; use --out-dir for several");
  const outDir = path.resolve(
    args["out-dir"] ||
      path.dirname(args.out ? path.resolve(args.out) : toolsFile),
  );

  let drift = false;
  for (const format of formats) {
    const outFile = args.out
      ? path.resolve(args.out)
      : path.join(outDir, defaultFileName(ir.meta.site, format));
    const provenance = {
      generatorVersion: GENERATOR_VERSION,
      manifest:
        path.relative(path.dirname(outFile), toolsFile) ||
        path.basename(toolsFile),
      corpus: path.relative(path.dirname(outFile), corpus.dir),
      corpusFiles: corpus.files.length,
      corpusHash: corpusHash(corpus.dir, corpus.files),
    };
    const content = emit(ir, format, provenance);
    if (args.check) {
      const existing = fs.existsSync(outFile)
        ? fs.readFileSync(outFile, "utf8")
        : null;
      if (existing !== content) {
        process.stderr.write(
          `drift: ${outFile} is stale — regenerate with sightmap-webmcp generate\n`,
        );
        drift = true;
      } else {
        process.stdout.write(`✓ ${outFile} is up to date\n`);
      }
    } else {
      fs.mkdirSync(path.dirname(outFile), { recursive: true });
      fs.writeFileSync(outFile, content);
      process.stdout.write(
        `wrote ${outFile} (${content.length} bytes, ${ir.tools.length} tools)\n`,
      );
    }
  }
  if (drift) process.exit(2);
}

function cmdInit(args) {
  const site = args.site;
  const baseUrl = args["base-url"];
  if (!site || !baseUrl) fail("init needs --site SLUG and --base-url URL");
  const sightmapDir = path.resolve(args["sightmap-dir"] || ".sightmap");
  let corpus;
  try {
    corpus = loadCorpus(sightmapDir);
  } catch (e) {
    fail(e.message);
  }
  const outFile = path.resolve(args.out || "webmcp.tools.yaml");
  if (fs.existsSync(outFile))
    fail(
      `${outFile} already exists — delete it first if you meant to re-scaffold`,
    );
  fs.writeFileSync(outFile, scaffold(corpus, { site, baseUrl }));
  process.stdout.write(
    `wrote ${outFile} — a draft; see the sightmap-webmcp skill for the authoring loop\n`,
  );
}

function main() {
  const argv = process.argv.slice(2);
  const cmd = argv[0];
  const args = parseArgs(argv.slice(1));
  if (cmd === "validate") return cmdValidate(args);
  if (cmd === "generate") return cmdGenerate(args);
  if (cmd === "init") return cmdInit(args);
  process.stderr.write(
    "usage: sightmap-webmcp <validate|generate|init> [options]\n" +
      "  validate  [--tools FILE] [--sightmap-dir DIR]\n" +
      "  generate  [--tools FILE] [--sightmap-dir DIR] [--format snippet|module|userscript|all]\n" +
      "            [--out FILE | --out-dir DIR] [--check]\n" +
      "  init      --site SLUG --base-url URL [--sightmap-dir DIR] [--out FILE]\n",
  );
  process.exit(cmd ? 1 : 0);
}

main();
