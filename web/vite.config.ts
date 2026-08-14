import { existsSync } from 'fs'
import { resolve, sep } from 'path'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// scripts/prerender.tsx writes one real file per route (dist/blog/index.html,
// dist/blog/<slug>/index.html, ...). Netlify's static-file resolution serves
// those directly for an extensionless request like `/blog` (trying
// `/blog/index.html` before falling back to the netlify.toml catch-all
// redirect). `vite preview`'s static server (sirv) only makes that match when
// the URL already has a trailing slash — without one it falls through to the
// SPA history fallback and silently serves the homepage shell for `/blog`,
// which then hydrates the wrong route and looks like a hydration bug. This
// plugin redirects extensionless, slash-less paths to their trailing-slash
// form when a prerendered index.html exists, so `pnpm preview` matches
// Netlify's routing instead of masking it.
function prerenderedRouteRedirect(distDir: string): Plugin {
  return {
    name: 'prerendered-route-redirect',
    configurePreviewServer(server) {
      server.middlewares.use((req, res, next) => {
        const url = req.url ?? ''
        const [pathname, search = ''] = url.split('?')
        const lastSegment = pathname.split('/').pop() ?? ''
        const looksLikeFile = lastSegment.includes('.')
        if (pathname === '/' || pathname.endsWith('/') || looksLikeFile) {
          next()
          return
        }
        // `pathname` comes straight off the request line and can contain
        // "..". Resolve it against distDir first, then require the result to
        // stay inside distDir before touching the filesystem — comparing
        // against `distDir + sep` (not a bare startsWith(distDir)) so a
        // sibling directory that merely shares distDir as a string prefix
        // (e.g. "distXYZ") can't pass. Without this, a request like
        // `/../../../etc/passwd` clears both guards above (no trailing
        // slash, no dot in the last segment) and existsSync becomes a
        // file-existence oracle for the whole filesystem. Preview-only,
        // never ships in dist/, but there is no reason to leave it open.
        const candidate = resolve(distDir, `.${pathname}`, 'index.html')
        if (!candidate.startsWith(distDir + sep)) {
          next()
          return
        }
        if (existsSync(candidate)) {
          res.statusCode = 301
          res.setHeader('Location', `${pathname}/${search ? `?${search}` : ''}`)
          res.end()
          return
        }
        next()
      })
    },
  }
}

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    prerenderedRouteRedirect(resolve(__dirname, './dist')),
  ],
  resolve: {
    alias: {
      '@': resolve(__dirname, './src'),
    },
  },
  build: {
    // Emit .map files next to the bundles so a Replay recording of
    // sightmap.org resolves to the original TSX instead of a 460KB minified
    // blob (scripts/upload-sourcemaps.ts pushes them to Replay after the
    // build). `true` rather than 'hidden' — the site is MIT and its source is
    // already public on GitHub, so suppressing the sourceMappingURL comment
    // would hide nothing while costing plain DevTools the same mapping.
    sourcemap: true,
  },
})
