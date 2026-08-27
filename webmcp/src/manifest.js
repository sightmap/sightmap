// Tool-manifest loader — structural validation of a webmcp.tools.yaml.
// Cross-validation against the sightmap corpus (component/request/view
// resolution, template params, flow-shape rules) lives in compile.js.
//
// Philosophy mirrors `sightmap validate`: structural problems are errors;
// unknown fields are warnings (they usually mean a typo'd key).

const fs = require("fs");
const yaml = require("js-yaml");

const SITE_RE = /^[a-z0-9][a-z0-9-]*$/;
const TOOL_NAME_RE = /^[a-z][a-z0-9_-]{0,63}$/;
const PARAM_NAME_RE = /^[a-z][a-z0-9_]*$/;
const PARAM_TYPES = ["string", "number", "integer", "boolean"];
const STEP_ACTIONS = [
  "navigate",
  "wait_for",
  "fill",
  "click",
  "press",
  "sleep",
  "scroll",
  "read",
];
const STEP_OPTION_KEYS = {
  press: ["target"],
  navigate: [],
  wait_for: [],
  fill: [],
  click: [],
  sleep: [],
  scroll: [],
  read: [],
};
const ROOT_KEYS = [
  "version",
  "site",
  "base_url",
  "description",
  "tool_version",
  "match",
  "sightmap",
  "tools",
];
const TOOL_KEYS = [
  "name",
  "title",
  "description",
  "read_only",
  "require_view",
  "view",
  "params",
  "api",
  "flow",
  "mode",
];
const PARAM_KEYS = [
  "name",
  "type",
  "description",
  "required",
  "enum",
  "default",
];
const API_KEYS = [
  "request",
  "method",
  "url",
  "query",
  "headers",
  "body",
  "result",
  "max_body_chars",
];
const RESULT_KEYS = ["name", "source", "field", "pattern", "transform"];
const RESULT_SOURCES = ["req.body", "rsp.body", "req.headers", "rsp.headers"];

function warnUnknown(obj, known, where, warnings) {
  for (const k of Object.keys(obj)) {
    if (!known.includes(k)) warnings.push(`${where}: unknown field "${k}"`);
  }
}

function validateParams(params, where, errors, warnings) {
  if (!Array.isArray(params)) {
    errors.push(`${where}: "params" must be a list`);
    return;
  }
  const seen = new Set();
  for (const p of params) {
    if (!p || typeof p !== "object") {
      errors.push(`${where}: each param must be a mapping`);
      continue;
    }
    warnUnknown(p, PARAM_KEYS, `${where} param "${p.name || "?"}"`, warnings);
    if (!p.name || !PARAM_NAME_RE.test(p.name)) {
      errors.push(
        `${where}: param name "${p.name}" must match ${PARAM_NAME_RE}`,
      );
    } else if (seen.has(p.name)) {
      errors.push(`${where}: duplicate param "${p.name}"`);
    } else {
      seen.add(p.name);
    }
    if (!p.type || !PARAM_TYPES.includes(p.type)) {
      errors.push(
        `${where} param "${p.name}": type must be one of ${PARAM_TYPES.join("|")}`,
      );
    }
    if (!p.description) {
      warnings.push(
        `${where} param "${p.name}": missing description (agents rely on it)`,
      );
    }
  }
}

function validateApi(api, where, errors, warnings) {
  if (!api || typeof api !== "object") {
    errors.push(`${where}: "api" must be a mapping`);
    return;
  }
  warnUnknown(api, API_KEYS, where, warnings);
  if (!api.url && !api.request) {
    errors.push(
      `${where}: api needs a "url" template (or a corpus "request" whose route is literal)`,
    );
  }
  if (api.method && !/^[A-Z]+$/.test(api.method)) {
    errors.push(
      `${where}: api method "${api.method}" must be an upper-case HTTP method`,
    );
  }
  for (const r of api.result || []) {
    const rw = `${where} result "${r && r.name}"`;
    if (!r || typeof r !== "object") {
      errors.push(`${where}: each result entry must be a mapping`);
      continue;
    }
    warnUnknown(r, RESULT_KEYS, rw, warnings);
    if (!r.name || !PARAM_NAME_RE.test(r.name))
      errors.push(`${rw}: name must match ${PARAM_NAME_RE}`);
    if (!RESULT_SOURCES.includes(r.source))
      errors.push(`${rw}: source must be one of ${RESULT_SOURCES.join("|")}`);
    if (!r.field && !r.pattern)
      errors.push(`${rw}: needs "field" and/or "pattern"`);
    if (
      (r.source === "req.headers" || r.source === "rsp.headers") &&
      !r.field
    ) {
      errors.push(`${rw}: a headers source requires "field"`);
    }
    if (r.pattern) {
      try {
        new RegExp(r.pattern);
      } catch (e) {
        errors.push(`${rw}: invalid pattern: ${e.message}`);
      }
    }
  }
}

function stepAction(step) {
  const keys = Object.keys(step).filter((k) => STEP_ACTIONS.includes(k));
  return keys.length === 1 ? keys[0] : null;
}

function validateFlow(flow, mode, where, errors, warnings) {
  if (!Array.isArray(flow) || flow.length === 0) {
    errors.push(`${where}: "flow" must be a non-empty list of steps`);
    return;
  }
  flow.forEach((step, i) => {
    const sw = `${where} step ${i + 1}`;
    if (!step || typeof step !== "object") {
      errors.push(`${sw}: each step must be a mapping with one action key`);
      return;
    }
    const action = stepAction(step);
    if (!action) {
      errors.push(
        `${sw}: expected exactly one action key (${STEP_ACTIONS.join("|")}), got: ${Object.keys(step).join(", ")}`,
      );
      return;
    }
    warnUnknown(
      step,
      [action, ...(STEP_OPTION_KEYS[action] || [])],
      sw,
      warnings,
    );
    if (action === "navigate") {
      if (mode === "fetch" && i !== 0) {
        errors.push(`${sw}: in a fetch flow "navigate" must be the first step`);
      }
      if (mode === "live" && i !== flow.length - 1) {
        errors.push(
          `${sw}: in a live flow "navigate" is only allowed as the final step — ` +
            `a WebMCP tool call dies with the document it runs in, so a mid-flow ` +
            `navigation can never return a result. Split the tool, or use "mode: fetch" for read-only flows.`,
        );
      }
    }
    if (mode === "fetch" && action !== "navigate" && action !== "read") {
      errors.push(
        `${sw}: a fetch flow only supports "navigate" and "read" (it runs on a detached fetched document)`,
      );
    }
    if (action === "wait_for") {
      const w = step.wait_for || {};
      if (!w.component && !w.selector && !w.url_includes) {
        errors.push(
          `${sw}: wait_for needs one of component|selector|url_includes`,
        );
      }
    }
    if (action === "fill") {
      const f = step.fill || {};
      if (!f.target || f.value == null)
        errors.push(`${sw}: fill needs "target" and "value"`);
    }
    if (
      action === "read" &&
      (typeof step.read !== "object" || step.read == null)
    ) {
      errors.push(
        `${sw}: read must be a mapping of result keys to value specs`,
      );
    }
  });
  if (mode === "fetch" && stepAction(flow[0] || {}) !== "navigate") {
    errors.push(`${where}: a fetch flow must start with "navigate"`);
  }
}

function validateManifest(doc, errors, warnings) {
  if (!doc || typeof doc !== "object") {
    errors.push("manifest is empty or not a mapping");
    return;
  }
  warnUnknown(doc, ROOT_KEYS, "manifest", warnings);
  if (doc.version !== 1) errors.push('manifest must declare "version: 1"');
  if (!doc.site || !SITE_RE.test(doc.site)) {
    errors.push(
      `"site" must be a slug matching ${SITE_RE} (got "${doc.site}")`,
    );
  }
  if (!doc.base_url || !/^https?:\/\//.test(doc.base_url)) {
    errors.push(
      `"base_url" must be an absolute http(s) URL (got "${doc.base_url}")`,
    );
  }
  if (!Array.isArray(doc.tools) || doc.tools.length === 0) {
    errors.push('"tools" must be a non-empty list');
    return;
  }
  const seen = new Set();
  doc.tools.forEach((tool, i) => {
    const where = `tool ${i + 1} ("${(tool && tool.name) || "?"}")`;
    if (!tool || typeof tool !== "object") {
      errors.push(`${where}: must be a mapping`);
      return;
    }
    warnUnknown(tool, TOOL_KEYS, where, warnings);
    if (!tool.name || !TOOL_NAME_RE.test(tool.name)) {
      errors.push(`${where}: "name" must match ${TOOL_NAME_RE}`);
    } else if (seen.has(tool.name)) {
      errors.push(`${where}: duplicate tool name`);
    } else {
      seen.add(tool.name);
    }
    if (!tool.description || String(tool.description).trim() === "") {
      errors.push(
        `${where}: "description" is required — it is what the agent decides by`,
      );
    }
    if (tool.params != null)
      validateParams(tool.params, where, errors, warnings);
    const hasApi = tool.api != null;
    const hasFlow = tool.flow != null;
    if (hasApi === hasFlow) {
      errors.push(`${where}: exactly one of "api" or "flow" is required`);
      return;
    }
    if (tool.mode != null && !hasFlow)
      warnings.push(`${where}: "mode" only applies to flow tools`);
    if (tool.mode != null && !["live", "fetch"].includes(tool.mode)) {
      errors.push(`${where}: "mode" must be live|fetch`);
    }
    if (
      tool.require_view != null &&
      (!hasFlow || (tool.mode || "live") !== "live")
    ) {
      warnings.push(`${where}: "require_view" only applies to live flow tools`);
    }
    if (tool.view != null && !hasFlow) {
      warnings.push(
        `${where}: "view" only affects flow tools (it scopes component resolution)`,
      );
    }
    if (hasApi) validateApi(tool.api, where, errors, warnings);
    if (hasFlow)
      validateFlow(tool.flow, tool.mode || "live", where, errors, warnings);
  });
}

function loadManifest(file) {
  const errors = [];
  const warnings = [];
  let doc = null;
  try {
    doc = yaml.load(fs.readFileSync(file, "utf8"));
  } catch (e) {
    errors.push(`${file}: ${e.message}`);
    return { manifest: null, errors, warnings };
  }
  validateManifest(doc, errors, warnings);
  return { manifest: errors.length === 0 ? doc : doc, errors, warnings };
}

module.exports = { loadManifest, validateManifest };
