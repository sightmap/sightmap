import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import BlogCard from '@/components/BlogCard'
import { blogPosts } from '@/generated/blog-manifest'

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
            <p className="section-desc">
              Research and release notes from the people building the sightmap spec.
            </p>
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
