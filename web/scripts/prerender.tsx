// Post-build static render. Vite has already written dist/index.html (the
// shell with hashed asset links); this renders the React tree into #root for
// every route and writes one static file per URL.
//
// Netlify serves an existing static file before applying a non-forced redirect,
// so these files win over the SPA catch-all in netlify.toml without any extra
// rule. That is the same mechanism the Subtext site relies on.
import fs from 'node:fs'
import path from 'node:path'
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
  type: 'website' | 'article'
}

export function renderMeta(shell: string, meta: PageMeta): string {
  return shell
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
      /<meta\s+name="twitter:title"[\s\S]*?>/,
      `<meta name="twitter:title" content="${esc(meta.title)}">`
    )
    .replace(
      /<meta\s+name="twitter:image"[\s\S]*?>/,
      `<meta name="twitter:image" content="${meta.image}">`
    )
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

    const inline =
      `<script id="__SIGHTMAP_POST__" type="application/json">` +
      JSON.stringify({ slug: fm.slug, html }).replace(/</g, '\\u003c') +
      `</script>`

    write(
      `blog/${fm.slug}`,
      renderRoute(
        shell,
        `/blog/${fm.slug}`,
        { url, title: `${fm.title} — ${SITE_NAME}`, description: fm.excerpt, image, type: 'article' },
        head,
        inline
      )
    )
  }

  console.log(`\n  prerender complete: ${posts.length + 2} page(s)`)
}

// Only run when invoked directly, so the test can import renderMeta.
if (process.argv[1]?.endsWith('prerender.tsx')) {
  main().catch((err) => {
    console.error('Prerender failed:\n', err instanceof Error ? err.message : err)
    process.exit(1)
  })
}
