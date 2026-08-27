// github — WebMCP tools generated from a sightmap corpus.
// Read-only repository tools for github.com — pull requests, issues, releases, files, and PR diffstat.
// tools: list_pull_requests, list_issues, list_releases, get_repo_files, get_pr_diffstat
// generator: @sightmap/webmcp-codegen v0.1.0 (github.com/sightmap/sightmap webmcp/)
// manifest: webmcp.tools.yaml | corpus: ../../../web/src/data/atlas/github/.sightmap (7 files, sha256:a875a99ca3fd)
// DO NOT EDIT — edit the manifest or corpus and regenerate: sightmap-webmcp generate
(() => {
  "use strict";

// Browser runtime embedded into every generated WebMCP bundle. A dumb
// interpreter over the compile-time IR: shadow-piercing selector resolution,
// sightmap property extraction, live-DOM actions, fetched-document reads, and
// API replay — plus registration with document.modelContext (WebMCP) and a
// window.__sightmapWebMCP shim for verification and non-WebMCP browsers.
//
// The deep-query and extraction functions are adapted from the sightmap
// reference implementation's own browser-side helpers (go/browser/deepquery.js
// and go/observe/properties.js) so generated tools resolve nodes and values
// the same way the CLI the corpus was authored with does.

// --- deep query (shadow-piercing, offline-matcher flattening order) --------

function __smwDeepQueryAll(root, sel) {
  const out = [];
  function visit(node) {
    for (const child of node.children) {
      let m = false;
      try {
        m = child.matches(sel);
      } catch (e) {
        throw new Error(`invalid selector "${sel}"`);
      }
      if (m) out.push(child);
      visit(child);
      if (child.shadowRoot) visit(child.shadowRoot);
    }
  }
  visit(root);
  if (root.shadowRoot) visit(root.shadowRoot);
  return out;
}

// --- property extraction (sightmap extract modes + transforms) -------------

function __smwExtractValue(el, extract) {
  if (!extract) return null;
  if (extract === "text") return el.textContent;
  if (extract === "inner_text")
    return el.innerText != null ? el.innerText : el.textContent;
  if (extract === "text_only") {
    const clone = el.cloneNode(true);
    clone.querySelectorAll("img,svg,[alt]").forEach((e) => e.remove());
    return clone.textContent;
  }
  if (extract === "inner_html") return el.innerHTML;
  if (extract.startsWith("attr=")) return el.getAttribute(extract.slice(5));
  if (extract.startsWith("exists:")) {
    return __smwDeepQueryAll(el, extract.slice(7)).length > 0 ? "true" : null;
  }
  const subs = __smwDeepQueryAll(el, extract);
  if (subs.length === 0) return null;
  const sub = subs[0];
  return sub.innerText != null ? sub.innerText : sub.textContent;
}

function __smwApplyTransform(val, transform) {
  if (!transform || !val) return val;
  if (transform.indexOf("match:") === 0) {
    try {
      const m = val.match(new RegExp(transform.slice(6)));
      if (!m) return val;
      return m[1] != null ? m[1] : m[0];
    } catch (e) {
      return val;
    }
  }
  const words = val.trim().split(/\s+/);
  switch (transform) {
    case "first_word":
      return words[0] || val;
    case "last_word":
      return words[words.length - 1] || val;
    case "first_number": {
      const m = val.match(/\d[\d,.]*/);
      return m ? m[0] : val;
    }
    case "first_dollar": {
      const m = val.match(/\$[\d,.]+/);
      return m ? m[0] : val;
    }
    case "number":
      return val.replace(/[^\d.]/g, "");
    case "slug":
      return val
        .toLowerCase()
        .replace(/\s+/g, "-")
        .replace(/[^a-z0-9-]/g, "");
    default:
      return val;
  }
}

const __SMW_VALUE_CAP = 300;

function __smwReadProp(el, spec) {
  // Value omission is silent (spec): an extract that errors — e.g. a
  // selector-shaped extract that is not valid CSS — omits the value rather
  // than aborting the tool call.
  let val;
  try {
    val = __smwExtractValue(el, spec.extract);
  } catch (e) {
    return null;
  }
  if (val == null) return null;
  val = String(val).trim().replace(/\s+/g, " ");
  if (val === "") return null;
  val = __smwApplyTransform(val, spec.transform || null);
  if (!val) return null;
  return String(val).slice(0, spec.cap || __SMW_VALUE_CAP);
}

// --- template interpolation ------------------------------------------------

function __smwInterpolate(template, args, urlMode) {
  const s = String(template);
  let out = "";
  let i = 0;
  while (i < s.length) {
    if (s[i] === "{" && s[i + 1] === "{") {
      out += "{";
      i += 2;
      continue;
    }
    if (s[i] === "}" && s[i + 1] === "}") {
      out += "}";
      i += 2;
      continue;
    }
    if (s[i] === "{") {
      const m = s.slice(i).match(/^\{([a-z][a-z0-9_]*)(\|raw)?\}/);
      if (m) {
        const v = args[m[1]];
        const str = v == null ? "" : String(v);
        out += urlMode && !m[2] ? encodeURIComponent(str) : str;
        i += m[0].length;
        continue;
      }
    }
    out += s[i];
    i++;
  }
  return out;
}

// A body leaf that is exactly one "{param}" of a non-string type substitutes
// the typed value, so JSON bodies keep their numbers and booleans.
function __smwInterpolateBody(node, args, paramTypes) {
  if (typeof node === "string") {
    const m = node.match(/^\{([a-z][a-z0-9_]*)\}$/);
    if (m && paramTypes[m[1]] && paramTypes[m[1]] !== "string") {
      return args[m[1]];
    }
    return __smwInterpolate(node, args, false);
  }
  if (Array.isArray(node))
    return node.map((n) => __smwInterpolateBody(n, args, paramTypes));
  if (node && typeof node === "object") {
    const out = {};
    for (const k of Object.keys(node))
      out[k] = __smwInterpolateBody(node[k], args, paramTypes);
    return out;
  }
  return node;
}

// --- target resolution -----------------------------------------------------

function __smwResolveLevels(scopes, levels) {
  let current = Array.isArray(scopes) ? scopes : [scopes];
  for (const alternatives of levels) {
    let matched = [];
    for (const sel of alternatives) {
      for (const scope of current) {
        for (const el of __smwDeepQueryAll(scope, sel)) {
          if (!matched.includes(el)) matched.push(el);
        }
      }
      if (matched.length > 0) break; // first alternative that matches wins
    }
    current = matched;
    if (current.length === 0) return [];
  }
  return current;
}

function __smwPredMatch(el, link, pred, args) {
  const propSpec = link.props[pred.prop];
  if (!propSpec) return false;
  const val = __smwReadProp(el, propSpec);
  if (val == null) return false;
  let want = __smwInterpolate(pred.value, args, false);
  let have = val;
  if (pred.ci) {
    want = want.toLowerCase();
    have = have.toLowerCase();
  }
  if (pred.op === "=") return have === want;
  if (pred.op === "^=") return have.startsWith(want);
  if (pred.op === "*=") return have.includes(want);
  return false;
}

function __smwResolveTarget(root, target, args) {
  if (target.kind === "css") {
    return __smwDeepQueryAll(
      root,
      __smwInterpolate(target.selector, args || {}, false),
    );
  }
  let scopes = [root];
  for (const link of target.links) {
    let els = __smwResolveLevels(scopes, link.chain);
    if (link.preds && link.preds.length > 0) {
      els = els.filter((el) =>
        link.preds.every((p) => __smwPredMatch(el, link, p, args)),
      );
    }
    if (link.index != null) {
      els = link.index < els.length ? [els[link.index]] : [];
    }
    scopes = els;
    if (scopes.length === 0) return [];
  }
  return scopes;
}

function __smwTargetDesc(target, args) {
  if (target.kind === "css") {
    return __smwInterpolate(target.selector, args || {}, false);
  }
  return target.links
    .map(
      (l) =>
        l.name.split(" ").pop() +
        (l.preds || [])
          .map(
            (p) =>
              `[${p.prop}${p.op}${__smwInterpolate(p.value, args || {}, false)}]`,
          )
          .join(""),
    )
    .join(" ");
}

function __smwRequireOne(root, target, args, what) {
  const els = __smwResolveTarget(root, target, args);
  if (els.length === 0) {
    throw new Error(
      `${what}: "${__smwTargetDesc(target, args)}" matched nothing on the current page`,
    );
  }
  if (els.length > 1) {
    throw new Error(
      `${what}: "${__smwTargetDesc(target, args)}" matched ${els.length} elements — add a property predicate or #N index to disambiguate`,
    );
  }
  return els[0];
}

// --- actions ---------------------------------------------------------------

function __smwFill(el, value) {
  const proto =
    typeof HTMLTextAreaElement !== "undefined" &&
    el instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : typeof HTMLInputElement !== "undefined" &&
          el instanceof HTMLInputElement
        ? HTMLInputElement.prototype
        : null;
  if (!proto) {
    if (el.isContentEditable) {
      el.textContent = value;
      el.dispatchEvent(new Event("input", { bubbles: true }));
      return;
    }
    throw new Error(
      "fill target is not an input, textarea, or contenteditable element",
    );
  }
  if (el.focus) el.focus();
  const setter = Object.getOwnPropertyDescriptor(proto, "value");
  if (setter && setter.set) setter.set.call(el, value);
  else el.value = value;
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
}

function __smwClick(el) {
  if (el.scrollIntoView) {
    try {
      el.scrollIntoView({
        block: "center",
        inline: "center",
        behavior: "instant",
      });
    } catch (e) {
      /* older engines */
    }
  }
  // Covered-target check, when the engine supports it (jsdom doesn't).
  if (
    typeof document.elementFromPoint === "function" &&
    el.getBoundingClientRect
  ) {
    const r = el.getBoundingClientRect();
    if (r.width > 0 && r.height > 0) {
      const cx = Math.floor(r.left + r.width / 2);
      const cy = Math.floor(r.top + r.height / 2);
      if (
        cx >= 0 &&
        cy >= 0 &&
        cx < window.innerWidth &&
        cy < window.innerHeight
      ) {
        // elementFromPoint does not pierce shadow roots — it returns the
        // shadow host for shadow content — so containment is checked over
        // the composed tree, not the light tree.
        const at = document.elementFromPoint(cx, cy);
        if (
          at &&
          at !== el &&
          !__smwComposedContains(el, at) &&
          !__smwComposedContains(at, el)
        ) {
          throw new Error(
            "click target is covered by another element (an open overlay or modal?)",
          );
        }
      }
    }
  }
  el.click();
}

// __smwComposedContains reports whether ancestor contains el in the composed
// (shadow-including) tree.
function __smwComposedContains(ancestor, el) {
  let n = el;
  while (n) {
    if (n === ancestor || (ancestor.contains && ancestor.contains(n))) {
      return true;
    }
    const root = n.getRootNode ? n.getRootNode() : null;
    n = root && root.host ? root.host : null;
  }
  return false;
}

const __SMW_KEYCODES = {
  Enter: 13,
  Tab: 9,
  Escape: 27,
  Backspace: 8,
  ArrowDown: 40,
  ArrowUp: 38,
};

function __smwPress(el, key) {
  const opts = { key, bubbles: true, cancelable: true };
  if (__SMW_KEYCODES[key]) {
    opts.keyCode = __SMW_KEYCODES[key];
    opts.which = __SMW_KEYCODES[key];
  }
  el.dispatchEvent(new KeyboardEvent("keydown", opts));
  el.dispatchEvent(new KeyboardEvent("keyup", opts));
}

function __smwSleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function __smwWaitFor(step, args, signal) {
  const deadline = Date.now() + step.timeoutMs;
  for (;;) {
    if (signal && signal.aborted) throw new Error("aborted");
    if (step.target) {
      if (__smwResolveTarget(document, step.target, args).length > 0) return;
    } else if (step.selector) {
      if (__smwDeepQueryAll(document, step.selector).length > 0) return;
    } else if (step.urlIncludes) {
      if (
        location.href.includes(__smwInterpolate(step.urlIncludes, args, false))
      )
        return;
    }
    if (Date.now() >= deadline) {
      const what = step.target
        ? __smwTargetDesc(step.target, args)
        : step.selector || step.urlIncludes;
      throw new Error(`wait_for "${what}" timed out after ${step.timeoutMs}ms`);
    }
    await __smwSleep(step.pollMs);
  }
}

// --- reads -----------------------------------------------------------------

function __smwReadValue(root, valueIR, args) {
  if (valueIR.one) {
    const spec = valueIR.one;
    let el = root;
    if (spec.target) {
      const els = __smwResolveTarget(root, spec.target, args);
      if (els.length === 0) return undefined;
      el = els[0];
    }
    const v = __smwReadProp(el, spec);
    return v == null ? undefined : v;
  }
  const list = valueIR.list;
  let els = __smwResolveTarget(root, list.target, args);
  let max = list.max;
  if (typeof max === "string")
    max = parseInt(__smwInterpolate(max, args, false), 10) || null;
  if (max != null && els.length > max) els = els.slice(0, max);
  return els.map((el) => {
    const row = {};
    for (const key of Object.keys(list.fields)) {
      const spec = list.fields[key];
      let target = el;
      if (spec.target) {
        const found = __smwResolveTarget(el, spec.target, args);
        if (found.length === 0) continue;
        target = found[0];
      }
      const v = __smwReadProp(target, spec);
      if (v != null) row[key] = v;
    }
    return row;
  });
}

function __smwRunRead(root, spec, args, out) {
  for (const key of Object.keys(spec)) {
    const v = __smwReadValue(root, spec[key], args);
    if (v !== undefined) out[key] = v;
  }
}

// --- request/response property extraction (api tools) ----------------------

function __smwGetPath(obj, path) {
  let cur = obj;
  for (const seg of String(path).split(".")) {
    if (cur == null) return undefined;
    if (Array.isArray(cur) && /^\d+$/.test(seg)) cur = cur[parseInt(seg, 10)];
    else if (typeof cur === "object") cur = cur[seg];
    else return undefined;
  }
  return cur;
}

// A JSON array in an api response projected into named per-row fields — the
// api-side counterpart of a flow read's for_each. `field:` walks a dot path in
// the row; `template:` composes a string, where {param} is the tool's argument
// and {row.some.path} is a value from the row (params resolve first, so data
// containing braces cannot be read back as a parameter).
function __smwInterpolateRow(template, row, args) {
  return __smwInterpolate(template, args, false).replace(
    /\{row\.([A-Za-z0-9_.]+)\}/g,
    (whole, path) => {
      const v = __smwGetPath(row, path);
      return v == null ? "" : String(v);
    },
  );
}

function __smwProjectRows(rows, rspBody, args) {
  const arr = rows.field ? __smwGetPath(rspBody, rows.field) : rspBody;
  if (!Array.isArray(arr)) return undefined;
  let max = arr.length;
  if (rows.max != null) {
    const n = parseInt(__smwInterpolate(String(rows.max), args, false), 10);
    if (!isNaN(n) && n >= 0) max = Math.min(max, n);
  }
  const out = [];
  for (let i = 0; i < arr.length && out.length < max; i++) {
    const row = arr[i];
    const projected = {};
    for (const name of Object.keys(rows.fields || {})) {
      const spec = rows.fields[name];
      let v;
      if (spec.template != null) v = __smwInterpolateRow(spec.template, row, args);
      else if (spec.field != null) v = __smwGetPath(row, spec.field);
      // Same silent-omission convention as request-property extraction: a
      // value that does not resolve, including an empty string, is absent
      // rather than reported as null or "".
      if (v !== undefined && v !== null && v !== "") projected[name] = v;
    }
    out.push(projected);
  }
  return out;
}

// A value read from the page itself rather than written into the manifest —
// how a tool reaches the session the user is already signed in with. Covers
// the ways browser apps actually hold credentials:
//
//   local_storage / session_storage  key (+ json path) — SPA bearer tokens
//   cookie                           key — readable session and CSRF cookies
//   dom                              selector (+ attr) — <meta name="csrf-token">,
//                                    hidden inputs, data attributes
//
// HttpOnly cookies need none of this; `credentials: include` already sends
// them. Deliberately declarative — a key or a selector, an optional JSON path,
// an optional literal prefix — and not an expression language: a manifest may
// name a value, not run code.
function __smwPageValue(spec) {
  let raw;
  if (spec.from === "cookie") {
    try {
      const all = String(document.cookie || "").split(";");
      for (const part of all) {
        const eq = part.indexOf("=");
        if (eq < 0) continue;
        if (part.slice(0, eq).trim() === spec.key) {
          raw = decodeURIComponent(part.slice(eq + 1).trim());
          break;
        }
      }
    } catch (e) {
      return undefined;
    }
  } else if (spec.from === "dom") {
    const el = __smwDeepQueryAll(document, spec.selector)[0];
    if (!el) return undefined;
    raw = spec.attr ? el.getAttribute(spec.attr) : el.textContent;
    if (raw != null) raw = String(raw).trim();
  } else {
    let store;
    try {
      store =
        spec.from === "session_storage"
          ? typeof sessionStorage !== "undefined" && sessionStorage
          : typeof localStorage !== "undefined" && localStorage;
    } catch (e) {
      return undefined; // storage blocked by the embedder
    }
    if (!store) return undefined;
    try {
      raw = store.getItem(spec.key);
    } catch (e) {
      return undefined;
    }
  }
  if (raw == null || raw === "") return undefined;
  let val = raw;
  if (spec.json) {
    let parsed;
    try {
      parsed = JSON.parse(raw);
    } catch (e) {
      return undefined;
    }
    val = __smwGetPath(parsed, spec.json);
    if (val == null) return undefined;
  }
  return (spec.prefix || "") + String(val);
}

// Header/query/body leaves are either a "{param}" template or a page-state
// mapping. Returns undefined when a page value is missing, so the caller can
// omit the header rather than send an empty one.
function __smwResolveValue(v, args) {
  if (v != null && typeof v === "object" && v.from) return __smwPageValue(v);
  return __smwInterpolate(v, args, false);
}

function __smwExtractResult(spec, ctx) {
  // ctx: { reqBody, reqHeaders, rspBody, rspHeaders } — bodies parsed JSON or
  // raw string; headers as maps of lower-cased names.
  let raw;
  if (spec.source === "rsp.headers" || spec.source === "req.headers") {
    const headers =
      spec.source === "rsp.headers" ? ctx.rspHeaders : ctx.reqHeaders;
    raw = headers ? headers[String(spec.field).toLowerCase()] : undefined;
  } else {
    const body = spec.source === "rsp.body" ? ctx.rspBody : ctx.reqBody;
    if (spec.field != null) {
      raw =
        typeof body === "string" ? undefined : __smwGetPath(body, spec.field);
    } else {
      raw = typeof body === "string" ? body : JSON.stringify(body);
    }
  }
  if (raw == null) return undefined;
  let val = typeof raw === "string" ? raw : JSON.stringify(raw);
  if (spec.pattern) {
    try {
      const m = val.match(new RegExp(spec.pattern));
      if (!m) return undefined;
      val = m[1] != null ? m[1] : m[0];
    } catch (e) {
      return undefined;
    }
  }
  if (spec.transform) val = __smwApplyTransform(val, spec.transform);
  return val === "" ? undefined : val;
}

async function __smwRunApi(tool, args, meta, signal) {
  const api = tool.api;
  const paramTypes = {};
  for (const p of tool.params) paramTypes[p.name] = p.type;
  const url = new URL(__smwInterpolate(api.url, args, true), meta.baseUrl);
  if (api.query) {
    for (const k of Object.keys(api.query)) {
      const qv = __smwResolveValue(api.query[k], args);
      if (qv !== undefined) url.searchParams.set(k, qv);
    }
  }
  const init = {
    method: api.method,
    credentials: api.credentials || "include",
  };
  if (signal) init.signal = signal;
  const headers = {};
  // Header names whose value came from page state. They are never exposed back
  // to the caller: a result spec reading req.headers must not be able to lift
  // the user's session token out of the request and hand it to the agent.
  const secretHeaders = {};
  if (api.headers) {
    for (const k of Object.keys(api.headers)) {
      const hv = __smwResolveValue(api.headers[k], args);
      if (hv === undefined) continue;
      headers[k] = hv;
      const spec = api.headers[k];
      if (spec != null && typeof spec === "object" && spec.from) {
        secretHeaders[k.toLowerCase()] = true;
      }
    }
  }
  let reqBody = null;
  if (api.body != null) {
    if (typeof api.body === "string") {
      reqBody = __smwInterpolate(api.body, args, false);
      init.body = reqBody;
    } else {
      reqBody = __smwInterpolateBody(api.body, args, paramTypes);
      init.body = JSON.stringify(reqBody);
      if (!headers["content-type"] && !headers["Content-Type"]) {
        headers["content-type"] = "application/json";
      }
    }
  }
  if (Object.keys(headers).length > 0) init.headers = headers;

  const resp = await fetch(url.toString(), init);
  const text = await resp.text();
  let rspBody = text;
  try {
    rspBody = JSON.parse(text);
  } catch (e) {
    /* not JSON */
  }
  const rspHeaders = {};
  if (resp.headers && resp.headers.forEach) {
    resp.headers.forEach((v, k) => {
      rspHeaders[String(k).toLowerCase()] = v;
    });
  }
  // A result spec named "status" shadows the HTTP identity (spec: reserved
  // identity names) — the declared extraction is the only thing that may set
  // it, and on a miss the key is silently absent, never the HTTP code.
  const out = {};
  if (!(api.result || []).some((r) => r.name === "status")) {
    out.status = resp.status;
  }
  if (api.result && api.result.length > 0) {
    // Header names match case-insensitively (spec) — normalize the request
    // side the same way the response side already is.
    const reqHeaders = {};
    for (const k of Object.keys(headers)) {
      if (secretHeaders[k.toLowerCase()]) continue;
      reqHeaders[k.toLowerCase()] = headers[k];
    }
    const ctx = { reqBody, reqHeaders, rspBody, rspHeaders };
    for (const spec of api.result) {
      const v = __smwExtractResult(spec, ctx);
      if (v !== undefined) out[spec.name] = v;
    }
  }
  if (api.rows) {
    const projected = __smwProjectRows(api.rows, rspBody, args);
    if (projected !== undefined) out.rows = projected;
  }
  if (!(api.result && api.result.length > 0) && !api.rows) {
    out.url = url.toString();
    if (typeof rspBody === "string") {
      out.body = rspBody.slice(0, api.maxBodyChars);
    } else if (JSON.stringify(rspBody).length > api.maxBodyChars) {
      out.body = JSON.stringify(rspBody).slice(0, api.maxBodyChars);
      out.body_truncated = true;
    } else {
      out.body = rspBody;
    }
  }
  return out;
}

// --- flow execution --------------------------------------------------------

// The corpus records a view's URL as the reference-capture URL it was authored
// against, which is typically localhost. Sending an agent there from a deployed
// page would walk it off the site, so resolve the view's route against the
// origin the tool is actually running on and keep the corpus URL only for
// routes too wildcarded to rebuild (and when there is no location to use).
function __smwViewUrl(requireView) {
  const route = requireView.route;
  if (route && route.indexOf("*") === -1 && typeof location !== "undefined") {
    try {
      return new URL(route, location.origin).toString();
    } catch (e) {
      /* fall through to the corpus URL */
    }
  }
  return requireView.url || undefined;
}

async function __smwRunFlow(tool, args, meta, signal) {
  const flow = tool.flow;
  const out = { ok: true };

  if (flow.mode === "fetch") {
    const nav = flow.steps[0];
    const url = new URL(__smwInterpolate(nav.url, args, true), meta.baseUrl);
    const init = { credentials: "include" };
    if (signal) init.signal = signal;
    const resp = await fetch(url.toString(), init);
    const text = await resp.text();
    const doc = new DOMParser().parseFromString(text, "text/html");
    out.url = resp.url || url.toString();
    out.status = resp.status;
    for (const step of flow.steps.slice(1)) {
      if (step.do === "read")
        __smwRunRead(doc.documentElement, step.spec, args, out);
    }
    return out;
  }

  if (flow.requireView) {
    const re = new RegExp(flow.requireView.pathRegex);
    let path = location.pathname;
    try {
      path = decodeURIComponent(path); // route globs match the decoded path
    } catch (e) {
      /* malformed escape: match the raw path */
    }
    if (path.length > 1) path = path.replace(/\/+$/, "");
    if (!re.test(path)) {
      return {
        error: `this tool runs on the ${flow.requireView.view} view (route ${flow.requireView.route}); the page is at ${location.pathname}`,
        expected_view: flow.requireView.view,
        navigate_to: __smwViewUrl(flow.requireView),
      };
    }
  }

  for (const step of flow.steps) {
    if (signal && signal.aborted) throw new Error("aborted");
    if (step.do === "navigate") {
      const url = new URL(
        __smwInterpolate(step.url, args, true),
        meta.baseUrl,
      ).toString();
      out.navigated = url;
      out.note =
        "navigation started; the tool set re-registers on the new document";
      setTimeout(() => {
        try {
          location.assign(url);
        } catch (e) {
          /* jsdom: not implemented */
        }
      }, 0);
    } else if (step.do === "wait_for") {
      await __smwWaitFor(step, args, signal);
    } else if (step.do === "fill") {
      const el = __smwRequireOne(document, step.target, args, "fill");
      __smwFill(el, __smwInterpolate(step.value, args, false));
    } else if (step.do === "click") {
      const el = __smwRequireOne(document, step.target, args, "click");
      __smwClick(el);
    } else if (step.do === "press") {
      const el = step.target
        ? __smwRequireOne(document, step.target, args, "press")
        : document.activeElement || document.body;
      __smwPress(el, step.key);
    } else if (step.do === "sleep") {
      await __smwSleep(step.ms);
    } else if (step.do === "scroll") {
      if (step.target) {
        const els = __smwResolveTarget(document, step.target, args);
        if (els[0] && els[0].scrollIntoView) {
          els[0].scrollIntoView({ block: "center", behavior: "instant" });
        }
      } else if (step.deltaY != null && typeof window.scrollBy === "function") {
        window.scrollBy(0, step.deltaY);
      }
    } else if (step.do === "read") {
      __smwRunRead(document.documentElement, step.spec, args, out);
    }
  }
  return out;
}

// --- tool execution + registration ----------------------------------------

function __smwValidateArgs(tool, args) {
  const out = {};
  for (const p of tool.params) {
    let v = args ? args[p.name] : undefined;
    if (v == null && p.default != null) v = p.default;
    if (v == null) {
      if (p.required) throw new Error(`missing required param "${p.name}"`);
      continue;
    }
    out[p.name] = v;
  }
  return out;
}

async function __smwExecuteTool(tool, meta, rawArgs, signal) {
  const args = __smwValidateArgs(tool, rawArgs || {});
  if (tool.kind === "api") return __smwRunApi(tool, args, meta, signal);
  return __smwRunFlow(tool, args, meta, signal);
}

function __smwDescriptor(tool) {
  const d = {
    name: tool.name,
    description: tool.description,
    inputSchema: tool.inputSchema,
    annotations: { readOnlyHint: !!tool.readOnly },
  };
  if (tool.title) d.title = tool.title;
  return d;
}

function __smwBoot(meta, tools) {
  const win = typeof window !== "undefined" ? window : globalThis;
  // Idempotent only for the SAME bundle: re-injecting an updated bundle
  // (the generate → inject → call verification loop) replaces the shim
  // instead of silently keeping stale tools. modelContext may reject
  // re-registration of unchanged names; the per-tool catch below logs and
  // moves on.
  const sig = meta.toolVersion + ":" + JSON.stringify(tools);
  if (
    win.__sightmapWebMCP &&
    win.__sightmapWebMCP.site === meta.site &&
    win.__sightmapWebMCP._sig === sig
  ) {
    return win.__sightmapWebMCP; // same bundle already booted
  }

  const shim = {
    site: meta.site,
    version: meta.toolVersion,
    _sig: sig,
    generator: "sightmap-webmcp",
    last: null,
    listTools() {
      return tools.map(__smwDescriptor);
    },
    callTool(name, args) {
      const tool = tools.find((t) => t.name === name);
      if (!tool) {
        return Promise.reject(
          new Error(
            `no tool named "${name}" (tools: ${tools.map((t) => t.name).join(", ")})`,
          ),
        );
      }
      return __smwExecuteTool(tool, meta, args);
    },
    // For harnesses whose eval bridge can't await a promise (e.g. `sightmap
    // browser eval`): starts the call, stores the outcome on shim.last.
    callToolAndStore(name, args) {
      shim.last = { tool: name, done: false };
      shim.callTool(name, args).then(
        (result) => {
          shim.last = { tool: name, done: true, result };
        },
        (err) => {
          shim.last = {
            tool: name,
            done: true,
            error: String((err && err.message) || err),
          };
        },
      );
      return "started";
    },
  };
  win.__sightmapWebMCP = shim;

  // WebMCP proper: document.modelContext today; navigator.modelContext in
  // earlier drafts of the proposal. Registration failures (origin trial off,
  // permissions policy) degrade to the shim alone.
  const mc =
    (typeof document !== "undefined" && document.modelContext) ||
    (typeof navigator !== "undefined" && navigator.modelContext) ||
    null;
  if (mc && typeof mc.registerTool === "function") {
    for (const tool of tools) {
      const descriptor = __smwDescriptor(tool);
      descriptor.execute = (input, options) =>
        __smwExecuteTool(tool, meta, input, options && options.signal);
      try {
        Promise.resolve(mc.registerTool(descriptor)).catch((e) => {
          console.warn(
            `[sightmap-webmcp] registerTool(${tool.name}) rejected:`,
            e,
          );
        });
      } catch (e) {
        console.warn(`[sightmap-webmcp] registerTool(${tool.name}) threw:`, e);
      }
    }
  }
  return shim;
}

const __SMW_META = {
  "site": "github",
  "baseUrl": "https://github.com",
  "description": "Read-only repository tools for github.com — pull requests, issues, releases, files, and PR diffstat.",
  "toolVersion": "0.1.0",
  "match": [
    "https://github.com/*"
  ]
};

const __SMW_TOOLS = [
  {
    "name": "list_pull_requests",
    "title": "List a repository's open pull requests",
    "description": "Fetch /{owner}/{repo}/pulls and return the open pull-request rows (server-rendered, so no browser navigation is needed). Rows deliberately carry legacy issue markup (ids like issue_<number>) — both lists render from one legacy row component — and each row's text concatenates title, labels, author, and review state. Review-decision badges arrive in a later batch fetch and are not part of this read.",
    "readOnly": true,
    "inputSchema": {
      "type": "object",
      "properties": {
        "owner": {
          "type": "string",
          "description": "Repository owner, e.g. \"sightmap\"."
        },
        "repo": {
          "type": "string",
          "description": "Repository name, e.g. \"sightmap\"."
        }
      },
      "required": [
        "owner",
        "repo"
      ]
    },
    "params": [
      {
        "name": "owner",
        "type": "string",
        "required": true,
        "default": null
      },
      {
        "name": "repo",
        "type": "string",
        "required": true,
        "default": null
      }
    ],
    "kind": "flow",
    "flow": {
      "mode": "fetch",
      "requireView": null,
      "steps": [
        {
          "do": "navigate",
          "url": "/{owner}/{repo}/pulls"
        },
        {
          "do": "read",
          "spec": {
            "pull_requests": {
              "list": {
                "target": {
                  "kind": "chain",
                  "links": [
                    {
                      "name": "PullRow",
                      "chain": [
                        [
                          ".js-issue-row"
                        ]
                      ],
                      "props": {
                        "row_text": {
                          "extract": "text"
                        }
                      },
                      "preds": [],
                      "index": null
                    }
                  ]
                },
                "max": null,
                "fields": {
                  "text": {
                    "extract": "text"
                  },
                  "title": {
                    "extract": "text",
                    "target": {
                      "kind": "chain",
                      "links": [
                        {
                          "name": "PullRow PullTitleLink",
                          "chain": [
                            [
                              "a[id^=\"issue_\"]"
                            ]
                          ],
                          "props": {
                            "title": {
                              "extract": "text"
                            },
                            "href": {
                              "extract": "attr=href"
                            }
                          },
                          "preds": [],
                          "index": null
                        }
                      ]
                    }
                  },
                  "url": {
                    "extract": "attr=href",
                    "target": {
                      "kind": "chain",
                      "links": [
                        {
                          "name": "PullRow PullTitleLink",
                          "chain": [
                            [
                              "a[id^=\"issue_\"]"
                            ]
                          ],
                          "props": {
                            "title": {
                              "extract": "text"
                            },
                            "href": {
                              "extract": "attr=href"
                            }
                          },
                          "preds": [],
                          "index": null
                        }
                      ]
                    }
                  }
                }
              }
            }
          }
        }
      ]
    }
  },
  {
    "name": "list_issues",
    "title": "List a repository's open issues",
    "description": "Fetch /{owner}/{repo}/issues and return the issue rows (25 per page by default). This route is served by GitHub's React frontend, but the rows are in the initial document. Row state (open/closed) is an icon with no text, so it is not part of this read — the list defaults to open issues.",
    "readOnly": true,
    "inputSchema": {
      "type": "object",
      "properties": {
        "owner": {
          "type": "string",
          "description": "Repository owner."
        },
        "repo": {
          "type": "string",
          "description": "Repository name."
        }
      },
      "required": [
        "owner",
        "repo"
      ]
    },
    "params": [
      {
        "name": "owner",
        "type": "string",
        "required": true,
        "default": null
      },
      {
        "name": "repo",
        "type": "string",
        "required": true,
        "default": null
      }
    ],
    "kind": "flow",
    "flow": {
      "mode": "fetch",
      "requireView": null,
      "steps": [
        {
          "do": "navigate",
          "url": "/{owner}/{repo}/issues"
        },
        {
          "do": "read",
          "spec": {
            "filters": {
              "one": {
                "extract": "text",
                "target": {
                  "kind": "chain",
                  "links": [
                    {
                      "name": "IssueFilters",
                      "chain": [
                        [
                          "nav[aria-label=\"Issue filters\"]"
                        ]
                      ],
                      "props": {
                        "filters": {
                          "extract": "text"
                        }
                      },
                      "preds": [],
                      "index": null
                    }
                  ]
                }
              }
            },
            "issues": {
              "list": {
                "target": {
                  "kind": "chain",
                  "links": [
                    {
                      "name": "IssueRowTitle",
                      "chain": [
                        [
                          "[data-testid=\"issue-pr-title-link\"]"
                        ]
                      ],
                      "props": {
                        "title": {
                          "extract": "text"
                        },
                        "href": {
                          "extract": "attr=href"
                        }
                      },
                      "preds": [],
                      "index": null
                    }
                  ]
                },
                "max": null,
                "fields": {
                  "title": {
                    "extract": "text"
                  },
                  "url": {
                    "extract": "attr=href"
                  }
                }
              }
            }
          }
        }
      ]
    }
  },
  {
    "name": "list_releases",
    "title": "List a repository's releases",
    "description": "Fetch /{owner}/{repo}/releases (newest first). Each entry is a link to a release tag. The same tag URL appears several times per release — tag name, source links, and the anchor heading all point at it — so deduplicate by url before counting releases.",
    "readOnly": true,
    "inputSchema": {
      "type": "object",
      "properties": {
        "owner": {
          "type": "string",
          "description": "Repository owner."
        },
        "repo": {
          "type": "string",
          "description": "Repository name."
        }
      },
      "required": [
        "owner",
        "repo"
      ]
    },
    "params": [
      {
        "name": "owner",
        "type": "string",
        "required": true,
        "default": null
      },
      {
        "name": "repo",
        "type": "string",
        "required": true,
        "default": null
      }
    ],
    "kind": "flow",
    "flow": {
      "mode": "fetch",
      "requireView": null,
      "steps": [
        {
          "do": "navigate",
          "url": "/{owner}/{repo}/releases"
        },
        {
          "do": "read",
          "spec": {
            "release_links": {
              "list": {
                "target": {
                  "kind": "chain",
                  "links": [
                    {
                      "name": "ReleaseTagLink",
                      "chain": [
                        [
                          "a[href*=\"/releases/tag/\"]"
                        ]
                      ],
                      "props": {
                        "tag": {
                          "extract": "text"
                        }
                      },
                      "preds": [],
                      "index": null
                    }
                  ]
                },
                "max": null,
                "fields": {
                  "tag": {
                    "extract": "text"
                  },
                  "url": {
                    "extract": "attr=href"
                  }
                }
              }
            }
          }
        }
      ]
    }
  },
  {
    "name": "get_repo_files",
    "title": "Read a repository's root file listing",
    "description": "Fetch a repository home page and return its root file table and (when GitHub inlines it) the README text. The table is in the initial document; the README, latest-commit strip, and sidebar are sometimes deferred to follow-up fetches, so an absent `readme` means \"deferred on this load\", not \"no README\". Use the repository's raw file URLs for file contents.",
    "readOnly": true,
    "inputSchema": {
      "type": "object",
      "properties": {
        "owner": {
          "type": "string",
          "description": "Repository owner."
        },
        "repo": {
          "type": "string",
          "description": "Repository name."
        }
      },
      "required": [
        "owner",
        "repo"
      ]
    },
    "params": [
      {
        "name": "owner",
        "type": "string",
        "required": true,
        "default": null
      },
      {
        "name": "repo",
        "type": "string",
        "required": true,
        "default": null
      }
    ],
    "kind": "flow",
    "flow": {
      "mode": "fetch",
      "requireView": null,
      "steps": [
        {
          "do": "navigate",
          "url": "/{owner}/{repo}"
        },
        {
          "do": "read",
          "spec": {
            "files": {
              "one": {
                "extract": "text",
                "target": {
                  "kind": "chain",
                  "links": [
                    {
                      "name": "FileTable",
                      "chain": [
                        [
                          "table[aria-labelledby=\"folders-and-files\"]"
                        ]
                      ],
                      "props": {
                        "entries": {
                          "extract": "text"
                        }
                      },
                      "preds": [],
                      "index": null
                    }
                  ]
                },
                "cap": 3000
              }
            },
            "readme": {
              "one": {
                "extract": "text",
                "target": {
                  "kind": "chain",
                  "links": [
                    {
                      "name": "Readme",
                      "chain": [
                        [
                          "article.markdown-body"
                        ]
                      ],
                      "props": {
                        "readme_text": {
                          "extract": "text"
                        }
                      },
                      "preds": [],
                      "index": null
                    }
                  ]
                }
              }
            }
          }
        }
      ]
    }
  },
  {
    "name": "get_pr_diffstat",
    "title": "Diffstat for one pull request",
    "description": "Lines added, deleted, and changed for one pull request, from the same page_data endpoint the PR header uses (the header's diffstat testids are filled in after the document from this response). Requires the JSON accept and XMLHttpRequest headers or GitHub answers 406.",
    "readOnly": true,
    "inputSchema": {
      "type": "object",
      "properties": {
        "owner": {
          "type": "string",
          "description": "Repository owner."
        },
        "repo": {
          "type": "string",
          "description": "Repository name."
        },
        "number": {
          "type": "integer",
          "description": "Pull request number."
        }
      },
      "required": [
        "owner",
        "repo",
        "number"
      ]
    },
    "params": [
      {
        "name": "owner",
        "type": "string",
        "required": true,
        "default": null
      },
      {
        "name": "repo",
        "type": "string",
        "required": true,
        "default": null
      },
      {
        "name": "number",
        "type": "integer",
        "required": true,
        "default": null
      }
    ],
    "kind": "api",
    "api": {
      "method": "GET",
      "url": "https://github.com/{owner}/{repo}/pull/{number}/page_data/diffstat",
      "query": null,
      "headers": {
        "accept": "application/json",
        "x-requested-with": "XMLHttpRequest"
      },
      "body": null,
      "result": [
        {
          "name": "lines_added",
          "source": "rsp.body",
          "field": "diffstat.linesAdded",
          "pattern": null,
          "transform": null
        },
        {
          "name": "lines_deleted",
          "source": "rsp.body",
          "field": "diffstat.linesDeleted",
          "pattern": null,
          "transform": null
        },
        {
          "name": "lines_changed",
          "source": "rsp.body",
          "field": "diffstat.linesChanged",
          "pattern": null,
          "transform": null
        }
      ],
      "rows": null,
      "maxBodyChars": 20000,
      "credentials": "include",
      "request": "Diffstat"
    }
  }
];

__smwBoot(__SMW_META, __SMW_TOOLS);
})();
