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

// The prerender (scripts/prerender.tsx) only runs for `pnpm build`; `vite dev`
// serves an empty `#root`. Choosing the mount strategy by whether markup is
// actually present — not by DEV/PROD mode — keeps real hydration for the
// built site and a clean client render in dev, instead of a permanent
// hydration-mismatch error on every dev session.
if (el.hasChildNodes()) {
  hydrateRoot(el, tree)
} else {
  createRoot(el).render(tree)
}
