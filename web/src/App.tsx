import { Routes, Route } from 'react-router'
import Home from '@/pages/Home'
import BlogIndex from '@/pages/BlogIndex'
import BlogPost from '@/pages/BlogPost'
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
        <Route path="*" element={<NotFound />} />
      </Routes>
      <ConsentUI />
    </ConsentProvider>
  )
}
