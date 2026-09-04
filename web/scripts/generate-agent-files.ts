// Emits the site's agent-facing machine files into dist/:
//
//   - openapi.json / api/openapi.yaml — OpenAPI 3.1 of the public HTTP API
//   - api/atlas.json and api/atlas/<slug>.json — JSON catalog + per-entry docs
//   - per-page markdown twins (index.md, blog.md, developers.md, 404.md, …)
//
// Runs after prerender so dist/ already exists. Vite copies public/ first;
// these files are generated, not authored, and must not linger after a
// takedown the way a committed public/ file would.
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { loadPosts } from './lib/posts'
import { loadAtlas } from './lib/atlas'
import { primaryDomain } from '../src/lib/atlas'
import type { FeedAtlasEntry, FeedPost } from './generate-feeds'
import {
  buildAtlasApiIndex,
  buildAtlasIndexMarkdown,
  buildBlogIndexMarkdown,
  buildBlogPostMarkdown,
  buildBuildingMarkdown,
  buildSightkickMarkdown,
  buildDevelopersMarkdown,
  buildHomeMarkdown,
  buildNotFoundMarkdown,
  buildOpenApiSpec,
  toAtlasApiEntry,
  toYaml,
} from './lib/agent'

const DIST = path.resolve('dist')
const CONTENT_DIR = path.resolve('content/blog')
const ATLAS_DIR = path.resolve('src/data/atlas')

function write(rel: string, body: string) {
  const full = path.join(DIST, rel)
  fs.mkdirSync(path.dirname(full), { recursive: true })
  fs.writeFileSync(full, body.endsWith('\n') ? body : `${body}\n`)
}

async function main() {
  if (!fs.existsSync(DIST)) {
    console.error('dist/ not found — run `vite build` first.')
    process.exit(1)
  }

  const loaded = await loadPosts(CONTENT_DIR)
  const posts: FeedPost[] = loaded.map((p) => ({
    slug: p.frontmatter.slug,
    title: p.frontmatter.title,
    excerpt: p.frontmatter.excerpt,
    date: p.frontmatter.date,
    author: p.frontmatter.author,
  }))

  const raw = await loadAtlas(ATLAS_DIR)
  const atlas: Array<FeedAtlasEntry & { site_url: string; categories: string[] }> =
    raw.entries.map((e) => ({
      slug: e.slug,
      name: e.name,
      description: e.description,
      updated: e.updated,
      domains: e.domains.length > 0 ? e.domains : [primaryDomain(e)],
      last_verified: e.last_verified,
      stats: e.stats,
      site_url: e.site_url,
      categories: e.categories,
    }))

  const spec = buildOpenApiSpec()
  write('openapi.json', JSON.stringify(spec, null, 2))
  write('api/openapi.yaml', `${toYaml(spec)}\n`)
  write('api/openapi.json', JSON.stringify(spec, null, 2))

  const catalog = buildAtlasApiIndex(atlas)
  write('api/atlas.json', JSON.stringify(catalog, null, 2))
  for (const entry of atlas) {
    write(`api/atlas/${entry.slug}.json`, JSON.stringify(toAtlasApiEntry(entry), null, 2))
  }

  write('404.md', buildNotFoundMarkdown())
  write('index.md', buildHomeMarkdown())
  write('blog.md', buildBlogIndexMarkdown(posts))
  write('atlas.md', buildAtlasIndexMarkdown(atlas))
  write('developers.md', buildDevelopersMarkdown())
  write('building.md', buildBuildingMarkdown())
  write('sightkick.md', buildSightkickMarkdown())

  for (const post of loaded) {
    write(
      `blog/${post.frontmatter.slug}.md`,
      buildBlogPostMarkdown({
        slug: post.frontmatter.slug,
        title: post.frontmatter.title,
        excerpt: post.frontmatter.excerpt,
        date: post.frontmatter.date,
        author: post.frontmatter.author,
        body: post.body,
      })
    )
  }

  console.log(
    `  wrote dist/openapi.json, dist/api/* and markdown twins (${posts.length} post(s), ${atlas.length} atlas entry(s))`
  )
}

const entry = process.argv[1]
if (entry && import.meta.url === pathToFileURL(entry).href) {
  main().catch((err) => {
    console.error('Agent-file generation failed:\n', err instanceof Error ? err.message : err)
    process.exit(1)
  })
}
