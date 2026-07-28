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

// Per-route <title> builders. scripts/prerender.tsx (the source of truth
// crawlers and unfurlers read) and src/components/Seo.tsx (which fixes the
// tab title on client-side navigation, since useEffect never runs during
// prerender's renderToString) both build titles from these instead of
// inlining their own template strings, so the two can't drift into different
// titles for the same route.
export const HOME_TITLE = `${SITE_NAME} — runtime context for agents using your web app`
export const BLOG_INDEX_TITLE = `Blog — ${SITE_NAME}`
export const postTitle = (title: string): string => `${title} — ${SITE_NAME}`

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
