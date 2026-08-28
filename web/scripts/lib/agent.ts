// Machine-readable files aimed at agents: OpenAPI, per-page markdown
// twins, JSON API documents, and the 404 recovery note. generate-agent-files.ts
// writes them into dist/; the negotiate edge function serves the twins
// when Accept prefers text/markdown.
import {
  DEVELOPERS_DESCRIPTION,
  DEVELOPERS_TITLE,
  WEBMCP_TITLE,
  WEBMCP_DESCRIPTION,
  SITE_DESCRIPTION,
  SITE_NAME,
  SITE_URL,
} from './site'
import type { FeedAtlasEntry, FeedPost } from '../generate-feeds'

export const DEVELOPERS_PATH = '/developers'

export interface AtlasApiEntry {
  slug: string
  name: string
  site_url: string
  domains: string[]
  description: string
  categories: string[]
  updated: string
  last_verified: string
  stats: FeedAtlasEntry['stats']
  html: string
  markdown: string
  archive: string
}

export function buildNotFoundMarkdown(): string {
  return `# Not found

This path is not on ${SITE_NAME}.

## Where to look next

- [Sightmap llms.txt](${SITE_URL}/llms.txt) — published site index for agents
- [Sitemap](${SITE_URL}/sitemap.xml)
- [Sightmap developer resources](${SITE_URL}${DEVELOPERS_PATH})
- [OpenAPI specification](${SITE_URL}/openapi.json)
- [Atlas HTTP API](${SITE_URL}/api/atlas)
- [Documentation](https://docs.sightmap.org)
- [GitHub](https://github.com/sightmap/sightmap)
`
}

export function buildHomeMarkdown(): string {
  return `# ${SITE_NAME}

> ${SITE_DESCRIPTION}

This is the ${SITE_NAME} homepage at ${SITE_URL}. ${SITE_NAME} is an open YAML spec and CLI that maps views, components, and API requests to source files, with memory for runtime behavior.

## Start here

- [Sightmap developer resources](${SITE_URL}${DEVELOPERS_PATH})
- [OpenAPI specification](${SITE_URL}/openapi.json)
- [llms.txt](${SITE_URL}/llms.txt)
- [Documentation](https://docs.sightmap.org)
- [Atlas](${SITE_URL}/atlas)
- [Blog](${SITE_URL}/blog)
- [GitHub](https://github.com/sightmap/sightmap)
- [CLI on npm](https://www.npmjs.com/package/@sightmap/sightmap)

## Get started

    npm install -g @sightmap/sightmap
    sightmap skills install
`
}

export function buildBlogIndexMarkdown(posts: FeedPost[]): string {
  const items =
    posts.length === 0
      ? '- No posts published yet.'
      : posts
          .map((p) => `- [${p.title}](${SITE_URL}/blog/${p.slug}): ${p.excerpt}`)
          .join('\n')
  return `# Blog — ${SITE_NAME}

Research and release notes from the people building the sightmap spec.

${items}
`
}

export function buildBlogPostMarkdown(post: FeedPost & { body: string }): string {
  return `# ${post.title}

${post.excerpt}

${post.body.trim()}
`
}

export function buildAtlasIndexMarkdown(atlas: FeedAtlasEntry[]): string {
  const items =
    atlas.length === 0
      ? '- No entries published yet.'
      : atlas
          .map(
            (e) =>
              `- [${e.name}](${SITE_URL}/atlas/${e.slug}.md) (${e.domains.join(', ')}): ${e.description}`
          )
          .join('\n')
  return `# Atlas — ${SITE_NAME}

Community-contributed maps of views, components, and requests.

Machine index: ${SITE_URL}/atlas/index.json
HTTP API: ${SITE_URL}/api/atlas

${items}
`
}

export function buildWebmcpMarkdown(): string {
  return `# ${WEBMCP_TITLE}

${WEBMCP_DESCRIPTION}

## The problem

WebMCP lets a page hand an agent callable tools instead of a click-path, but it
assumes the site's own authors write them, and almost none have. A computer-use
agent is left driving pixels: a screenshot before every decision and another
after every action.

## What a sightmap adds

A corpus already holds what a tool needs — verified selectors, per-instance
properties, view routes, the API requests the app actually makes, and the
hazard notes that explain what goes wrong. \`sightmap webmcp\` compiles that
plus a short manifest into a bundle, resolving every reference at generate time
and failing on anything it cannot resolve.

## Shipping formats

- Snippet — injected by an agent harness or an extension.
- ES module — for the site's own owners: \`<script type="module">\`.
- Userscript — for everyone else, via Tampermonkey or Violentmonkey.

Bundles register with \`document.modelContext\` where the browser supports
WebMCP and always install a \`window.__sightmapWebMCP\` shim, so a browser
without it renders and behaves exactly as before.

## Where it pays

- **Cobrowsing** — the agent is already in the signed-in session, on the page.
- **VM agents** — a browser on a virtual machine pays the full screenshot loop.
- **Agentic QA** — one call puts an account in the state you meant to test.

## Links

- [CLI reference](https://docs.sightmap.org/cli/webmcp)
- [Generator source](https://github.com/sightmap/sightmap/tree/main/webmcp)
- [Quickstart](https://docs.sightmap.org/start/quickstart)
- [llms.txt](${SITE_URL}/llms.txt)
`
}

export function buildDevelopersMarkdown(): string {
  return `# ${DEVELOPERS_TITLE}

${DEVELOPERS_DESCRIPTION}

${SITE_NAME} is an open specification and CLI. The public HTTP API on this site is read-only and requires no authentication.

## HTTP API

- [OpenAPI specification (JSON)](${SITE_URL}/openapi.json)
- [OpenAPI specification (YAML)](${SITE_URL}/api/openapi.yaml)
- [Atlas catalog](${SITE_URL}/api/atlas) — \`GET /api/atlas\`
- [One Atlas entry](${SITE_URL}/api/atlas/{slug}) — \`GET /api/atlas/{slug}\`

Errors are JSON objects with \`error.code\`, \`error.message\`, and \`error.hint\`.

## Documentation

- [Sightmap documentation](https://docs.sightmap.org)
- [Quickstart](https://docs.sightmap.org/start/quickstart)
- [Schema reference](https://docs.sightmap.org/reference/schema)
- [Specification (normative)](https://github.com/sightmap/sightmap/tree/main/spec)

## CLI and skills

- [CLI on npm](https://www.npmjs.com/package/@sightmap/sightmap) — \`npm install -g @sightmap/sightmap\`
- [GitHub repository](https://github.com/sightmap/sightmap)
- Agent skills: \`sightmap skills install\` installs \`sightmap-authoring\` and \`sightmap-browser\`

## Site index

- [llms.txt](${SITE_URL}/llms.txt)
- [Sitemap](${SITE_URL}/sitemap.xml)
- [RSS](${SITE_URL}/rss.xml)
`
}

export function toAtlasApiEntry(
  entry: FeedAtlasEntry & { site_url?: string; categories?: string[] }
): AtlasApiEntry {
  return {
    slug: entry.slug,
    name: entry.name,
    site_url: entry.site_url ?? `${SITE_URL}/atlas/${entry.slug}`,
    domains: entry.domains,
    description: entry.description,
    categories: entry.categories ?? [],
    updated: entry.updated,
    last_verified: entry.last_verified,
    stats: entry.stats,
    html: `${SITE_URL}/atlas/${entry.slug}`,
    markdown: `${SITE_URL}/atlas/${entry.slug}.md`,
    archive: `${SITE_URL}/atlas/${entry.slug}.tar.gz`,
  }
}

export function buildAtlasApiIndex(
  entries: Array<FeedAtlasEntry & { site_url?: string; categories?: string[] }>
): { schema_version: number; entries: AtlasApiEntry[] } {
  return {
    schema_version: 1,
    entries: entries.map(toAtlasApiEntry),
  }
}

export function buildSiteJsonLd(): object {
  return {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'Organization',
        '@id': `${SITE_URL}/#organization`,
        name: SITE_NAME,
        alternateName: ['Sightmap spec', 'Sightmap.org'],
        url: SITE_URL,
        description: SITE_DESCRIPTION,
        sameAs: [
          'https://github.com/sightmap/sightmap',
          'https://www.npmjs.com/package/@sightmap/sightmap',
          'https://docs.sightmap.org',
        ],
      },
      {
        '@type': 'WebSite',
        '@id': `${SITE_URL}/#website`,
        name: SITE_NAME,
        alternateName: ['Sightmap spec', 'Sightmap.org'],
        url: SITE_URL,
        description: SITE_DESCRIPTION,
        publisher: { '@id': `${SITE_URL}/#organization` },
      },
      {
        '@type': 'SoftwareApplication',
        '@id': `${SITE_URL}/#software`,
        name: SITE_NAME,
        alternateName: 'Sightmap spec',
        applicationCategory: 'DeveloperApplication',
        operatingSystem: 'Linux, macOS, Windows',
        url: SITE_URL,
        downloadUrl: 'https://www.npmjs.com/package/@sightmap/sightmap',
        description: SITE_DESCRIPTION,
        offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
        publisher: { '@id': `${SITE_URL}/#organization` },
      },
    ],
  }
}

export function buildOpenApiSpec(): Record<string, unknown> {
  const errorSchema = {
    type: 'object',
    additionalProperties: false,
    required: ['error'],
    properties: {
      error: {
        type: 'object',
        additionalProperties: false,
        required: ['code', 'message', 'hint', 'status'],
        properties: {
          code: { type: 'string', examples: ['not_found'] },
          message: { type: 'string' },
          hint: {
            type: 'string',
            description: 'Where an agent should look next to recover.',
          },
          status: { type: 'integer' },
        },
      },
    },
  }

  const atlasEntrySchema = {
    type: 'object',
    required: ['slug', 'name', 'domains', 'html', 'markdown', 'archive'],
    properties: {
      slug: { type: 'string' },
      name: { type: 'string' },
      site_url: { type: 'string', format: 'uri' },
      domains: { type: 'array', items: { type: 'string' } },
      description: { type: 'string' },
      categories: { type: 'array', items: { type: 'string' } },
      updated: { type: 'string' },
      last_verified: { type: 'string' },
      stats: {
        type: 'object',
        properties: {
          views: { type: 'integer' },
          components: { type: 'integer' },
          requests: { type: 'integer' },
        },
      },
      html: { type: 'string', format: 'uri' },
      markdown: { type: 'string', format: 'uri' },
      archive: { type: 'string', format: 'uri' },
    },
  }

  const errorResponse = {
    description: 'Structured JSON error',
    content: {
      'application/json': {
        schema: { $ref: '#/components/schemas/Error' },
      },
    },
  }

  return {
    openapi: '3.1.0',
    info: {
      title: `${SITE_NAME} HTTP API`,
      summary: `Public read-only HTTP API for the ${SITE_NAME} site and Atlas catalog`,
      description: `Machine-readable surface of ${SITE_URL}. No authentication. Errors are JSON objects with error.code, error.message, and error.hint.`,
      version: '1.0.0',
      license: {
        name: 'MIT',
        identifier: 'MIT',
        url: 'https://github.com/sightmap/sightmap/blob/main/LICENSE',
      },
      contact: {
        name: SITE_NAME,
        url: 'https://github.com/sightmap/sightmap',
      },
    },
    servers: [{ url: SITE_URL, description: `${SITE_NAME} production` }],
    tags: [
      { name: 'Atlas', description: 'Community-contributed sightmaps of live sites' },
      { name: 'Discovery', description: 'Site index and specification documents' },
    ],
    paths: {
      '/openapi.json': {
        get: {
          tags: ['Discovery'],
          summary: 'OpenAPI specification',
          operationId: 'getOpenApi',
          responses: {
            '200': {
              description: 'OpenAPI 3.1 document',
              content: {
                'application/json': { schema: { type: 'object' } },
              },
            },
          },
        },
      },
      '/api/openapi.yaml': {
        get: {
          tags: ['Discovery'],
          summary: 'OpenAPI specification (YAML)',
          operationId: 'getOpenApiYaml',
          responses: {
            '200': {
              description: 'OpenAPI 3.1 document in YAML',
              content: { 'application/yaml': { schema: { type: 'string' } } },
            },
          },
        },
      },
      '/api/atlas': {
        get: {
          tags: ['Atlas'],
          summary: 'List published Atlas entries',
          operationId: 'listAtlas',
          responses: {
            '200': {
              description: 'Atlas catalog',
              content: {
                'application/json': {
                  schema: { $ref: '#/components/schemas/AtlasCatalog' },
                },
              },
            },
            '404': { $ref: '#/components/responses/NotFound' },
          },
        },
      },
      '/api/atlas/{slug}': {
        get: {
          tags: ['Atlas'],
          summary: 'Get one Atlas entry',
          operationId: 'getAtlasEntry',
          parameters: [
            {
              name: 'slug',
              in: 'path',
              required: true,
              schema: { type: 'string' },
              description: 'Atlas entry slug, e.g. airbnb',
            },
          ],
          responses: {
            '200': {
              description: 'One Atlas entry plus machine-twin URLs',
              content: {
                'application/json': {
                  schema: { $ref: '#/components/schemas/AtlasEntry' },
                },
              },
            },
            '404': { $ref: '#/components/responses/NotFound' },
          },
        },
      },
      '/llms.txt': {
        get: {
          tags: ['Discovery'],
          summary: 'Agent site index (llmstxt.org)',
          operationId: 'getLlmsTxt',
          responses: {
            '200': {
              description: 'Plain-text site index',
              content: { 'text/plain': { schema: { type: 'string' } } },
            },
          },
        },
      },
      '/atlas/index.json': {
        get: {
          tags: ['Atlas'],
          summary: 'Vendored Atlas index (full gallery record)',
          operationId: 'getAtlasIndex',
          responses: {
            '200': {
              description: 'Full Atlas index.json',
              content: { 'application/json': { schema: { type: 'object' } } },
            },
          },
        },
      },
    },
    components: {
      schemas: {
        Error: errorSchema,
        AtlasEntry: atlasEntrySchema,
        AtlasCatalog: {
          type: 'object',
          required: ['schema_version', 'entries'],
          properties: {
            schema_version: { type: 'integer' },
            entries: {
              type: 'array',
              items: { $ref: '#/components/schemas/AtlasEntry' },
            },
          },
        },
      },
      responses: {
        NotFound: errorResponse,
      },
    },
  }
}

/** Minimal YAML 1.2 dump for the OpenAPI document (JSON-serializable values). */
export function toYaml(value: unknown, indent = 0): string {
  const pad = '  '.repeat(indent)
  if (value === null) return 'null'
  if (typeof value === 'boolean' || typeof value === 'number') return String(value)
  if (typeof value === 'string') {
    if (value === '' || /[:#{}[\],&*?|<>=!%@`'"\n]/.test(value) || value !== value.trim()) {
      return JSON.stringify(value)
    }
    return value
  }
  if (Array.isArray(value)) {
    if (value.length === 0) return '[]'
    return value
      .map((item) => {
        if (item !== null && typeof item === 'object' && !Array.isArray(item)) {
          const keys = Object.keys(item as Record<string, unknown>)
          if (keys.length === 0) return `${pad}- {}`
          const innerLines = toYaml(item, indent + 1).split('\n')
          const first = (innerLines[0] ?? '').replace(/^\s+/, '')
          const rest = innerLines.slice(1).join('\n')
          return rest ? `${pad}- ${first}\n${rest}` : `${pad}- ${first}`
        }
        if (Array.isArray(item)) {
          const inner = toYaml(item, indent + 1)
          return `${pad}-\n${inner}`
        }
        return `${pad}- ${toYaml(item, 0)}`
      })
      .join('\n')
  }
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
    if (entries.length === 0) return '{}'
    return entries
      .map(([k, v]) => {
        const key = /^[A-Za-z_][A-Za-z0-9_/-]*$/.test(k) ? k : JSON.stringify(k)
        if (v !== null && typeof v === 'object') {
          const inner = toYaml(v, indent + 1)
          if (inner === '{}' || inner === '[]') return `${pad}${key}: ${inner}`
          return `${pad}${key}:\n${inner}`
        }
        return `${pad}${key}: ${toYaml(v, 0)}`
      })
      .join('\n')
  }
  return JSON.stringify(value)
}
