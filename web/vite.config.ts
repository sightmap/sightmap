import { existsSync } from 'fs'
import { resolve } from 'path'
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
        if (existsSync(resolve(distDir, `.${pathname}/index.html`))) {
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
})
