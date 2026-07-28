import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import BlogCard from '@/components/BlogCard'
import { blogPosts } from '@/generated/blog-manifest'
// Shared with scripts/prerender.tsx (this page's meta description) and
// scripts/generate-feeds.ts (the RSS channel description) — see the note on
// BLOG_DESCRIPTION for why the constant lives there.
import { BLOG_DESCRIPTION } from '../../scripts/lib/site'

export default function BlogIndex() {
  const posts = blogPosts.filter((p) => !p.draft)

  return (
    <>
      <Navigation />
      <main className="blog-index" data-component="BlogIndex">
        <div className="container">
          <div className="blog-index__header">
            <div className="section-label">Blog</div>
            <h1>Notes on the map</h1>
            <p className="section-desc">{BLOG_DESCRIPTION}</p>
          </div>
          {posts.length === 0 ? (
            <p className="blog-index__empty">No posts yet.</p>
          ) : (
            <div className="blog-cards">
              {posts.map((post) => (
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
