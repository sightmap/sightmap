import { describe, it, expect } from 'vitest'
import { renderMeta } from './prerender'

const SHELL = `<!DOCTYPE html><html><head>
<title>Old title</title>
<meta name="description" content="old">
<link rel="canonical" href="https://sightmap.org/">
<meta property="og:title" content="old">
<meta property="og:url" content="https://sightmap.org/">
<meta property="og:image" content="https://sightmap.org/og-image.png">
<meta property="og:type" content="website">
<meta name="twitter:title" content="old">
<meta name="twitter:image" content="https://sightmap.org/og-image.png">
</head><body><div id="root"></div></body></html>`

describe('renderMeta', () => {
  it('rewrites title, description, canonical, and og:url', () => {
    const out = renderMeta(SHELL, {
      url: 'https://sightmap.org/blog/x',
      title: 'A post — Sightmap',
      description: 'About the post.',
      image: 'https://sightmap.org/blog/og/x.png',
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
      title: 'He said "hi"',
      description: 'x',
      image: 'https://sightmap.org/og-image.png',
      type: 'website',
    })
    expect(out).toContain('content="He said &quot;hi&quot;"')
  })
})
