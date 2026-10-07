// The script the scanner installs in every document *before* any page script
// runs. It records WebMCP tool registrations without ever executing a tool.
//
// WebMCP has no page-side "list the tools" call — that is the agent's side of
// the API — so the only way to see what a page registers is to be the surface
// it registers against. Four cases:
//
//   1. The browser already provides a native `navigator.modelContext` (Chrome
//      with the ModelContext blink feature). Its methods are wrapped so each
//      registration is copied into the record before being passed through.
//   2. The browser provides nothing. A spec-shaped recorder is installed on
//      both `navigator` and `document` (Sightkick registers on the latter), so
//      a page that feature-detects the API finds it and registers, and a page
//      that would have installed its own polyfill uses ours instead. Either
//      way the tool set the page *would* expose to a WebMCP agent is what gets
//      recorded.
//   3. The page installs its own surface over ours anyway. The two ways it can
//      do that are both covered: an *assignment* (`navigator.modelContext =
//      mine`) lands in the setter of the accessor we install, which adopts the
//      replacement — wrapping its `registerTool`/`provideContext` so later
//      registrations are still recorded; a *redefinition*
//      (`Object.defineProperty(document, 'modelContext', …)`, what the mcp-b
//      polyfill does) throws our accessor away entirely, so nothing can be
//      observed live and the surface is instead *enumerated* at collect time
//      through whatever it exposes (`getToolInfos`, `getRegisteredToolInfos`,
//      `getTools`, `listTools`, a `tools` map, or the testing shim on
//      `navigator.modelContextTesting`). Those land as `via: 'replaced-surface'`.
//   4. Declarative tools (`<form toolname>`) are read off the DOM after load.
//
// `executeTool` always rejects — on our own recorder, and on any surface we
// adopt from the page: discovery never runs site-defined code with site-chosen
// arguments.
//
// The recorder also listens for failed script loads, so a report can say "the
// polyfill never loaded" instead of silently reporting no WebMCP surface.
//
// Kept as a plain string so `sightmap browser inject --persist` can install it
// on every new document, and so the exact bytes that run in the page are
// reviewable here, not assembled at runtime.
export const RECORDER_KEY = '__sightmapAtlasScan'

export const RECORDER_SCRIPT = `(() => {
  const KEY = ${JSON.stringify(RECORDER_KEY)};
  if (Object.prototype.hasOwnProperty.call(globalThis, KEY)) return;
  const state = { records: [], native: { navigator: false, document: false }, errors: [] };
  Object.defineProperty(globalThis, KEY, { value: state, enumerable: false, configurable: false, writable: false });

  const NEVER_EXECUTE = 'sightmap atlas scan: tools are enumerated, never executed';
  const isNative = (fn) => {
    try { return typeof fn === 'function' && /\\[native code\\]/.test(Function.prototype.toString.call(fn)); }
    catch { return false; }
  };
  const clone = (v) => { try { return v === undefined ? undefined : JSON.parse(JSON.stringify(v)); } catch { return null; } };
  const str = (v, max) => { const s = v == null ? '' : String(v); return s.length > max ? s.slice(0, max) : s; };
  const note = (msg) => { try { const m = str(msg, 500); if (state.errors.length < 100 && state.errors.indexOf(m) === -1) state.errors.push(m); } catch {} };
  const record = (api, via, def) => {
    try {
      if (state.records.length >= 500) return;
      state.records.push({
        api, via,
        name: str(def && def.name, 200),
        description: str(def && def.description, 4000),
        inputSchema: clone(def && def.inputSchema),
        hasExecute: !!(def && typeof def.execute === 'function'),
        t: Date.now(),
      });
    } catch (e) { note('record: ' + e); }
  };

  const makeRecorder = (api) => {
    const tools = new Map();
    const target = new EventTarget();
    const rec = Object.create(target);
    const change = () => { try { target.dispatchEvent(new Event('toolchange')); } catch {} };
    rec.registerTool = (def, opts) => {
      record(api, 'registerTool', def);
      if (def && def.name) tools.set(String(def.name), def);
      const signal = opts && opts.signal;
      if (signal && typeof signal.addEventListener === 'function') {
        signal.addEventListener('abort', () => { if (def && tools.get(String(def.name)) === def) { tools.delete(String(def.name)); change(); } }, { once: true });
      }
      change();
      return Promise.resolve();
    };
    rec.unregisterTool = (name) => { tools.delete(String(name)); change(); return Promise.resolve(); };
    rec.provideContext = (ctx) => {
      const list = (ctx && Array.isArray(ctx.tools)) ? ctx.tools : [];
      tools.clear();
      for (const def of list) { record(api, 'provideContext', def); if (def && def.name) tools.set(String(def.name), def); }
      change();
      return Promise.resolve();
    };
    rec.clearContext = () => { tools.clear(); change(); return Promise.resolve(); };
    rec.getTools = () => Promise.resolve([...tools.values()].map((t) => ({ name: t.name, description: t.description, inputSchema: t.inputSchema })));
    rec.executeTool = () => Promise.reject(new Error(NEVER_EXECUTE));
    rec.addEventListener = (...a) => target.addEventListener(...a);
    rec.removeEventListener = (...a) => target.removeEventListener(...a);
    rec.dispatchEvent = (...a) => target.dispatchEvent(...a);
    Object.defineProperty(rec, '__sightmapAtlasRecorder', { value: true });
    return rec;
  };

  // ---- surfaces the page brought itself -----------------------------------
  //
  // Anything that is not our recorder — a native implementation, a page
  // polyfill assigned over the accessor, or one defined over it — is wrapped
  // once so live calls are recorded, and enumerated so registrations that
  // already happened out of sight are not lost.

  const wrapped = new WeakSet();
  // Surfaces we got hold of before a single page script ran: every
  // registration on one of those is seen live, so it never needs enumerating.
  const preempted = new WeakSet();
  const seenReplaced = new Set();

  const isThenable = (v) => !!v && (typeof v === 'object' || typeof v === 'function') && typeof v.then === 'function';

  const normalize = (t) => {
    if (!t || typeof t !== 'object') return null;
    let schema = t.inputSchema !== undefined ? t.inputSchema : (t.input_schema !== undefined ? t.input_schema : t.schema);
    if (typeof schema === 'string') { try { schema = JSON.parse(schema); } catch { schema = undefined; } }
    const name = t.name != null ? t.name : (t.toolName != null ? t.toolName : undefined);
    if (name == null || name === '') return null;
    return { name, description: t.description, inputSchema: schema, execute: t.execute };
  };

  const recordList = (api, list) => {
    for (const raw of list || []) {
      const def = normalize(raw);
      if (!def) continue;
      const k = api + '\\u0000' + String(def.name);
      if (seenReplaced.has(k)) continue;
      seenReplaced.add(k);
      record(api, 'replaced-surface', def);
    }
  };

  const defer = (api, p) => {
    try { p.then((v) => { if (Array.isArray(v)) recordList(api, v); }, () => {}); } catch {}
  };

  const asArray = (v) => {
    if (!v || isThenable(v)) return null;
    if (Array.isArray(v)) return v;
    if (typeof v.forEach === 'function' && typeof v.size === 'number') { const out = []; try { v.forEach((x) => out.push(x)); } catch { return null; } return out; }
    if (typeof v === 'object') { const out = Object.values(v); return out.length && out.every((x) => x && typeof x === 'object') ? out : null; }
    return null;
  };

  // Whatever the replacement is willing to tell us about its tools, in the
  // order that costs least and is most likely to be synchronous. A method that
  // hands back a promise is not awaited here — the collect script is a plain
  // expression — it is left to settle into the record for the next poll.
  const enumerate = (api, obj) => {
    let deferred = false;
    for (const key of ['getToolInfos', 'getRegisteredToolInfos', 'getTools', 'listTools', 'tools']) {
      let v;
      try { v = typeof obj[key] === 'function' ? obj[key]() : obj[key]; } catch { continue; }
      if (isThenable(v)) { defer(api, v); deferred = true; continue; }
      const arr = asArray(v);
      if (arr && arr.length) { recordList(api, arr); return true; }
    }
    try {
      const shim = navigator.modelContextTesting;
      if (shim && typeof shim.listTools === 'function') {
        const v = shim.listTools();
        if (isThenable(v)) { defer(api, v); deferred = true; }
        else { const arr = asArray(v); if (arr && arr.length) { recordList(api, arr); return true; } }
      }
    } catch {}
    return deferred;
  };

  const adopt = (api, obj) => {
    if (!obj || (typeof obj !== 'object' && typeof obj !== 'function')) return;
    if (obj.__sightmapAtlasRecorder) return;
    if (wrapped.has(obj)) return;
    wrapped.add(obj);
    if (isNative(obj.registerTool) || isNative(obj.provideContext)) state.native[api] = true;
    for (const method of ['registerTool', 'provideContext']) {
      let orig;
      try { orig = obj[method]; } catch { continue; }
      if (typeof orig !== 'function') continue;
      try {
        Object.defineProperty(obj, method, {
          configurable: true, writable: true, enumerable: false,
          value: function (arg, ...rest) {
            try {
              // Seen live, so a later enumeration of the same surface must not
              // record it a second time.
              const seen = (d) => { if (d && d.name != null) seenReplaced.add(api + '\\u0000' + String(d.name)); };
              if (method === 'registerTool') { record(api, method, arg); seen(arg); }
              else for (const def of (arg && Array.isArray(arg.tools)) ? arg.tools : []) { record(api, method, def); seen(def); }
            } catch (e) { note('wrap ' + api + '.' + method + ': ' + e); }
            return orig.apply(this, [arg, ...rest]);
          },
        });
      } catch (e) { note('wrap ' + api + '.' + method + ': ' + e); }
    }
    // Whoever the surface belongs to, execution stays off during a scan.
    try {
      if (typeof obj.executeTool === 'function') {
        Object.defineProperty(obj, 'executeTool', {
          configurable: true, writable: true, enumerable: false,
          value: () => Promise.reject(new Error(NEVER_EXECUTE)),
        });
      }
    } catch {}
  };

  // Reconciles the recorder with whatever is live on the two hosts right now.
  // Cheap and idempotent: safe to call on every settle poll and again at
  // collect time.
  const hosts = { navigator, document };
  state.sync = () => {
    const done = new Set();
    for (const api of ['navigator', 'document']) {
      let live;
      try { live = hosts[api].modelContext; } catch { continue; }
      if (!live || live.__sightmapAtlasRecorder) continue;
      if (done.has(live)) continue;
      done.add(live);
      const fresh = !wrapped.has(live);
      adopt(api, live);
      if (preempted.has(live)) continue;
      if (fresh) note('page replaced the ' + api + '.modelContext surface; enumerating it');
      try { enumerate(api, live); } catch (e) { note('enumerate ' + api + ': ' + e); }
    }
  };

  const install = (api, host) => {
    try {
      let existing;
      try { existing = host.modelContext; } catch { existing = undefined; }
      if (existing && (isNative(existing.registerTool) || isNative(existing.provideContext))) {
        state.native[api] = true;
        adopt(api, existing);
        preempted.add(existing);
        return;
      }
      let current = makeRecorder(api);
      Object.defineProperty(host, 'modelContext', {
        configurable: true,
        enumerable: true,
        get() { return current; },
        // A page that assigns its own polyfill over ours is still recorded:
        // the replacement becomes the live surface and is wrapped in place.
        set(v) {
          current = v;
          try { adopt(api, v); enumerate(api, v); } catch (e) { note('adopt ' + api + ': ' + e); }
        },
      });
    } catch (e) { note('install ' + api + ': ' + e); }
  };

  install('navigator', navigator);
  install('document', document);

  // A polyfill or SDK that never arrives looks exactly like a site with no
  // WebMCP surface. Say which script failed instead.
  try {
    addEventListener('error', (e) => {
      try {
        const el = e && e.target;
        if (!el || el === globalThis || typeof el.tagName !== 'string') return;
        const src = el.src || el.href || '';
        if (!src) return;
        if (el.tagName !== 'SCRIPT' && !/\\.m?js($|[?#])/i.test(src)) return;
        note('script failed to load: ' + str(src, 300));
      } catch {}
    }, true);
  } catch {}
})();`

/**
 * The cheap poll the driver runs while the page settles: reconciles the
 * recorder with any surface the page swapped in, then answers with the number
 * of distinct tool names visible so far. The driver stops waiting once that
 * number holds still.
 */
export const COUNT_SCRIPT = `(() => {
  const KEY = ${JSON.stringify(RECORDER_KEY)};
  const state = globalThis[KEY];
  if (!state) return -1;
  try { if (state.sync) state.sync(); } catch {}
  const names = new Set();
  try { for (const r of state.records) if (r && r.name) names.add(r.name); } catch {}
  try { for (const f of document.querySelectorAll('form[toolname]')) names.add(f.getAttribute('toolname')); } catch {}
  return names.size;
})()`

/**
 * Runs in the page after load: drains the records and reads declarative
 * tools off the DOM. Returns plain JSON.
 */
export const COLLECT_SCRIPT = `(() => {
  const KEY = ${JSON.stringify(RECORDER_KEY)};
  const state = globalThis[KEY] || { records: [], native: { navigator: false, document: false }, errors: ['recorder missing'] };
  try { if (state.sync) state.sync(); } catch (e) { state.errors.push('sync: ' + e); }
  const text = (v, max) => { const s = v == null ? '' : String(v); return s.length > max ? s.slice(0, max) : s; };
  const declarative = [];
  try {
    for (const form of document.querySelectorAll('form[toolname]')) {
      if (declarative.length >= 200) break;
      const props = {};
      const required = [];
      for (const el of form.querySelectorAll('input[name], select[name], textarea[name]')) {
        const name = el.getAttribute('name');
        if (!name || props[name]) continue;
        const type = el.tagName === 'SELECT' ? 'string' : (el.getAttribute('type') === 'number' ? 'number' : (el.getAttribute('type') === 'checkbox' ? 'boolean' : 'string'));
        props[name] = { type, description: text(el.getAttribute('toolparamdescription') || el.getAttribute('placeholder') || el.getAttribute('aria-label'), 500) };
        if (el.hasAttribute('required')) required.push(name);
      }
      declarative.push({
        name: text(form.getAttribute('toolname'), 200),
        description: text(form.getAttribute('tooldescription'), 4000),
        inputSchema: { type: 'object', properties: props, required },
      });
    }
  } catch (e) { state.errors.push('declarative: ' + e); }
  const links = [];
  try {
    for (const a of document.querySelectorAll('nav a[href], header a[href], main a[href], a[href]')) {
      if (links.length >= 150) break;
      const href = a.getAttribute('href');
      if (href) links.push(href);
    }
  } catch {}
  const forms = [];
  try {
    for (const f of document.querySelectorAll('form:not([toolname])')) {
      if (forms.length >= 20) break;
      const fields = [];
      for (const el of f.querySelectorAll('input[name], select[name], textarea[name]')) {
        const t = (el.getAttribute('type') || '').toLowerCase();
        if (t === 'hidden' || t === 'password' || t === 'submit') continue;
        fields.push(text(el.getAttribute('name'), 100));
        if (fields.length >= 12) break;
      }
      forms.push({ action: text(f.getAttribute('action'), 500), method: text(f.getAttribute('method') || 'get', 10).toLowerCase(), fields });
    }
  } catch {}
  const meta = document.querySelector('meta[name="description"]');
  let status = null;
  try { const nav = performance.getEntriesByType('navigation')[0]; if (nav && typeof nav.responseStatus === 'number' && nav.responseStatus > 0) status = nav.responseStatus; } catch {}
  return {
    url: String(location.href),
    status,
    records: state.records.splice(0),
    native: state.native,
    errors: state.errors.splice(0),
    declarative,
    links,
    forms,
    title: text(document.title, 300),
    description: text(meta ? meta.getAttribute('content') : '', 1000),
  };
})()`
