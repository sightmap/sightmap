import { Routes, Route } from 'react-router'
import Home from '@/pages/Home'
import BlogIndex from '@/pages/BlogIndex'
import BlogPost from '@/pages/BlogPost'
import AtlasIndex from '@/pages/AtlasIndex'
import AtlasEntry from '@/pages/AtlasEntry'
import Developers from '@/pages/Developers'
import Building from '@/pages/Building'
import NotFound from '@/pages/NotFound'
import { ConsentProvider } from '@/components/consent/ConsentContext'
import ConsentUI from '@/components/consent/ConsentUI'

// No <BrowserRouter> here on purpose: main.tsx supplies BrowserRouter for the
// client and scripts/prerender.tsx supplies StaticRouter at build time.
export default function App() {
  return (
    <ConsentProvider>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/blog" element={<BlogIndex />} />
        <Route path="/blog/:slug" element={<BlogPost />} />
        {/* Every route here needs a matching entry in scripts/prerender.tsx —
            one declared only in this file ships as a client-only page. */}
        <Route path="/atlas" element={<AtlasIndex />} />
        <Route path="/atlas/:slug" element={<AtlasEntry />} />
        <Route path="/developers" element={<Developers />} />
        <Route path="/building" element={<Building />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
      <ConsentUI />
    </ConsentProvider>
  )
}
