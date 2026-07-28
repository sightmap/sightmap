import { describe, it, expect } from 'vitest'
import { renderMeta, renderInlinePostJson } from './prerender'

// Mirrors the real head in web/index.html (see the meta-tag audit in the
// task report) so tests exercise every tag renderMeta touches, not just the
// reduced set from the original brief.
const SHELL = `<!DOCTYPE html><html><head>
<title>Old title</title>
<meta name="description" content="old">
<link rel="canonical" href="https://sightmap.org/">
<meta property="og:title" content="old">
<meta property="og:description" content="old">
<meta property="og:type" content="website">
<meta property="og:site_name" content="Sightmap">
<meta property="og:url" content="https://sightmap.org/">
<meta property="og:image" content="https://sightmap.org/og-image.png">
<meta property="og:image:type" content="image/png">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">
<meta property="og:image:alt" content="old alt">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="old">
<meta name="twitter:description" content="old">
<meta name="twitter:image" content="https://sightmap.org/og-image.png">
<meta name="twitter:image:alt" content="old alt">
</head><body><div id="root"></div></body></html>`

describe('renderMeta', () => {
  it('rewrites title, description, canonical, and og:url', () => {
    const out = renderMeta(SHELL, {
      url: 'https://sightmap.org/blog/x',
      ogUrl: 'https://sightmap.org/blog/x',
      title: 'A post — Sightmap',
      description: 'About the post.',
      image: 'https://sightmap.org/blog/og/x.png',
      ogImage: 'https://sightmap.org/blog/og/x.png',
      imageAlt: 'A post',
      imageDimensionsKnown: true,
      type: 'article',
    })
    expect(out).toContain('<title>A post — Sightmap</title>')
    expect(out).toContain('content="About the post."')
    expect(out).toContain('href="https://sightmap.org/blog/x"')
    expect(out).toContain('property="og:url" content="https://sightmap.org/blog/x"')
    expect(out).toContain('property="og:type" content="article"')
    expect(out).not.toContain('Old title')
  })

  it('escapes quotes in titles so the attribute cannot break out', () => {
    const out = renderMeta(SHELL, {
      url: 'https://sightmap.org/',
      ogUrl: 'https://sightmap.org/',
      title: 'He said "hi"',
      description: 'x',
      image: 'https://sightmap.org/og-image.png',
      ogImage: 'https://sightmap.org/og-image.png',
      imageAlt: 'x',
      imageDimensionsKnown: true,
      type: 'website',
    })
    expect(out).toContain('content="He said &quot;hi&quot;"')
  })

  it('rewrites twitter:description independently of og:description', () => {
    const out = renderMeta(SHELL, {
      url: 'https://sightmap.org/blog',
      ogUrl: 'https://sightmap.org/blog',
      title: 'Blog — Sightmap',
      description: 'Research and release notes.',
      image: 'https://sightmap.org/og-image.png',
      ogImage: 'https://sightmap.org/og-image.png',
      imageAlt: 'x',
      imageDimensionsKnown: true,
      type: 'website',
    })
    expect(out).toContain('name="twitter:description" content="Research and release notes."')
    expect(out).toContain('property="og:description" content="Research and release notes."')
    // The homepage's stale "old" description must not survive on either tag.
    expect(out).not.toContain('content="old"')
  })

  it('rewrites og:image:alt and twitter:image:alt per route', () => {
    const out = renderMeta(SHELL, {
      url: 'https://sightmap.org/blog/x',
      ogUrl: 'https://sightmap.org/blog/x',
      title: 'A post — Sightmap',
      description: 'About the post.',
      image: 'https://sightmap.org/blog/og/x.png',
      ogImage: 'https://sightmap.org/blog/og/x.png',
      imageAlt: 'A post — Sightmap',
      imageDimensionsKnown: true,
      type: 'article',
    })
    expect(out).toContain('property="og:image:alt" content="A post — Sightmap"')
    expect(out).toContain('name="twitter:image:alt" content="A post — Sightmap"')
    expect(out).not.toContain('old alt')
  })

  it('keeps og:image:type/width/height when the image is a known-dimension card', () => {
    const out = renderMeta(SHELL, {
      url: 'https://sightmap.org/',
      ogUrl: 'https://sightmap.org/',
      title: 'Sightmap',
      description: 'x',
      image: 'https://sightmap.org/og-image.png',
      ogImage: 'https://sightmap.org/og-image.png',
      imageAlt: 'x',
      imageDimensionsKnown: true,
      type: 'website',
    })
    expect(out).toContain('property="og:image:type" content="image/png"')
    expect(out).toContain('property="og:image:width" content="1200"')
    expect(out).toContain('property="og:image:height" content="630"')
  })

  it('drops og:image:type/width/height when the image is not a known-dimension card', () => {
    const out = renderMeta(SHELL, {
      url: 'https://sightmap.org/blog/x',
      ogUrl: 'https://sightmap.org/blog/x',
      title: 'A post — Sightmap',
      description: 'x',
      image: 'https://sightmap.org/blog/hero/x.jpg',
      ogImage: 'https://sightmap.org/blog/hero/x.jpg',
      imageAlt: 'x',
      imageDimensionsKnown: false,
      type: 'article',
    })
    expect(out).not.toContain('og:image:type')
    expect(out).not.toContain('og:image:width')
    expect(out).not.toContain('og:image:height')
    // og:image:alt uses the same "og:image:" prefix — make sure it survives
    // the strip, i.e. the regexes above are specific to the tag they target.
    expect(out).toContain('og:image:alt')
  })

  // Deploy-preview regression: a preview build resolves `ogUrl`/`ogImage`
  // from DEPLOY_PRIME_URL while `url`/`image` stay pinned to SITE_URL (see
  // scripts/prerender.tsx). Canonical must not follow og:url/og:image onto the
  // preview host; the two card-image tags must.
  it('sends both card images to the deploy host while canonical stays on production', () => {
    const out = renderMeta(SHELL, {
      url: 'https://sightmap.org/blog/x',
      ogUrl: 'https://deploy-preview-1--sightmap.netlify.app/blog/x',
      title: 'A post — Sightmap',
      description: 'About the post.',
      image: 'https://sightmap.org/blog/og/x.png',
      ogImage: 'https://deploy-preview-1--sightmap.netlify.app/blog/og/x.png',
      imageAlt: 'A post',
      imageDimensionsKnown: true,
      type: 'article',
    })
    expect(out).toContain('href="https://sightmap.org/blog/x"')
    expect(out).toContain(
      'property="og:url" content="https://deploy-preview-1--sightmap.netlify.app/blog/x"'
    )
    expect(out).toContain(
      'property="og:image" content="https://deploy-preview-1--sightmap.netlify.app/blog/og/x.png"'
    )
    // twitter:image tracks og:image, not canonical. A card image is an asset
    // the unfurling client fetches, so both tags must name a host that can
    // actually serve it — otherwise the same page unfurls differently
    // depending on which tag the client reads, and a preview-only card is
    // invisible to anything reading twitter:image.
    expect(out).toContain(
      'name="twitter:image" content="https://deploy-preview-1--sightmap.netlify.app/blog/og/x.png"'
    )
    expect(out).not.toContain(
      'name="twitter:image" content="https://sightmap.org/blog/og/x.png"'
    )
  })
})

describe('renderInlinePostJson', () => {
  it('round-trips a plain post body with no escaping needed', () => {
    const tag = renderInlinePostJson('plain', '<p>hello</p>')
    const payload = tag.slice(tag.indexOf('>') + 1, tag.lastIndexOf('</script>'))
    const parsed = JSON.parse(payload) as { slug: string; html: string }
    expect(parsed).toEqual({ slug: 'plain', html: '<p>hello</p>' })
  })

  it('escapes </script> and <!-- in the post body so they cannot break out of the script tag, and round-trips exactly', () => {
    const slug = 'x'
    const html = 'before </script> middle <!-- comment --> after <script>alert(1)</script> end'
    const tag = renderInlinePostJson(slug, html)

    // The tag markup legitimately contains "<script ...>" once (to open) and
    // "</script>" once (to close). Isolate just the JSON payload between them
    // so the assertions below can't be satisfied by the wrapper markup.
    const payload = tag.slice(tag.indexOf('>') + 1, tag.lastIndexOf('</script>'))

    expect(payload).not.toContain('</script>')
    expect(payload).not.toContain('<!--')
    expect(payload).not.toContain('<script>')

    const parsed = JSON.parse(payload) as { slug: string; html: string }
    expect(parsed.slug).toBe(slug)
    expect(parsed.html).toBe(html)
  })
})
