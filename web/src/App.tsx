import { Routes, Route } from 'react-router'
import Home from '@/pages/Home'
import BlogIndex from '@/pages/BlogIndex'
import BlogPost from '@/pages/BlogPost'
import AtlasIndex from '@/pages/AtlasIndex'
import AtlasCity from '@/pages/AtlasCity'
import AtlasSlug from '@/pages/AtlasSlug'
import Developers from '@/pages/Developers'
import Building from '@/pages/Building'
import Sightkick from '@/pages/Sightkick'
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
        {/* Ahead of /atlas/:slug in the file for readers; react-router ranks
            the literal segment above the dynamic one whatever the order. */}
        <Route path="/atlas/city" element={<AtlasCity />} />
        {/* One route, two pages: AtlasSlug picks the WebMCP listing when the
            slug is in directoryListings and the community entry otherwise.
            Keep this on one literal line — check-route-coverage.ts reads the
            route table straight out of this file. */}
        <Route path="/atlas/:slug" element={<AtlasSlug />} />
        <Route path="/developers" element={<Developers />} />
        <Route path="/building" element={<Building />} />
        <Route path="/sightkick" element={<Sightkick />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
      <ConsentUI />
    </ConsentProvider>
  )
}
