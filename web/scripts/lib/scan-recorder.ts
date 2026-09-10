// The script the scanner installs in every document *before* any page script
// runs. It records WebMCP tool registrations without ever executing a tool.
//
// WebMCP has no page-side "list the tools" call — that is the agent's side of
// the API — so the only way to see what a page registers is to be the surface
// it registers against. Three cases:
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
//   3. Declarative tools (`<form toolname>`) are read off the DOM after load.
//
// `executeTool` on the recorder always rejects: discovery never runs
// site-defined code with site-chosen arguments.
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

  const isNative = (fn) => {
    try { return typeof fn === 'function' && /\\[native code\\]/.test(Function.prototype.toString.call(fn)); }
    catch { return false; }
  };
  const clone = (v) => { try { return v === undefined ? undefined : JSON.parse(JSON.stringify(v)); } catch { return null; } };
  const str = (v, max) => { const s = v == null ? '' : String(v); return s.length > max ? s.slice(0, max) : s; };
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
    } catch (e) { state.errors.push('record: ' + e); }
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
    rec.executeTool = () => Promise.reject(new Error('sightmap atlas scan: tools are enumerated, never executed'));
    rec.addEventListener = (...a) => target.addEventListener(...a);
    rec.removeEventListener = (...a) => target.removeEventListener(...a);
    rec.dispatchEvent = (...a) => target.dispatchEvent(...a);
    Object.defineProperty(rec, '__sightmapAtlasRecorder', { value: true });
    return rec;
  };

  const wrapNative = (api, mc) => {
    state.native[api] = true;
    for (const method of ['registerTool', 'provideContext']) {
      const orig = mc[method];
      if (typeof orig !== 'function') continue;
      try {
        Object.defineProperty(mc, method, {
          configurable: true, writable: true,
          value: function (arg, ...rest) {
            if (method === 'registerTool') record(api, method, arg);
            else for (const def of (arg && Array.isArray(arg.tools)) ? arg.tools : []) record(api, method, def);
            return orig.call(this, arg, ...rest);
          },
        });
      } catch (e) { state.errors.push('wrap ' + api + '.' + method + ': ' + e); }
    }
  };

  const install = (api, host) => {
    try {
      let existing;
      try { existing = host.modelContext; } catch { existing = undefined; }
      if (existing && (isNative(existing.registerTool) || isNative(existing.provideContext))) { wrapNative(api, existing); return; }
      Object.defineProperty(host, 'modelContext', { value: makeRecorder(api), configurable: true, writable: true });
    } catch (e) { state.errors.push('install ' + api + ': ' + e); }
  };

  install('navigator', navigator);
  install('document', document);
})();`

/**
 * Runs in the page after load: drains the records and reads declarative
 * tools off the DOM. Returns plain JSON.
 */
export const COLLECT_SCRIPT = `(() => {
  const KEY = ${JSON.stringify(RECORDER_KEY)};
  const state = globalThis[KEY] || { records: [], native: { navigator: false, document: false }, errors: ['recorder missing'] };
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
