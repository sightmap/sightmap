import { useEffect, useMemo, useState } from 'react'
import { Link, Navigate, useParams, useSearchParams } from 'react-router'
import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import { formatDate } from '@/components/BlogCard'
import { renderPostBody } from '@/components/blog/widgets'
import { readPostHtml, loadPostHtml } from '@/lib/postHtml'
import { blogPosts } from '@/generated/blog-manifest'

export default function BlogPost() {
  const { slug = '' } = useParams()
  const [searchParams] = useSearchParams()
  const isPreview = searchParams.get('preview') === 'true'
  const meta = blogPosts.find((p) => p.slug === slug)

  // Seeded synchronously from the prerendered page so first render matches the
  // server markup; null only when navigating client-side to another post.
  const [html, setHtml] = useState<string | null>(() => readPostHtml(slug))

  useEffect(() => {
    if (html !== null) return
    let cancelled = false
    loadPostHtml(slug).then((loaded) => {
      if (!cancelled) setHtml(loaded)
    })
    return () => {
      cancelled = true
    }
  }, [slug, html])

  const body = useMemo(() => (html ? renderPostBody(html) : null), [html])

  if (!meta) return <Navigate to="/blog" replace />
  if (meta.draft && !isPreview) return <Navigate to="/blog" replace />

  return (
    <>
      {meta.draft && (
        <div className="blog-post__draft-banner">Draft preview — not published</div>
      )}
      <Navigation />
      <main className="blog-post" data-component="BlogPost">
        <div className="container">
          <Link to="/blog" className="blog-post__back">
            &larr; Back to blog
          </Link>
          <header className="blog-post__header">
            <div className="blog-card__meta">
              <span>{meta.topic}</span>
              <time dateTime={meta.date}>{formatDate(meta.date)}</time>
            </div>
            <h1>{meta.title}</h1>
            <div className="blog-post__byline">
              {meta.author} · {meta.readingTime} min read
            </div>
          </header>
          {body ? <article className="prose">{body}</article> : null}
        </div>
      </main>
      <Footer />
    </>
  )
}
