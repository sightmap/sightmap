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
  "credentials",
  "rows",
];
const ROWS_KEYS = ["field", "max", "fields"];
const ROW_FIELD_KEYS = ["field", "template"];
const CREDENTIALS_MODES = ["include", "same-origin", "omit"];
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
    if (
      p.enum != null &&
      (!Array.isArray(p.enum) ||
        p.enum.length === 0 ||
        p.enum.some((v) => typeof v === "object"))
    ) {
      errors.push(
        `${where} param "${p.name}": enum must be a non-empty list of scalars`,
      );
    }
    if (p.default != null && typeof p.default === "object") {
      errors.push(`${where} param "${p.name}": default must be a scalar`);
    }
  }
}

function validateApiRows(rows, where, errors, warnings) {
  if (typeof rows !== "object" || Array.isArray(rows)) {
    errors.push(`${where}: api "rows" must be a mapping`);
    return;
  }
  warnUnknown(rows, ROWS_KEYS, `${where} rows`, warnings);
  if (rows.field != null && typeof rows.field !== "string") {
    errors.push(`${where}: rows "field" must be a dot path into the response`);
  }
  if (
    !rows.fields ||
    typeof rows.fields !== "object" ||
    Array.isArray(rows.fields) ||
    Object.keys(rows.fields).length === 0
  ) {
    errors.push(`${where}: rows needs a non-empty "fields" mapping`);
    return;
  }
  for (const name of Object.keys(rows.fields)) {
    const f = rows.fields[name];
    if (!f || typeof f !== "object" || Array.isArray(f)) {
      errors.push(`${where}: rows field "${name}" must be a mapping`);
      continue;
    }
    warnUnknown(f, ROW_FIELD_KEYS, `${where} rows.${name}`, warnings);
    if (f.field == null && f.template == null) {
      errors.push(
        `${where}: rows field "${name}" needs a "field" path or a "template"`,
      );
    }
    if (f.field != null && f.template != null) {
      errors.push(
        `${where}: rows field "${name}" sets both "field" and "template" — pick one`,
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
  if (api.result != null && !Array.isArray(api.result)) {
    errors.push(`${where}: api "result" must be a list of extractions`);
    api.result = [];
  }
  for (const key of ["query", "headers"]) {
    const v = api[key];
    if (v == null) continue;
    if (typeof v !== "object" || Array.isArray(v)) {
      errors.push(
        `${where}: api "${key}" must be a mapping of names to string templates`,
      );
    } else {
      for (const [k2, v2] of Object.entries(v)) {
        if (
          typeof v2 !== "string" &&
          typeof v2 !== "number" &&
          typeof v2 !== "boolean"
        ) {
          errors.push(`${where}: api ${key}.${k2} must be a scalar template`);
        }
      }
    }
  }
  if (
    api.body != null &&
    typeof api.body !== "object" &&
    typeof api.body !== "string"
  ) {
    errors.push(
      `${where}: api "body" must be a mapping (JSON body) or a string (raw body)`,
    );
  }
  if (api.rows != null) validateApiRows(api.rows, where, errors, warnings);
  if (
    api.credentials != null &&
    !CREDENTIALS_MODES.includes(api.credentials)
  ) {
    errors.push(
      `${where}: api "credentials" must be one of ${CREDENTIALS_MODES.join(", ")}`,
    );
  }
  if (
    api.max_body_chars != null &&
    (!Number.isInteger(api.max_body_chars) || api.max_body_chars < 1)
  ) {
    errors.push(`${where}: "max_body_chars" must be a positive integer`);
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
      const WAIT_KEYS = [
        "component",
        "selector",
        "url_includes",
        "timeout_ms",
        "poll_ms",
      ];
      for (const k of Object.keys(w)) {
        if (!WAIT_KEYS.includes(k)) {
          warnings.push(`${sw}: unknown wait_for key "${k}"`);
        }
      }
      if (!w.component && !w.selector && !w.url_includes) {
        errors.push(
          `${sw}: wait_for needs one of component|selector|url_includes`,
        );
      }
      for (const k of ["timeout_ms", "poll_ms"]) {
        if (w[k] != null && (!Number.isInteger(w[k]) || w[k] < 1)) {
          errors.push(
            `${sw}: "${k}" must be a positive integer (milliseconds)`,
          );
        }
      }
    }
    if (
      action === "sleep" &&
      (!Number.isInteger(step.sleep) || step.sleep < 1)
    ) {
      errors.push(`${sw}: sleep must be a positive integer (milliseconds)`);
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
  if (
    doc.match != null &&
    (!Array.isArray(doc.match) || doc.match.some((v) => typeof v !== "string"))
  ) {
    errors.push('"match" must be a list of URL match patterns (strings)');
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
