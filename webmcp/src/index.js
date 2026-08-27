// Public entry point for @sightmap/webmcp-codegen — the sightmap → WebMCP
// codegen adapter. See webmcp/README.md for the pipeline.

const { loadCorpus } = require("./corpus");
const { loadManifest, validateManifest } = require("./manifest");
const { compile } = require("./compile");
const { emit, FORMATS, defaultFileName, corpusHash } = require("./emit");
const { parseQuery } = require("./query");
const { routeGlobToRegex, pathMatchesRoute } = require("./globs");
const { scaffold } = require("./scaffold");

module.exports = {
  loadCorpus,
  loadManifest,
  validateManifest,
  compile,
  emit,
  FORMATS,
  defaultFileName,
  corpusHash,
  parseQuery,
  routeGlobToRegex,
  pathMatchesRoute,
  scaffold,
};
