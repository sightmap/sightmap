// Boots the Atlas's own WebMCP tool layer in the browser.
//
// scripts/build-sightkick.ts compiles .sightkick/tools.yaml against the site's
// .sightmap/ corpus into two files under public/atlas/. This loads them, on the
// Atlas pages only:
//
//   1. inject <script src="/atlas/sightkick-runtime.js">, which installs
//      window.__sightkick (and polyfills document.modelContext when the browser
//      has no native one), then
//   2. fetch /atlas/tools.ir.json and hand it to window.__sightkick.load(ir).
//
// The IR is loaded *after* the bundle rather than parked on window.__sightkick_ir
// first, because the runtime only reads that global at its own boot: it is
// synchronous, and we cannot have the JSON in hand before the script tag runs
// without blocking. `load()` is the documented path for exactly this case, and
// it re-registers view-scoped tools on SPA navigation on its own.
//
// Everything here is best-effort. A build that skipped the layer serves neither
// file; both requests 404 and this returns quietly. Nothing on the page depends
// on it, so no failure is worth a console error, let alone an exception.

const RUNTIME_SRC = '/atlas/sightkick-runtime.js'
const IR_SRC = '/atlas/tools.ir.json'

/** The runtime's global, as much of it as this module touches. */
interface SightkickGlobal {
  load(ir: unknown): void
}

declare global {
  interface Window {
    __sightkick?: SightkickGlobal
  }
}

/** Only /atlas pages carry tools, so only they pay for the two requests. */
function onAtlas(pathname: string): boolean {
  return pathname === '/atlas' || pathname.startsWith('/atlas/')
}

let started = false

function injectRuntime(): Promise<void> {
  return new Promise((resolve, reject) => {
    const el = document.createElement('script')
    el.src = RUNTIME_SRC
    el.async = true
    el.dataset.sightkick = 'runtime'
    el.onload = () => resolve()
    el.onerror = () => reject(new Error(`could not load ${RUNTIME_SRC}`))
    document.head.appendChild(el)
  })
}

/**
 * Load the tool layer. Safe to call more than once — the second call is a no-op.
 * Runs in the browser only: scripts/prerender.tsx renders this app under tsx
 * with no DOM, and the `typeof window` guard is what keeps it out of that pass.
 */
export function bootSightkick(): void {
  if (typeof window === 'undefined' || typeof document === 'undefined') return
  if (started || !onAtlas(window.location.pathname)) return
  started = true

  void (async () => {
    try {
      const [, ir] = await Promise.all([
        injectRuntime(),
        fetch(IR_SRC).then((res) => (res.ok ? res.json() : Promise.reject(new Error(`HTTP ${res.status}`)))),
      ])
      window.__sightkick?.load(ir)
    } catch {
      // No tool layer in this build (or it failed to load). The page is
      // unchanged either way; an agent simply finds no tools registered.
    }
  })()
}
