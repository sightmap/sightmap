import { useMemo, useState } from 'react'
import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import BlogCard from '@/components/BlogCard'
import BlogFilters from '@/components/blog/BlogFilters'
import Seo from '@/components/Seo'
import { blogPosts } from '@/generated/blog-manifest'
// Shared with scripts/prerender.tsx (this page's meta description/title) and
// scripts/generate-feeds.ts (the RSS channel description) — see the note on
// BLOG_DESCRIPTION for why the constant lives there.
import { BLOG_DESCRIPTION, BLOG_INDEX_TITLE } from '../../scripts/lib/site'

export default function BlogIndex() {
  const posts = useMemo(() => blogPosts.filter((p) => !p.draft), [])
  const topics = useMemo(() => [...new Set(posts.map((p) => p.topic))].sort(), [posts])

  const [activeTopic, setActiveTopic] = useState<string | null>(null)
  const [query, setQuery] = useState('')

  const visible = posts.filter((post) => {
    if (activeTopic && post.topic !== activeTopic) return false
    if (!query.trim()) return true
    const haystack = `${post.title} ${post.excerpt}`.toLowerCase()
    return haystack.includes(query.trim().toLowerCase())
  })

  return (
    <>
      <Seo title={BLOG_INDEX_TITLE} description={BLOG_DESCRIPTION} />
      <Navigation />
      <main className="blog-index" data-component="BlogIndex">
        <div className="container">
          <div className="blog-index__header">
            <div className="section-label">Blog</div>
            <h1>Notes on the map</h1>
            <p className="section-desc">{BLOG_DESCRIPTION}</p>
          </div>
          <BlogFilters
            topics={topics}
            activeTopic={activeTopic}
            onTopicChange={setActiveTopic}
            query={query}
            onQueryChange={setQuery}
          />
          {visible.length === 0 ? (
            <p className="blog-index__empty">No posts match that filter.</p>
          ) : (
            <div className="blog-cards">
              {visible.map((post) => (
                <BlogCard key={post.slug} post={post} />
              ))}
            </div>
          )}
        </div>
      </main>
      <Footer />
    </>
  )
}
