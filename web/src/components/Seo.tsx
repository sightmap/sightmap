import { useEffect } from 'react'

export interface SeoProps {
  title: string
  description: string
  /** Optional OG/Twitter title; defaults to `title`. */
  ogTitle?: string
}

/**
 * Per-route document metadata for the SPA. scripts/prerender.tsx already
 * writes correct per-route <title>/<meta> into each prerendered HTML file —
 * that's what crawlers and unfurlers read — so this component exists only to
 * fix *client-side* navigation (e.g. BlogCard's <Link>), where the browser tab
 * and history would otherwise keep showing the previous route's title.
 *
 * Updates document.title and the existing index.html <meta> tags in place (no
 * duplicate tags are created), then restores the previous values on unmount.
 * Runs in a useEffect, which never fires during react-dom/server's
 * renderToString, so this component is inert during prerender and cannot
 * cause a hydration mismatch — keep it that way; don't move this work into
 * render or a useState initializer.
 */
export default function Seo({ title, description, ogTitle }: SeoProps) {
  useEffect(() => {
    const ogt = ogTitle ?? title
    const targets: [string, string][] = [
      ['meta[name="description"]', description],
      ['meta[property="og:title"]', ogt],
      ['meta[property="og:description"]', description],
      ['meta[name="twitter:title"]', ogt],
      ['meta[name="twitter:description"]', description],
    ]
    const prevTitle = document.title
    const restore = targets.map(([sel, content]) => {
      const el = document.head.querySelector<HTMLMetaElement>(sel)
      const prev = el?.getAttribute('content') ?? null
      if (el) el.setAttribute('content', content)
      return { el, prev }
    })
    document.title = title
    return () => {
      document.title = prevTitle
      for (const { el, prev } of restore) {
        if (el && prev !== null) el.setAttribute('content', prev)
      }
    }
  }, [title, description, ogTitle])

  return null
}
