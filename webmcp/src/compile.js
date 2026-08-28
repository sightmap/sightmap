// Compiler — resolves a validated tool manifest against a loaded sightmap
// corpus and produces the self-contained IR the emitter embeds into the
// generated bundle. All corpus knowledge (selector chains, property
// extractors, request routes, view routes) is resolved here, at generate
// time, so the browser runtime stays a dumb interpreter over inlined data.

const { parseQuery } = require("./query");
const { routeGlobToRegex, pathMatchesRoute } = require("./globs");

// ---------------------------------------------------------------------------
// Template handling: "{param}" interpolation with "{{"/"}}" escapes and an
// optional "|raw" modifier (skip URL-encoding). Returns referenced names.

const TEMPLATE_RE = /\{([a-z][a-z0-9_]*)(\|raw)?\}/g;

function templateRefs(str) {
  const refs = [];
  const s = String(str).replace(/\{\{|\}\}/g, "");
  let m;
  while ((m = TEMPLATE_RE.exec(s)) !== null) refs.push(m[1]);
  return refs;
}

// ---------------------------------------------------------------------------
// Component resolution

// entriesFor filters a breadcrumb's definitions by view scope: a global
// entry (scope null) applies everywhere; a view-scoped entry applies only
// under its view. When both a view-scoped and a global definition survive,
// the view-scoped one wins — it is what the author means on that view.
function entriesFor(corpus, crumb, viewScope) {
  const all = corpus.components.get(crumb) || [];
  const applicable = all.filter(
    (e) => e.scope == null || e.scope === viewScope,
  );
  const scoped = applicable.filter((e) => e.scope != null);
  return scoped.length > 0 ? scoped : applicable;
}

// resolveName finds the single corpus entry a query-link name refers to,
// preferring descendants of the previous link's breadcrumb.
function resolveName(corpus, name, prevEntry, viewScope) {
  const crumbs = (corpus.byName.get(name) || []).filter(
    (c) => entriesFor(corpus, c, viewScope).length > 0,
  );
  if (crumbs.length === 0) {
    // The name may itself be a breadcrumb-qualified path ("Parent Child").
    if (entriesFor(corpus, name, viewScope).length > 0) {
      return pickEntry(corpus, name, prevEntry, viewScope);
    }
    throw new Error(
      `no component named "${name}" in the corpus` +
        (viewScope ? ` (in view ${viewScope} or globals)` : "") +
        (corpus.byName.has(name)
          ? ` — it exists only view-scoped; set the tool's "view:"`
          : ""),
    );
  }
  let candidates = crumbs;
  if (prevEntry) {
    const under = crumbs.filter((c) => c.startsWith(prevEntry.crumb + " "));
    if (under.length > 0) candidates = under;
  }
  if (candidates.length > 1) {
    throw new Error(
      `component name "${name}" is ambiguous — qualify it with its parent. Candidates: ${candidates.join(", ")}`,
    );
  }
  return pickEntry(corpus, candidates[0], prevEntry, viewScope);
}

function pickEntry(corpus, crumb, prevEntry, viewScope) {
  const defs = entriesFor(corpus, crumb, viewScope);
  if (defs.length === 0) throw new Error(`no component at "${crumb}"`);
  if (defs.length > 1) {
    const scopes = defs.map((d) => d.scope || "global").join(", ");
    throw new Error(
      `"${crumb}" has ${defs.length} definitions (${scopes}) — set the tool's "view:" to pick the one that applies`,
    );
  }
  const def = defs[0];
  // Relative chain: when this entry is authored under the previous link's
  // breadcrumb, drop the shared prefix; otherwise use the full chain, which
  // the runtime evaluates inside the previous element's subtree (matching the
  // CLI's DOM-containment semantics).
  let levels = def.chainLevels;
  if (prevEntry && def.path.startsWith(prevEntry.crumb + " ")) {
    levels = def.chainLevels.slice(prevEntry.levelsLen);
  }
  return {
    crumb: def.path,
    levels,
    levelsLen: def.chainLevels.length,
    props: def.props,
  };
}

// resolveTarget compiles a component reference (name, query, or css: escape)
// into a runtime TargetIR. Returns { ir, lastEntry } — lastEntry carries the
// corpus entry of the final link for property lookups and relative reads.
function resolveTarget(corpus, ref, scopeEntry, viewScope) {
  const parsed = parseQuery(ref);
  if (parsed.kind === "css") {
    return { ir: { kind: "css", selector: parsed.selector }, lastEntry: null };
  }
  const links = [];
  let prev = scopeEntry || null;
  for (const link of parsed.links) {
    const entry = resolveName(corpus, link.name, prev, viewScope);
    for (const pred of link.preds) {
      if (!entry.props[pred.prop]) {
        const have = Object.keys(entry.props);
        throw new Error(
          `"${entry.crumb}" has no property "${pred.prop}"${have.length ? ` (properties: ${have.join(", ")})` : " (it declares no properties)"}`,
        );
      }
    }
    links.push({
      name: entry.crumb,
      chain: entry.levels,
      props: entry.props,
      preds: link.preds,
      index: link.index,
    });
    prev = entry;
  }
  return { ir: { kind: "chain", links }, lastEntry: prev };
}

// ---------------------------------------------------------------------------
// Read specs

function compileValueSpec(corpus, spec, where, scopeEntry, refs, viewScope) {
  if (spec == null || typeof spec !== "object") {
    throw new Error(`${where}: a read value must be a mapping`);
  }
  if (spec.for_each) {
    for (const k of Object.keys(spec)) {
      if (!["for_each", "max", "fields"].includes(k)) {
        throw new Error(
          `${where}: unknown key "${k}" in a for_each spec (for_each, max, fields)`,
        );
      }
    }
    const { ir, lastEntry } = resolveTarget(
      corpus,
      spec.for_each,
      scopeEntry,
      viewScope,
    );
    collectPredRefs(ir, refs);
    if (!spec.fields || typeof spec.fields !== "object") {
      throw new Error(`${where}: for_each needs a "fields" mapping`);
    }
    const fields = {};
    for (const [key, fspec] of Object.entries(spec.fields)) {
      fields[key] = compileFieldSpec(
        corpus,
        fspec,
        `${where}.fields.${key}`,
        lastEntry,
        refs,
        viewScope,
      );
    }
    let max = null;
    if (spec.max != null) {
      if (typeof spec.max === "number") max = spec.max;
      else {
        max = String(spec.max);
        for (const r of templateRefs(max)) refs.add(r);
      }
    }
    return { list: { target: ir, max, fields } };
  }
  return {
    one: compileFieldSpec(corpus, spec, where, scopeEntry, refs, viewScope),
  };
}

const FIELD_SPEC_KEYS = [
  "component",
  "selector",
  "extract",
  "transform",
  "property",
  "max_chars",
];

function compileFieldSpec(corpus, spec, where, scopeEntry, refs, viewScope) {
  if (spec == null || typeof spec !== "object") {
    throw new Error(`${where}: must be a mapping`);
  }
  for (const k of Object.keys(spec)) {
    if (!FIELD_SPEC_KEYS.includes(k)) {
      throw new Error(
        `${where}: unknown key "${k}" in a read value spec (${FIELD_SPEC_KEYS.join(", ")})`,
      );
    }
  }
  let target = null;
  let lastEntry = scopeEntry || null;
  if (spec.component) {
    const resolved = resolveTarget(
      corpus,
      spec.component,
      scopeEntry,
      viewScope,
    );
    target = resolved.ir;
    lastEntry = resolved.lastEntry;
    collectPredRefs(target, refs);
  } else if (spec.selector) {
    target = { kind: "css", selector: String(spec.selector) };
    collectPredRefs(target, refs);
    lastEntry = null;
  }
  let extract = spec.extract ? String(spec.extract) : null;
  let transform = spec.transform ? String(spec.transform) : null;
  if (spec.property) {
    if (extract)
      throw new Error(`${where}: give "property" or "extract", not both`);
    if (!lastEntry) {
      throw new Error(
        `${where}: "property" lookup needs a component target (got a raw selector)`,
      );
    }
    const prop = lastEntry.props[spec.property];
    if (!prop) {
      const have = Object.keys(lastEntry.props);
      throw new Error(
        `${where}: "${lastEntry.crumb}" has no property "${spec.property}"${have.length ? ` (properties: ${have.join(", ")})` : ""}`,
      );
    }
    extract = prop.extract;
    if (!transform && prop.transform) transform = prop.transform;
  }
  if (!extract) extract = "text";
  const out = { extract };
  if (target) out.target = target;
  if (transform) out.transform = transform;
  if (spec.max_chars != null) {
    const cap = Number(spec.max_chars);
    if (!Number.isInteger(cap) || cap < 1) {
      throw new Error(`${where}: "max_chars" must be a positive integer`);
    }
    out.cap = cap;
  }
  return out;
}

function collectPredRefs(targetIR, refs) {
  if (targetIR.kind === "css") {
    for (const r of templateRefs(targetIR.selector)) refs.add(r);
    return;
  }
  if (targetIR.kind !== "chain") return;
  for (const link of targetIR.links) {
    for (const pred of link.preds) {
      for (const r of templateRefs(pred.value)) refs.add(r);
    }
  }
}

// ---------------------------------------------------------------------------
// Tools

function compileParams(tool) {
  const properties = {};
  const required = [];
  for (const p of tool.params || []) {
    const prop = { type: p.type, description: p.description || "" };
    if (p.enum) prop.enum = p.enum;
    if (p.default !== undefined) prop.default = p.default;
    properties[p.name] = prop;
    if (p.required) required.push(p.name);
  }
  const schema = { type: "object", properties };
  if (required.length > 0) schema.required = required;
  return schema;
}

function paramNames(tool) {
  return new Set((tool.params || []).map((p) => p.name));
}

function addStringRefs(value, refs) {
  if (typeof value === "string") {
    for (const r of templateRefs(value)) refs.add(r);
  } else if (Array.isArray(value)) {
    for (const v of value) addStringRefs(v, refs);
  } else if (value && typeof value === "object") {
    for (const v of Object.values(value)) addStringRefs(v, refs);
  }
}

// query/headers values are either scalar templates or page-value mappings; the
// mappings are emitted in a fixed key order so the Go port can match byte for
// byte.
function normalizeValueMap(m) {
  if (!m) return null;
  const out = {};
  for (const k of Object.keys(m)) {
    const v = m[k];
    if (v != null && typeof v === "object" && !Array.isArray(v)) {
      out[k] = {
        from: v.from,
        key: v.key != null ? String(v.key) : null,
        selector: v.selector != null ? String(v.selector) : null,
        attr: v.attr != null ? String(v.attr) : null,
        json: v.json != null ? String(v.json) : null,
        prefix: v.prefix != null ? String(v.prefix) : null,
        optional: v.optional === true ? true : null,
      };
    } else {
      out[k] = v;
    }
  }
  return out;
}

function usesPageValues(api) {
  for (const key of ["query", "headers"]) {
    const m = api[key];
    if (!m) continue;
    for (const k of Object.keys(m)) {
      const v = m[k];
      if (v != null && typeof v === "object" && !Array.isArray(v) && v.from) {
        return true;
      }
    }
  }
  return false;
}

function compileApiTool(corpus, tool, errors, warnings) {
  const api = tool.api;
  const refs = new Set();
  let method = api.method || null;
  let requestName = null;
  if (api.request) {
    const req = corpus.requests.get(api.request);
    if (!req) {
      errors.push(
        `tool "${tool.name}": no corpus request named "${api.request}" (requests: ${[...corpus.requests.keys()].join(", ") || "none"})`,
      );
      return null;
    }
    requestName = req.name;
    if (!method) method = req.method || "GET";
    // Sanity: the url template should land on the request's route glob.
    if (api.url && req.route) {
      try {
        const probe = new URL(
          String(api.url).replace(TEMPLATE_RE, "x"),
          "https://x.invalid",
        );
        if (!pathMatchesRoute(probe.pathname, req.route)) {
          warnings.push(
            `tool "${tool.name}": url path "${probe.pathname}" does not match corpus request "${req.name}" route "${req.route}"`,
          );
        }
      } catch (_) {
        /* un-parseable template; runtime will surface it */
      }
    }
    // Inherit the corpus request's properties as the result spec when the
    // manifest doesn't give one — the corpus already says what live traffic
    // means.
    if (
      !api.result &&
      Array.isArray(req.properties) &&
      req.properties.length > 0
    ) {
      api.result = req.properties.map((p) => ({
        name: p.name,
        source: p.source,
        field: p.field,
        pattern: p.pattern,
        transform: p.transform,
      }));
    }
  }
  if (!method) method = "GET";
  if (!api.url && requestName) {
    // A corpus request whose route is literal (no wildcards, no :params) IS
    // a usable URL — the fallback the validator promises.
    const route = corpus.requests.get(requestName).route || "";
    if (route && !route.includes("*") && !/\/:/g.test(route)) {
      api.url = route;
    }
  }
  if (!api.url) {
    errors.push(
      `tool "${tool.name}": api "url" template is required (the referenced request's route has wildcards, so it cannot serve as one)`,
    );
    return null;
  }
  if (
    requestName &&
    corpus.duplicateRequests &&
    corpus.duplicateRequests.has(requestName)
  ) {
    warnings.push(
      `tool "${tool.name}": the corpus defines more than one request named "${requestName}" — the first definition's route/method/properties are used`,
    );
  }
  addStringRefs(api.url, refs);
  {
    const u = String(api.url);
    const origin = u.match(/^https?:\/\/[^/]*/);
    const parameterizedOrigin =
      u.startsWith("{") || (origin && origin[0].includes("{"));
    if (parameterizedOrigin) {
      // A tool that reads the user's session out of the page and sends it to a
      // host the agent chose is a credential-exfiltration primitive, so this is
      // an error there rather than the usual warning.
      if (usesPageValues(api)) {
        errors.push(
          `tool "${tool.name}": the api url's origin is parameterized and the tool reads page state — an agent-supplied param would choose who receives the user's session; pin the origin`,
        );
      } else {
        warnings.push(
          `tool "${tool.name}": the api url's origin is parameterized — agent-supplied params choose the host that receives a credentialed request; prefer a fixed origin`,
        );
      }
    }
  }
  addStringRefs(api.query, refs);
  addStringRefs(api.headers, refs);
  addStringRefs(api.body, refs);

  const readOnly = tool.read_only != null ? !!tool.read_only : method === "GET";
  if (tool.read_only === true && method !== "GET") {
    warnings.push(
      `tool "${tool.name}": read_only: true on a ${method} request — double-check`,
    );
  }
  const ir = {
    kind: "api",
    api: {
      method,
      url: String(api.url),
      query: normalizeValueMap(api.query),
      headers: normalizeValueMap(api.headers),
      body: api.body != null ? api.body : null,
      result: (api.result || []).map((r) => ({
        name: r.name,
        source: r.source,
        field: r.field || null,
        pattern: r.pattern || null,
        transform: r.transform || null,
      })),
      rows: api.rows
        ? {
            field: api.rows.field || null,
            max: api.rows.max != null ? String(api.rows.max) : null,
            fields: Object.fromEntries(
              Object.keys(api.rows.fields).map((n) => [
                n,
                {
                  field: api.rows.fields[n].field || null,
                  template: api.rows.fields[n].template || null,
                },
              ]),
            ),
          }
        : null,
      maxBodyChars: api.max_body_chars || 20000,
      credentials: api.credentials || "include",
      request: requestName,
    },
  };
  return { ir, refs, readOnly };
}

function compileFlowTool(corpus, tool, errors, warnings) {
  const refs = new Set();
  const mode = tool.mode || "live";
  const viewScope = tool.view || tool.require_view || null;
  if (tool.view && !corpus.views.has(tool.view)) {
    errors.push(
      `tool "${tool.name}": no corpus view named "${tool.view}" (views: ${[...corpus.views.keys()].join(", ")})`,
    );
  }
  const steps = [];
  let hasRead = false;
  let requireView = null;

  if (tool.require_view) {
    const view = corpus.views.get(tool.require_view);
    if (!view) {
      errors.push(
        `tool "${tool.name}": no corpus view named "${tool.require_view}" (views: ${[...corpus.views.keys()].join(", ")})`,
      );
    } else {
      requireView = {
        view: view.name,
        route: view.route,
        pathRegex: routeGlobToRegex(view.route).source,
        url: view.url || null,
      };
    }
  }

  for (const [i, step] of tool.flow.entries()) {
    const where = `tool "${tool.name}" step ${i + 1}`;
    try {
      if (step.navigate != null) {
        const url = String(step.navigate);
        addStringRefs(url, refs);
        steps.push({ do: "navigate", url });
      } else if (step.wait_for != null) {
        const w = step.wait_for;
        const ir = {
          do: "wait_for",
          timeoutMs: w.timeout_ms || 10000,
          pollMs: w.poll_ms || 200,
        };
        if (w.component) {
          const t = resolveTarget(corpus, w.component, null, viewScope);
          collectPredRefs(t.ir, refs);
          ir.target = t.ir;
        } else if (w.selector) {
          ir.selector = String(w.selector);
        } else if (w.url_includes) {
          ir.urlIncludes = String(w.url_includes);
          addStringRefs(ir.urlIncludes, refs);
        }
        steps.push(ir);
      } else if (step.fill != null) {
        const t = resolveTarget(corpus, step.fill.target, null, viewScope);
        collectPredRefs(t.ir, refs);
        addStringRefs(step.fill.value, refs);
        steps.push({
          do: "fill",
          target: t.ir,
          value: String(step.fill.value),
        });
      } else if (step.click != null) {
        const t = resolveTarget(corpus, step.click, null, viewScope);
        collectPredRefs(t.ir, refs);
        steps.push({ do: "click", target: t.ir });
      } else if (step.press != null) {
        const ir = { do: "press", key: String(step.press) };
        if (step.target) {
          const t = resolveTarget(corpus, step.target, null, viewScope);
          collectPredRefs(t.ir, refs);
          ir.target = t.ir;
        }
        steps.push(ir);
      } else if (step.sleep != null) {
        steps.push({ do: "sleep", ms: Number(step.sleep) });
      } else if (step.scroll != null) {
        const s = step.scroll;
        const ir = { do: "scroll" };
        if (s && s.to) {
          const t = resolveTarget(corpus, s.to, null, viewScope);
          collectPredRefs(t.ir, refs);
          ir.target = t.ir;
        } else if (s && s.delta_y != null) {
          ir.deltaY = Number(s.delta_y);
        }
        steps.push(ir);
      } else if (step.read != null) {
        hasRead = true;
        const spec = {};
        for (const [key, vspec] of Object.entries(step.read)) {
          spec[key] = compileValueSpec(
            corpus,
            vspec,
            `${where} read.${key}`,
            null,
            refs,
            viewScope,
          );
        }
        steps.push({ do: "read", spec });
      }
    } catch (e) {
      errors.push(`${where}: ${e.message}`);
    }
  }

  if (!hasRead && !steps.some((s) => s.do === "navigate")) {
    warnings.push(
      `tool "${tool.name}": flow has no "read" step — the tool returns only {ok: true}`,
    );
  }
  const readOnly =
    tool.read_only != null
      ? !!tool.read_only
      : mode === "fetch" ||
        !steps.some(
          (s) => s.do === "fill" || s.do === "click" || s.do === "press",
        );
  const ir = { kind: "flow", flow: { mode, requireView, steps } };
  return { ir, refs, readOnly };
}

// compile resolves the whole manifest. Returns { ir, errors, warnings }.
function compile(corpus, manifest) {
  const errors = [];
  const warnings = [];
  const tools = [];
  for (const tool of manifest.tools || []) {
    const declared = paramNames(tool);
    let compiled = null;
    try {
      compiled = tool.api
        ? compileApiTool(corpus, tool, errors, warnings)
        : compileFlowTool(corpus, tool, errors, warnings);
    } catch (e) {
      errors.push(`tool "${tool.name}": ${e.message}`);
    }
    if (!compiled) continue;
    for (const r of compiled.refs) {
      if (!declared.has(r)) {
        errors.push(
          `tool "${tool.name}": template references undeclared param "{${r}}"`,
        );
      }
    }
    for (const p of declared) {
      if (!compiled.refs.has(p)) {
        warnings.push(
          `tool "${tool.name}": param "${p}" is declared but never referenced`,
        );
      }
    }
    const toolIR = {
      name: tool.name,
      title: tool.title || null,
      description: String(tool.description).trim(),
      readOnly: compiled.readOnly,
      inputSchema: compileParams(tool),
      params: (tool.params || []).map((p) => ({
        name: p.name,
        type: p.type,
        required: !!p.required,
        default: p.default !== undefined ? p.default : null,
      })),
      ...compiled.ir,
    };
    tools.push(toolIR);
  }
  const ir = {
    meta: {
      site: manifest.site,
      baseUrl: manifest.base_url,
      description: manifest.description || null,
      toolVersion: manifest.tool_version || "0.1.0",
      match: manifest.match || null,
    },
    tools,
  };
  return { ir, errors, warnings };
}

module.exports = { compile, resolveTarget, templateRefs };
