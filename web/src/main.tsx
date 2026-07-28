import { StrictMode } from 'react'
import { createRoot, hydrateRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import App from '@/App'
import '@/index.css'

const el = document.getElementById('root')!
const tree = (
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>
)

// Trailing slashes are routing noise here: `vite preview` (and Netlify) serve
// a prerendered `/blog/index.html` at `/blog/`, while the prerender stamped it
// as `/blog`. React Router matches both, so normalize before comparing.
const normalize = (p: string): string => (p.length > 1 && p.endsWith('/') ? p.slice(0, -1) : p)

// Hydrate only when the markup in `#root` was rendered for *this* URL.
//
// "`#root` has children" is not the same question. `vite dev` serves an empty
// `#root`, so that test got the dev case right, but Netlify's catch-all
// (netlify.toml) serves the homepage `dist/index.html` for every route with no
// prerendered file — `/blog/<slug>?preview=true` for a draft, or any 404 path.
// Those arrive with a full homepage in `#root`, and hydrating the real route
// against it throws a mismatch and strands the homepage's <title>.
// scripts/prerender.tsx stamps the route it rendered onto `#root`; anything
// else is a fresh client render.
const stamp = el.dataset.prerenderRoute
if (stamp !== undefined && normalize(stamp) === normalize(location.pathname)) {
  hydrateRoot(el, tree)
} else {
  createRoot(el).render(tree)
}
