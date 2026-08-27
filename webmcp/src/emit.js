// Emitter — turns compiled IR into a self-contained JS bundle in one of three
// formats:
//
//   snippet     plain IIFE. Inject into a live session for verification
//               (`sightmap browser eval "$(cat site.webmcp.js)"`), or serve
//               with a <script> tag.
//   module      the same body as an ES module, for site owners
//               (<script type="module" src="...">).
//   userscript  Tampermonkey/Violentmonkey header + the IIFE, for publishing
//               tools for a third-party site you don't control.
//
// Output is deterministic for a given corpus + manifest (no timestamps): the
// provenance line carries a content hash instead, so committed bundles can be
// drift-checked in CI.

const fs = require("fs");
const path = require("path");
const crypto = require("crypto");

const RUNTIME_PATH = path.join(__dirname, "runtime", "runtime.js");

function runtimeSource() {
  // Generated bundles carry the runtime up to the [emit-strip-below] marker;
  // the CommonJS export guard after it exists only for the jest tests.
  const src = fs.readFileSync(RUNTIME_PATH, "utf8");
  const cut = src.indexOf("// [emit-strip-below]");
  return cut === -1 ? src : src.slice(0, cut);
}

function corpusHash(corpusDir, files) {
  const h = crypto.createHash("sha256");
  for (const f of files) {
    h.update(path.relative(corpusDir, f));
    h.update("\n");
    h.update(fs.readFileSync(f));
  }
  return h.digest("hex");
}

function banner(ir, provenance) {
  const lines = [];
  lines.push(
    `// ${ir.meta.site} — WebMCP tools generated from a sightmap corpus.`,
  );
  if (ir.meta.description) lines.push(`// ${oneLine(ir.meta.description)}`);
  lines.push(
    `// tools: ${ir.tools.map((t) => t.name).join(", ")}`,
    `// generator: @sightmap/webmcp-codegen v${provenance.generatorVersion} (github.com/sightmap/sightmap webmcp/)`,
    `// manifest: ${provenance.manifest} | corpus: ${provenance.corpus} (${provenance.corpusFiles} files, sha256:${provenance.corpusHash.slice(0, 12)})`,
    `// DO NOT EDIT — edit the manifest or corpus and regenerate: sightmap-webmcp generate`,
  );
  return lines.join("\n");
}

// oneLine collapses whitespace (newlines included) so authored text cannot
// break out of a // comment line or a userscript header field.
function oneLine(s) {
  return String(s).replace(/\s+/g, " ").trim();
}

function userscriptHeader(ir) {
  const origin = new URL(ir.meta.baseUrl).origin;
  const matches =
    ir.meta.match && ir.meta.match.length > 0 ? ir.meta.match : [`${origin}/*`];
  const lines = [
    "// ==UserScript==",
    `// @name         ${ir.meta.site} WebMCP tools (sightmap)`,
    "// @namespace    https://sightmap.org/webmcp",
    `// @version      ${ir.meta.toolVersion}`,
    `// @description  ${oneLine(ir.meta.description || `WebMCP tools for ${ir.meta.site}, generated from its sightmap corpus.`)}`,
  ];
  for (const m of matches) lines.push(`// @match        ${m}`);
  lines.push(
    "// @run-at       document-idle",
    "// @grant        none",
    "// @noframes",
    "// ==/UserScript==",
  );
  return lines.join("\n");
}

function bundleBody(ir) {
  const meta = JSON.stringify(ir.meta, null, 2).replace(/<\//g, "<\\/");
  const tools = JSON.stringify(ir.tools, null, 2).replace(/<\//g, "<\\/");
  return [
    "(() => {",
    '  "use strict";',
    "",
    runtimeSource().trimEnd(),
    "",
    `const __SMW_META = ${meta};`,
    "",
    `const __SMW_TOOLS = ${tools};`,
    "",
    "__smwBoot(__SMW_META, __SMW_TOOLS);",
    "})();",
  ].join("\n");
}

// emit renders one format. `provenance` comes from the CLI (paths relative to
// the output directory, corpus hash, generator version).
function emit(ir, format, provenance) {
  const body = bundleBody(ir);
  const head = banner(ir, provenance);
  if (format === "snippet") return `${head}\n${body}\n`;
  if (format === "module") return `${head}\n${body}\nexport {};\n`;
  if (format === "userscript")
    return `${userscriptHeader(ir)}\n${head}\n${body}\n`;
  throw new Error(`unknown format "${format}" (snippet|module|userscript)`);
}

const FORMATS = ["snippet", "module", "userscript"];

function defaultFileName(site, format) {
  if (format === "snippet") return `${site}.webmcp.js`;
  if (format === "module") return `${site}.webmcp.module.js`;
  return `${site}.webmcp.user.js`;
}

module.exports = { emit, FORMATS, defaultFileName, corpusHash, runtimeSource };
