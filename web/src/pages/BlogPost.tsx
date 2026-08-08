import { useEffect, useMemo, useState } from 'react'
import { Link, Navigate, useParams, useSearchParams } from 'react-router'
import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import Seo from '@/components/Seo'
import { formatDate } from '@/components/BlogCard'
import { renderPostBody } from '@/components/blog/widgets'
import { readPostHtml, loadPostHtml } from '@/lib/postHtml'
import { useScrollToTopOnPush } from '@/lib/useScrollToTopOnPush'
import { blogPosts } from '@/generated/blog-manifest'
// Shared with scripts/prerender.tsx so a client-side navigation to a post and
// a fresh/prerendered load of that same post produce an identical <title>.
import { postTitle } from '../../scripts/lib/site'

export default function BlogPost() {
  const { slug = '' } = useParams()
  const [searchParams] = useSearchParams()
  const isPreview = searchParams.get('preview') === 'true'
  const meta = blogPosts.find((p) => p.slug === slug)

  // Seeded synchronously from the prerendered page so first render matches the
  // server markup; null only when navigating client-side to another post.
  // The seed is bound to the slug it was read for so that client-side
  // navigation between posts (which reuses this component instance without
  // remounting) doesn't keep showing the previous post's cached body.
  const [entry, setEntry] = useState<{ slug: string; html: string | null }>(() => ({
    slug,
    html: readPostHtml(slug),
  }))
  const [loadFailed, setLoadFailed] = useState(false)

  // Only trust the cached html when it belongs to the slug currently routed.
  const html = entry.slug === slug ? entry.html : null

  useEffect(() => {
    // Hooks run before the <Navigate> guards below, so without the `meta`
    // check an unknown slug fires a dynamic import that can never resolve and
    // logs a load error on its way to the redirect.
    if (!meta || html !== null) return
    let cancelled = false
    setLoadFailed(false)
    loadPostHtml(slug)
      .then((loaded) => {
        if (!cancelled) setEntry({ slug, html: loaded })
      })
      .catch((err) => {
        if (cancelled) return
        console.error(`Failed to load post "${slug}"`, err)
        setLoadFailed(true)
      })
    return () => {
      cancelled = true
    }
  }, [slug, html, meta])

  const body = useMemo(() => (html ? renderPostBody(html) : null), [html])

  // A click from halfway down /blog would otherwise open the post halfway down.
  // Safe to run before the body has loaded: the reset lands while the page is
  // still header-only, and the article growing underneath does not move it.
  useScrollToTopOnPush()

  if (!meta) return <Navigate to="/blog" replace />
  if (meta.draft && !isPreview) return <Navigate to="/blog" replace />

  return (
    <>
      <Seo title={postTitle(meta.title)} description={meta.excerpt} />
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
          {loadFailed ? (
            <p className="blog-post__byline">Couldn&rsquo;t load this post. Try refreshing the page.</p>
          ) : body ? (
            <article className="prose">{body}</article>
          ) : null}
        </div>
      </main>
      <Footer />
    </>
  )
}
