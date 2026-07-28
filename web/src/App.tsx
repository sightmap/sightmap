import { Routes, Route } from 'react-router'
import Home from '@/pages/Home'
import BlogIndex from '@/pages/BlogIndex'
import BlogPost from '@/pages/BlogPost'

// No <BrowserRouter> here on purpose: main.tsx supplies BrowserRouter for the
// client and scripts/prerender.tsx supplies StaticRouter at build time.
export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Home />} />
      <Route path="/blog" element={<BlogIndex />} />
      <Route path="/blog/:slug" element={<BlogPost />} />
    </Routes>
  )
}
