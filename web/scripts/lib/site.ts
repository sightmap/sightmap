// Everything a build script needs to know about the deployed site. Scripts run
// in Node and cannot read src/index.css, so anything duplicated from the app
// lives here once rather than in each script.

export const SITE_URL = 'https://sightmap.org'
export const SITE_NAME = 'Sightmap'
export const SITE_DESCRIPTION =
  'An open YAML spec and CLI that maps views, components, and API requests to source files, with memory for runtime behavior.'

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
