// Fails the build when a route declared in src/App.tsx has no prerendered page
// in dist/. Without this, forgetting to add a new route to scripts/prerender.tsx
// would silently give it a hard 404, because netlify.toml's catch-all is no
// longer a 200 that lets the SPA shell render it client-side.
//
// Checks the built output rather than a parallel list of routes. The Subtext
// site's equivalent compares App.tsx against exported PUBLIC_ROUTES /
// INTERNAL_ROUTES arrays; this repo has no such registry — prerender.tsx writes
// its routes inline — and reading dist/ is the stronger check anyway, since it
// verifies the artifact that actually ships instead of a list that claims to
// describe it.
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'

const DIST = path.resolve('dist')
const APP = path.resolve('src/App.tsx')

// Regex-based, not an AST parse: sufficient for this codebase's
// one-route-per-line formatting in App.tsx, but it would miss a route path
// split across lines or built from a template literal rather than a string
// literal.
export function extractRoutePaths(appSource: string): string[] {
  return [...appSource.matchAll(/<Route\s+path="([^"]+)"/g)].map((m) => m[1])
}

// Every directory under dist/ holding an index.html, expressed as the route it
// answers. dist/index.html is '/', dist/blog/index.html is '/blog', and so on.
export function prerenderedRoutes(distDir: string): string[] {
  const found: string[] = []
  const walk = (dir: string, route: string) => {
    if (fs.existsSync(path.join(dir, 'index.html'))) found.push(route || '/')
    for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
      if (e.isDirectory()) walk(path.join(dir, e.name), `${route}/${e.name}`)
    }
  }
  if (fs.existsSync(distDir)) walk(distDir, '')
  return found
}

export interface Coverage {
  // Static routes with no prerendered file. These are build failures: the
  // route exists in the app and would answer 404 in production.
  uncovered: string[]
  // Dynamic routes (/blog/:slug) with no concrete page under their prefix.
  // Reported but NOT a failure — zero instances is a legitimate state. On a
  // production build drafts are excluded, so a blog with nothing published
  // should have every /blog/<slug> URL 404, which is exactly what happens.
  emptyDynamic: string[]
}

export function checkCoverage(declared: string[], prerendered: string[]): Coverage {
  const pre = new Set(prerendered)
  const uncovered: string[] = []
  const emptyDynamic: string[] = []

  for (const route of declared) {
    // The catch-all is what dist/404.html answers; it has no page of its own.
    if (route === '*') continue
    if (route.includes(':')) {
      // prefix keeps its trailing slash ('/blog/'), so the index route
      // '/blog' does not count as an instance of '/blog/:slug'.
      const prefix = route.slice(0, route.indexOf(':'))
      if (!prerendered.some((p) => p.startsWith(prefix))) emptyDynamic.push(route)
      continue
    }
    if (!pre.has(route)) uncovered.push(route)
  }
  return { uncovered, emptyDynamic }
}

function main() {
  const declared = extractRoutePaths(fs.readFileSync(APP, 'utf-8'))
  if (declared.length === 0) {
    console.error('\nRoute coverage check failed: no <Route path="..."> found in src/App.tsx.')
    console.error('The regex in extractRoutePaths likely no longer matches the file.\n')
    process.exit(1)
  }

  const { uncovered, emptyDynamic } = checkCoverage(declared, prerenderedRoutes(DIST))

  // netlify.toml points its catch-all at this file. If it is missing, every
  // unknown URL gets Netlify's default 404 body instead of the site's.
  if (!fs.existsSync(path.join(DIST, '404.html'))) {
    console.error('\nRoute coverage check failed: dist/404.html is missing.')
    console.error("netlify.toml's catch-all serves that exact path.\n")
    process.exit(1)
  }

  if (uncovered.length > 0) {
    console.error(
      `\nRoute coverage check failed. These routes in src/App.tsx have no prerendered page:\n` +
        uncovered.map((r) => `  ${r}`).join('\n') +
        `\n\nEvery route needs a prerendered file, because netlify.toml's catch-all now
returns 404 rather than the SPA shell. Add each route to scripts/prerender.tsx.\n`
    )
    process.exit(1)
  }

  for (const route of emptyDynamic) {
    console.log(`  route coverage: ${route} has no concrete pages (nothing published yet)`)
  }
  console.log(`  route coverage: ${declared.length} route(s) declared, all covered`)
}

const entry = process.argv[1]
if (entry && import.meta.url === pathToFileURL(entry).href) {
  main()
}
