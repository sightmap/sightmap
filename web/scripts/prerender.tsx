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
import { setServerPostHtml, clearServerPostHtml } from '../src/lib/postHtml'
import { loadPosts, renderPostHtml } from './lib/posts'
import { loadAtlas } from './lib/atlas'
import {
  SITE_URL,
  SITE_NAME,
  SITE_DESCRIPTION,
  BLOG_DESCRIPTION,
  ATLAS_DESCRIPTION,
  HOME_TITLE,
  BLOG_INDEX_TITLE,
  ATLAS_INDEX_TITLE,
  NOT_FOUND_TITLE,
  NOT_FOUND_DESCRIPTION,
  postTitle,
  atlasTitle,
  esc,
} from './lib/site'

const DIST = path.resolve('dist')
const CONTENT_DIR = path.resolve('content/blog')
const ATLAS_DIR = path.resolve('src/data/atlas')

// Netlify sets DEPLOY_PRIME_URL on deploy previews and branch deploys (and
// URL, which already equals SITE_URL, on production). Locally and in
// production neither DEPLOY_PRIME_URL nor this fallback changes anything, so
// a preview build is the only case where this differs from SITE_URL.
//
// Deliberately computed here rather than exported from scripts/lib/site.ts:
// that module is imported by src/pages/BlogIndex.tsx and src/pages/BlogPost.tsx
// and gets pulled into the browser bundle (see its file header comment), where
// `process` does not exist. This file only runs under Node.
const DEPLOY_URL = process.env.DEPLOY_PRIME_URL || SITE_URL

export interface PageMeta {
  // Canonical, production-authoritative URL. Feeds <link rel="canonical">
  // (and, for posts, JSON-LD mainEntityOfPage) — always SITE_URL-based, even
  // on a deploy preview, since a preview should never claim to be canonical
  // for itself.
  url: string
  // og:url. DEPLOY_URL-based so a deploy preview's social card links back to
  // the preview it was actually generated from, not production.
  ogUrl: string
  title: string
  description: string
  // JSON-LD image — always SITE_URL-based, paired with the canonical `url`
  // above. Structured data describes the page's canonical identity, so it
  // should not shift to whatever host happens to be serving a preview.
  image: string
  // og:image AND twitter:image. DEPLOY_URL-based for the same reason as
  // ogUrl: a card image is an asset the unfurling client has to fetch, so it
  // must be advertised at a host that can actually serve it. On a preview
  // that is the preview host, which is the only way a preview-only card can
  // be seen at all.
  ogImage: string
  // Alt text for og:image / twitter:image. These describe the *image*, not
  // the page, so they get their own field rather than reusing title.
  imageAlt: string
  // True only when `image`/`ogImage` are guaranteed to be the site's 1200x630
  // PNG (currently: the og-image.png fallback used by the homepage and blog
  // index). Post images come from frontmatter `image`, which the zod schema
  // in scripts/lib/posts.ts only validates as "starts with /" — nothing
  // enforces its dimensions or format — so for those we cannot honestly claim
  // og:image:type/width/height and instead drop the three tags rather than
  // ship a false dimension/type claim paired with an image that may not
  // match it.
  imageDimensionsKnown: boolean
  type: 'website' | 'article'
  // Adds `<meta name="robots" content="noindex">` and drops the canonical
  // link. Only the 404 page sets this. The canonical has to go rather than
  // just point elsewhere: the shell's is hardcoded to the homepage, and
  // leaving it would tell a crawler that this page *is* the homepage — the
  // exact duplicate-content claim the 404 status exists to retract.
  noindex?: boolean
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
    .replace(/<meta\s+property="og:url"[\s\S]*?>/, `<meta property="og:url" content="${meta.ogUrl}">`)
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
      `<meta property="og:image" content="${meta.ogImage}">`
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
      `<meta name="twitter:image" content="${meta.ogImage}">`
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

  if (meta.noindex) {
    html = stripTag(html, /\n[ \t]*<link\s+rel="canonical"[\s\S]*?>/)
    // Asserted rather than left as a silent `.replace()` no-op like the meta
    // rewrites above. Those degrade to a stale-but-valid tag; this one
    // degrades to a 404 page a crawler is free to index, which is the failure
    // this whole change exists to prevent.
    if (!html.includes('</head>')) {
      throw new Error('renderMeta: no </head> in shell — cannot insert noindex')
    }
    html = html.replace('</head>', `  <meta name="robots" content="noindex">\n</head>`)
  }

  return html
}

function renderRoute(
  shell: string,
  route: string,
  meta: PageMeta,
  extraHead = '',
  inlineJson = '',
  stamp = true
): string {
  const body = renderToString(
    <StaticRouter location={route}>
      <App />
    </StaticRouter>
  )
  let html = renderMeta(shell, meta)
  if (extraHead) html = html.replace('</head>', `  ${extraHead}\n</head>`)
  // Stamp which route this markup was rendered for. Netlify's catch-all serves
  // the homepage dist/index.html for every route with no prerendered file, so
  // "#root has children" is not enough to know the markup belongs to the URL
  // in the address bar. src/main.tsx compares this stamp against
  // location.pathname and only hydrates on a match.
  //
  // `stamp` is false only for a draft post, rendered here at
  // `/blog/<slug>?preview=true` so BlogPost renders the article instead of its
  // `<Navigate>` redirect. That route string (with the query) is not what a
  // visitor's address bar shows when they open the bare `/blog/<slug>` — and
  // Netlify serves this same static file for both, since query strings don't
  // affect static-file resolution. Stamping it would make main.tsx hydrate
  // the bare URL against markup rendered for a route it doesn't match
  // (client-side BlogPost would redirect instead), reproducing the exact
  // hydration mismatch this stamp exists to prevent. Leaving drafts unstamped
  // means main.tsx always falls through to createRoot and renders fresh
  // client-side, so there's no mismatch either way. Crawlers/unfurlers still
  // get the full prerendered article regardless, since they never run JS to
  // notice the stamp is absent.
  html = stamp
    ? html.replace(
        '<div id="root"></div>',
        `<div id="root" data-prerender-route="${esc(route)}">${body}</div>`
      )
    : html.replace('<div id="root"></div>', `<div id="root">${body}</div>`)
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
      ogUrl: `${DEPLOY_URL}/`,
      title: HOME_TITLE,
      description: SITE_DESCRIPTION,
      image: `${SITE_URL}/og-image.png`,
      ogImage: `${DEPLOY_URL}/og-image.png`,
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
      ogUrl: `${DEPLOY_URL}/blog`,
      title: BLOG_INDEX_TITLE,
      description: BLOG_DESCRIPTION,
      image: `${SITE_URL}/og-image.png`,
      ogImage: `${DEPLOY_URL}/og-image.png`,
      imageAlt: DEFAULT_IMAGE_ALT,
      imageDimensionsKnown: true,
      type: 'website',
    })
  )

  // Netlify sets CONTEXT to 'production', 'deploy-preview', or
  // 'branch-deploy'; it's unset in a plain local build. Drafts get a
  // prerendered page everywhere except production, so a deploy preview can
  // publish a reviewable link for a draft post — sharing one on production
  // would leak an unpublished post, so that path must stay exactly as before.
  const isProductionBuild = !process.env.CONTEXT || process.env.CONTEXT === 'production'

  // One page per post (published always; drafts too on a non-production
  // build — see above). The post body is inlined as JSON so the client can
  // seed its first render from it and hydrate without a mismatch (see
  // src/lib/postHtml.ts).
  const posts = await loadPosts(CONTENT_DIR, { includeDrafts: !isProductionBuild })
  for (const post of posts) {
    const fm = post.frontmatter
    const url = `${SITE_URL}/blog/${fm.slug}`
    const ogUrl = `${DEPLOY_URL}/blog/${fm.slug}`
    const image = fm.image ? `${SITE_URL}${fm.image}` : `${SITE_URL}/og-image.png`
    const ogImage = fm.image ? `${DEPLOY_URL}${fm.image}` : `${DEPLOY_URL}/og-image.png`
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

    // src/lib/postHtml.ts reads the body through readPostHtml(), which on the
    // client finds the inlined JSON tag in the DOM. There is no DOM here, so
    // hand it the same string directly for the duration of this render —
    // otherwise the prerendered page contains nav, title, byline and footer
    // and no article at all.
    setServerPostHtml(fm.slug, html)
    try {
      // A draft is only reachable client-side at `?preview=true` (see
      // BlogPost's <Navigate> guard) — route the StaticRouter render through
      // that same query so the prerendered output is the article, not a
      // redirect. The file still publishes at the normal
      // dist/blog/<slug>/index.html path; Netlify's static lookup ignores
      // query strings. `stamp: false` leaves the page unstamped — see the
      // comment at the stamp site in renderRoute for why.
      write(
        `blog/${fm.slug}`,
        renderRoute(
          shell,
          fm.draft ? `/blog/${fm.slug}?preview=true` : `/blog/${fm.slug}`,
          {
            url,
            ogUrl,
            title: postTitle(fm.title),
            description: fm.excerpt,
            image,
            ogImage,
            imageAlt,
            imageDimensionsKnown,
            type: 'article',
          },
          head,
          inline,
          !fm.draft
        )
      )
    } finally {
      clearServerPostHtml()
    }
  }

  // ---- Atlas ----
  //
  // scripts/build-atlas.ts has already written src/generated/atlas-manifest.ts
  // (what the components read) and public/atlas/ (screenshots + machine
  // twins). This reads the vendored data a second time rather than importing
  // that manifest, for the same reason the blog reads content/blog here: the
  // manifest is a generated artifact, and a prerender that depends on it would
  // silently emit a stale page set if the two steps ever ran out of order.
  const atlas = await loadAtlas(ATLAS_DIR)

  write(
    'atlas',
    renderRoute(shell, '/atlas', {
      url: `${SITE_URL}/atlas`,
      ogUrl: `${DEPLOY_URL}/atlas`,
      title: ATLAS_INDEX_TITLE,
      description: ATLAS_DESCRIPTION,
      image: `${SITE_URL}/og-image.png`,
      ogImage: `${DEPLOY_URL}/og-image.png`,
      imageAlt: DEFAULT_IMAGE_ALT,
      imageDimensionsKnown: true,
      type: 'website',
    })
  )

  for (const entry of atlas.entries) {
    const url = `${SITE_URL}/atlas/${entry.slug}`
    const ogUrl = `${DEPLOY_URL}/atlas/${entry.slug}`

    // Every entry shares the site-wide card. Per-entry OG images are
    // deliberately not generated: og/render.mjs is a manual, Chrome-driven
    // step that writes a committed PNG, while atlas entries arrive by
    // automated bot PR — so a per-entry card would exist only for whichever
    // entries someone happened to hand-process, and be silently absent for the
    // rest. One card that is correct for every entry beats that.

    // A sightmap *is* a dataset about a website, and the atlas exists to be
    // consumed by machines — so the structured data describes it as one, with
    // the two machine twins as its distributions.
    const jsonLd = {
      '@context': 'https://schema.org',
      '@type': 'Dataset',
      name: `${entry.name} sightmap`,
      description: entry.description,
      url,
      creator: { '@type': 'Person', name: entry.author },
      dateCreated: entry.created,
      dateModified: entry.updated,
      isBasedOn: entry.site_url,
      keywords: entry.categories,
      distribution: [
        {
          '@type': 'DataDownload',
          encodingFormat: 'text/markdown',
          contentUrl: `${SITE_URL}/atlas/${entry.slug}.md`,
        },
        {
          '@type': 'DataDownload',
          encodingFormat: 'application/json',
          contentUrl: `${SITE_URL}/atlas/index.json`,
        },
      ],
      includedInDataCatalog: {
        '@type': 'DataCatalog',
        name: `${SITE_NAME} Atlas`,
        url: `${SITE_URL}/atlas`,
      },
    }

    const head = [
      // The machine twin, advertised where a crawler already looks for
      // alternate representations rather than only as a link in the sidebar.
      `<link rel="alternate" type="text/markdown" href="${SITE_URL}/atlas/${entry.slug}.md">`,
      `<script type="application/ld+json">${JSON.stringify(jsonLd).replace(/</g, '\\u003c')}</script>`,
    ].join('\n    ')

    write(
      `atlas/${entry.slug}`,
      renderRoute(
        shell,
        `/atlas/${entry.slug}`,
        {
          url,
          ogUrl,
          title: atlasTitle(entry.name),
          description: entry.description,
          image: `${SITE_URL}/og-image.png`,
          ogImage: `${DEPLOY_URL}/og-image.png`,
          imageAlt: DEFAULT_IMAGE_ALT,
          imageDimensionsKnown: true,
          type: 'website',
        },
        head
      )
    )
  }

  // ---- 404 ----
  //
  // Written to dist/404.html, not dist/404/index.html: netlify.toml's
  // catch-all names that exact path. Every real route above has a prerendered
  // file and Netlify resolves static files ahead of a non-forced redirect, so
  // the catch-all now only ever answers URLs that do not exist — which is what
  // lets it return a real 404 instead of the homepage shell at status 200.
  //
  // Unstamped on purpose. This one file answers every unknown URL, so any
  // fixed `data-prerender-route` value would mismatch the address bar; leaving
  // it off makes src/main.tsx fall through to createRoot and render NotFound
  // client-side for the actual URL, the same way drafts are handled above.
  fs.writeFileSync(
    path.join(DIST, '404.html'),
    renderRoute(
      shell,
      '/404',
      {
        url: `${SITE_URL}/404`,
        ogUrl: `${DEPLOY_URL}/404`,
        title: NOT_FOUND_TITLE,
        description: NOT_FOUND_DESCRIPTION,
        image: `${SITE_URL}/og-image.png`,
        ogImage: `${DEPLOY_URL}/og-image.png`,
        imageAlt: DEFAULT_IMAGE_ALT,
        imageDimensionsKnown: true,
        type: 'website',
        noindex: true,
      },
      '',
      '',
      false
    )
  )
  console.log('  prerendered /404.html')

  console.log(
    `\n  prerender complete: ${posts.length + atlas.entries.length + 4} page(s)`
  )
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
