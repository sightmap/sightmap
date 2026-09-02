// Everything a build script needs to know about the deployed site. Scripts run
// in Node and cannot read src/index.css, so anything duplicated from the app
// lives here once rather than in each script.

export const SITE_URL = 'https://sightmap.org'
export const SITE_NAME = 'Sightmap'
export const SITE_DESCRIPTION =
  'An open YAML spec and CLI that maps views, components, and API requests to source files, with memory for runtime behavior.'

// The blog's own tagline, distinct from SITE_DESCRIPTION (which is the
// CLI/spec pitch). Used for /blog's meta description, the copy under the /blog
// heading, and the RSS <channel><description> — feed readers show that as the
// feed's subtitle, and subscribers came for the blog, not the CLI.
//
// src/pages/BlogIndex.tsx imports this, which is the one place app code reaches
// into scripts/. It is deliberate: this module is the single source of site
// copy, and duplicating the line into src/ is exactly the drift this finding
// was about. Keep this file free of node: imports and side effects so it stays
// safe to pull into the browser bundle.
export const BLOG_DESCRIPTION =
  'Research and release notes from the people building the sightmap spec.'

// The atlas gallery's tagline, distinct again from both of the above: the
// atlas is community-contributed maps of other people's sites, not the spec
// pitch and not the blog. Used for /atlas's meta description and the copy
// under its heading, and imported by src/pages/AtlasIndex.tsx for the same
// single-source reason BLOG_DESCRIPTION is.
// Sits directly under the /atlas heading ("Sightmaps of real sites"), so it
// must not restate it — and it has to stand alone as the page's meta
// description and its llms.txt section summary.
export const ATLAS_DESCRIPTION =
  'Community-contributed maps of views, components, and requests, made by browsing the live site with no source access.'

// Per-route <title> builders. scripts/prerender.tsx (the source of truth
// crawlers and unfurlers read) and src/components/Seo.tsx (which fixes the
// tab title on client-side navigation, since useEffect never runs during
// prerender's renderToString) both build titles from these instead of
// inlining their own template strings, so the two can't drift into different
// titles for the same route.
export const HOME_TITLE = `${SITE_NAME} — runtime context for agents using your web app`
export const BLOG_INDEX_TITLE = `Blog — ${SITE_NAME}`
export const ATLAS_INDEX_TITLE = `Atlas — ${SITE_NAME}`
// Read by both src/pages/NotFound.tsx and the dist/404.html prerender. The
// prerendered head is what a crawler sees and the <Seo> tag is what a visitor
// gets after hydration; sharing the strings keeps a single page from
// advertising two different titles depending on who asked.
export const NOT_FOUND_TITLE = `Page not found — ${SITE_NAME}`
export const NOT_FOUND_DESCRIPTION = "This page doesn't exist."
export const DEVELOPERS_TITLE = `${SITE_NAME} developer resources`
export const DEVELOPERS_DESCRIPTION =
  'OpenAPI, the Atlas HTTP API, documentation, CLI, and agent skills for the Sightmap spec.'
// The immersive tour at /building: the app-as-a-building metaphor, rendered as
// a scroll-driven 3D scene. Read by scripts/prerender.tsx, src/pages/Building.tsx,
// and the sitemap / llms.txt / markdown-twin generators.
export const BUILDING_TITLE = `The Building — how ${SITE_NAME} works`
export const BUILDING_DESCRIPTION =
  'An interactive tour of how Sightmap works: code is the blueprint, the running app is the building, a sightmap is its wayfinding, and agents are the people moving through it.'
export const postTitle = (title: string): string => `${title} — ${SITE_NAME}`
// An atlas entry's own title carries the mapped site's name, so the suffix
// says which gallery it came from rather than repeating the site name twice.
export const atlasTitle = (name: string): string => `${name} — ${SITE_NAME} Atlas`

// Escape for use inside an HTML attribute value. Ampersand goes first so the
// entities introduced by the later replacements are not themselves escaped.
export const esc = (s: string): string =>
  s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')

// XML text nodes additionally require apostrophe escaping.
export const escXml = (s: string): string => esc(s).replace(/'/g, '&apos;')
