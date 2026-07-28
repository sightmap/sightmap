// Post-build static render. Vite has already written dist/index.html (the
// shell with hashed asset links); this renders the React tree into #root for
// every route and writes one static file per URL.
//
// Netlify serves an existing static file before applying a non-forced redirect,
// so these files win over the SPA catch-all in netlify.toml without any extra
// rule. That is the same mechanism the Subtext site relies on.
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { renderToString } from 'react-dom/server'
import { StaticRouter } from 'react-router'
import App from '../src/App'
import { loadPosts, renderPostHtml } from './lib/posts'
import { SITE_URL, SITE_NAME, SITE_DESCRIPTION, esc } from './lib/site'

const DIST = path.resolve('dist')
const CONTENT_DIR = path.resolve('content/blog')

export interface PageMeta {
  url: string
  title: string
  description: string
  image: string
  // Alt text for og:image / twitter:image. These describe the *image*, not
  // the page, so they get their own field rather than reusing title.
  imageAlt: string
  // True only when `image` is guaranteed to be the site's 1200x630 PNG
  // (currently: the og-image.png fallback used by the homepage and blog
  // index). Post images come from frontmatter `image`, which the zod schema
  // in scripts/lib/posts.ts only validates as "starts with /" — nothing
  // enforces its dimensions or format — so for those we cannot honestly claim
  // og:image:type/width/height and instead drop the three tags rather than
  // ship a false dimension/type claim paired with an image that may not
  // match it.
  imageDimensionsKnown: boolean
  type: 'website' | 'article'
}

// Deletes a whole meta-tag line (including its leading newline/indentation)
// from `html` if present, so removing a tag doesn't leave a blank line.
function stripTag(html: string, pattern: RegExp): string {
  return html.replace(pattern, '')
}

export function renderMeta(shell: string, meta: PageMeta): string {
  let html = shell
    .replace(/<title>[\s\S]*?<\/title>/, `<title>${esc(meta.title)}</title>`)
    .replace(
      /<meta\s+name="description"[\s\S]*?>/,
      `<meta name="description" content="${esc(meta.description)}">`
    )
    .replace(/<link\s+rel="canonical"[\s\S]*?>/, `<link rel="canonical" href="${meta.url}">`)
    .replace(/<meta\s+property="og:url"[\s\S]*?>/, `<meta property="og:url" content="${meta.url}">`)
    .replace(
      /<meta\s+property="og:type"[\s\S]*?>/,
      `<meta property="og:type" content="${meta.type}">`
    )
    .replace(
      /<meta\s+property="og:title"[\s\S]*?>/,
      `<meta property="og:title" content="${esc(meta.title)}">`
    )
    .replace(
      /<meta\s+property="og:description"[\s\S]*?>/,
      `<meta property="og:description" content="${esc(meta.description)}">`
    )
    .replace(
      /<meta\s+property="og:image"[\s\S]*?>/,
      `<meta property="og:image" content="${meta.image}">`
    )
    .replace(
      /<meta\s+property="og:image:alt"[\s\S]*?>/,
      `<meta property="og:image:alt" content="${esc(meta.imageAlt)}">`
    )
    .replace(
      /<meta\s+name="twitter:title"[\s\S]*?>/,
      `<meta name="twitter:title" content="${esc(meta.title)}">`
    )
    .replace(
      /<meta\s+name="twitter:description"[\s\S]*?>/,
      `<meta name="twitter:description" content="${esc(meta.description)}">`
    )
    .replace(
      /<meta\s+name="twitter:image"[\s\S]*?>/,
      `<meta name="twitter:image" content="${meta.image}">`
    )
    .replace(
      /<meta\s+name="twitter:image:alt"[\s\S]*?>/,
      `<meta name="twitter:image:alt" content="${esc(meta.imageAlt)}">`
    )

  if (meta.imageDimensionsKnown) {
    html = html
      .replace(
        /<meta\s+property="og:image:type"[\s\S]*?>/,
        `<meta property="og:image:type" content="image/png">`
      )
      .replace(
        /<meta\s+property="og:image:width"[\s\S]*?>/,
        `<meta property="og:image:width" content="1200">`
      )
      .replace(
        /<meta\s+property="og:image:height"[\s\S]*?>/,
        `<meta property="og:image:height" content="630">`
      )
  } else {
    html = stripTag(html, /\n[ \t]*<meta\s+property="og:image:type"[\s\S]*?>/)
    html = stripTag(html, /\n[ \t]*<meta\s+property="og:image:width"[\s\S]*?>/)
    html = stripTag(html, /\n[ \t]*<meta\s+property="og:image:height"[\s\S]*?>/)
  }

  return html
}

function renderRoute(shell: string, route: string, meta: PageMeta, extraHead = '', inlineJson = ''): string {
  const body = renderToString(
    <StaticRouter location={route}>
      <App />
    </StaticRouter>
  )
  let html = renderMeta(shell, meta)
  if (extraHead) html = html.replace('</head>', `  ${extraHead}\n</head>`)
  html = html.replace('<div id="root"></div>', `<div id="root">${body}</div>`)
  if (inlineJson) html = html.replace('</body>', `${inlineJson}\n</body>`)
  return html
}

function write(routeDir: string, html: string) {
  const dir = path.join(DIST, routeDir)
  fs.mkdirSync(dir, { recursive: true })
  fs.writeFileSync(path.join(dir, 'index.html'), html)
  console.log(`  prerendered /${routeDir}`)
}

// Alt text for the default site-wide OG card (og-image.png), used whenever a
// page has no image of its own.
const DEFAULT_IMAGE_ALT = `${SITE_NAME}: Runtime context for agents using your web app.`

// Builds the `<script id="__SIGHTMAP_POST__">` tag that inlines a post's
// rendered HTML as JSON so the client can seed hydration from it (see
// src/lib/postHtml.ts). The `<` escape is the only thing standing between a
// post body containing a literal `</script>` or `<!--` and that markup
// terminating the script tag / opening an HTML comment early, so this is
// exported and covered directly by scripts/prerender.test.ts rather than only
// being exercised indirectly through `main()`.
export function renderInlinePostJson(slug: string, html: string): string {
  return (
    `<script id="__SIGHTMAP_POST__" type="application/json">` +
    JSON.stringify({ slug, html }).replace(/</g, '\\u003c') +
    `</script>`
  )
}

async function main() {
  const shellPath = path.join(DIST, 'index.html')
  if (!fs.existsSync(shellPath)) {
    console.error('dist/index.html not found — run `vite build` first.')
    process.exit(1)
  }
  const shell = fs.readFileSync(shellPath, 'utf-8')

  // Homepage. Overwrites the shell in place.
  fs.writeFileSync(
    shellPath,
    renderRoute(shell, '/', {
      url: `${SITE_URL}/`,
      title: `${SITE_NAME} — runtime context for agents using your web app`,
      description: SITE_DESCRIPTION,
      image: `${SITE_URL}/og-image.png`,
      imageAlt: DEFAULT_IMAGE_ALT,
      imageDimensionsKnown: true,
      type: 'website',
    })
  )
  console.log('  prerendered /')

  // Blog index.
  write(
    'blog',
    renderRoute(shell, '/blog', {
      url: `${SITE_URL}/blog`,
      title: `Blog — ${SITE_NAME}`,
      description: 'Research and release notes from the people building the sightmap spec.',
      image: `${SITE_URL}/og-image.png`,
      imageAlt: DEFAULT_IMAGE_ALT,
      imageDimensionsKnown: true,
      type: 'website',
    })
  )

  // One page per published post. The post body is inlined as JSON so the
  // client can seed its first render from it and hydrate without a mismatch
  // (see src/lib/postHtml.ts).
  const posts = await loadPosts(CONTENT_DIR)
  for (const post of posts) {
    const fm = post.frontmatter
    const url = `${SITE_URL}/blog/${fm.slug}`
    const image = fm.image ? `${SITE_URL}${fm.image}` : `${SITE_URL}/og-image.png`
    // frontmatter `image` is only validated as "starts with /" (see
    // scripts/lib/posts.ts) — nothing guarantees it is the 1200x630 PNG a
    // future OG-card-generation step would produce, so we can't assert
    // og:image:type/width/height for it. The site-wide fallback is a known
    // asset we control, so that case stays known.
    const imageAlt = fm.image ? fm.title : DEFAULT_IMAGE_ALT
    const imageDimensionsKnown = !fm.image
    const html = await renderPostHtml(post.body)

    const jsonLd = {
      '@context': 'https://schema.org',
      '@type': 'Article',
      headline: fm.title,
      description: fm.excerpt,
      image,
      datePublished: fm.date,
      author: { '@type': 'Person', name: fm.author },
      publisher: {
        '@type': 'Organization',
        name: SITE_NAME,
        url: SITE_URL,
      },
      mainEntityOfPage: url,
    }

    const head = [
      `<meta property="article:published_time" content="${fm.date}">`,
      `<meta property="article:author" content="${esc(fm.author)}">`,
      `<meta property="article:section" content="${esc(fm.topic)}">`,
      `<script type="application/ld+json">${JSON.stringify(jsonLd).replace(/</g, '\\u003c')}</script>`,
    ].join('\n    ')

    const inline = renderInlinePostJson(fm.slug, html)

    write(
      `blog/${fm.slug}`,
      renderRoute(
        shell,
        `/blog/${fm.slug}`,
        {
          url,
          title: `${fm.title} — ${SITE_NAME}`,
          description: fm.excerpt,
          image,
          imageAlt,
          imageDimensionsKnown,
          type: 'article',
        },
        head,
        inline
      )
    )
  }

  console.log(`\n  prerender complete: ${posts.length + 2} page(s)`)
}

// Only run when invoked directly, so the test can import renderMeta and
// renderInlinePostJson. Compares resolved URLs rather than a filename suffix
// check (`.endsWith('prerender.tsx')`) so a different runner — a symlink, a
// wrapper script, being required under another name — can't silently skip
// the entire prerender with no error.
const entry = process.argv[1]
if (entry && import.meta.url === pathToFileURL(entry).href) {
  main().catch((err) => {
    console.error('Prerender failed:\n', err instanceof Error ? err.message : err)
    process.exit(1)
  })
}
