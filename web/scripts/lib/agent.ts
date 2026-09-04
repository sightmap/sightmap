// Machine-readable files aimed at agents: OpenAPI, per-page markdown
// twins, JSON API documents, and the 404 recovery note. generate-agent-files.ts
// writes them into dist/; the negotiate edge function serves the twins
// when Accept prefers text/markdown.
import {
  BUILDING_DESCRIPTION,
  BUILDING_TITLE,
  DEVELOPERS_DESCRIPTION,
  DEVELOPERS_TITLE,
  SIGHTKICK_DESCRIPTION,
  SIGHTKICK_TITLE,
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
- [The Building — how Sightmap works](${SITE_URL}/building)
- [Sightkick — WebMCP tools for your web app](${SITE_URL}/sightkick)
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

export function buildBuildingMarkdown(): string {
  return `# ${BUILDING_TITLE}

${BUILDING_DESCRIPTION}

The interactive version at ${SITE_URL}/building is a scroll-driven 3D scene. This is the same story in text.

## The metaphor

- **Code is the blueprint.** Source files describe every wall: components, routes, API handlers. They are complete and exact, and almost useless to someone standing in the lobby.
- **The running app is the building.** Each view is a floor with its own route. Components are the rooms on that floor. API requests are the service risers running up the core.
- **A sightmap is the wayfinding.** A \`.sightmap/\` directory names every floor, room, and riser, links each back to its source file, and keeps memory notes for the quirks the drawings never recorded.
- **Users and agents are the people.** Every session is a journey through rooms and floors. Subtext records those journeys against the map, so a replay reads as named components rather than a list of divs.

## Built on top (exploratory)

- **Self-healing tests.** A test written against the map asks the building where a room went when a selector changes; the name stays stable and the run finishes.
- **Trajectories.** Codified journeys: the views a flow visits, the components it touches, and the requests it expects on the way.
- **Web MCP tools.** Tools generated from the map, each backed by a real view and a real request, so an agent walks up to the front desk instead of wandering the halls.

## Next

- [Homepage](${SITE_URL}/)
- [Quickstart](https://docs.sightmap.org/start/quickstart)
- [Documentation](https://docs.sightmap.org)
- [GitHub](https://github.com/sightmap/sightmap)
`
}

export function buildSightkickMarkdown(): string {
  return `# ${SIGHTKICK_TITLE}

${SIGHTKICK_DESCRIPTION}

The page at ${SITE_URL}/sightkick is the same content with the worked example rendered.

## What it is

Sightmap maps a running web app into a \`.sightmap/\` corpus: named views, named
components, and the properties worth reading off them. Sightkick is its
companion CLI. It compiles that corpus plus a \`.sightkick/\` tool layer into
WebMCP tool IR, so an agent calls \`search_flights(origin, destination, date)\`
instead of guessing which element on the page is the search box.

WebMCP is an early W3C proposal from Google and Microsoft for how a page hands
the agent in the same browser tab a list of callable actions. Sightkick
compiles that surface from the outside, for apps that do not declare one.

## How the two fit together

1. \`.sightmap/\` — the corpus. Views, components, extracted properties. The only
   place a CSS selector appears.
2. \`.sightkick/\` — the tool layer. Any number of YAML files, merged. Tools name
   corpus components; they never carry selectors.
3. \`sightkick build\` — compiles both into one self-contained IR, resolving every
   reference against the corpus. \`--verify\` checks each returns extractor
   against a captured snapshot.
4. The runtime — a ~19 KB bundle that registers the IR on
   \`document.modelContext\`, the browser's native WebMCP surface on Chrome for
   Testing.

## Commands

- \`sightkick build <dir>\` — compile \`.sightkick/\` + \`.sightmap/\` into tool IR.
- \`sightkick browser <dir>\` — build, start a sightmap session, and persist-inject
  the runtime so tools re-register on every new document.
- \`sightkick call <dir> <tool> --param k=v\` — invoke one tool and print its
  ToolResult as JSON. \`--via cli\` drives real browser input from any page;
  \`--via webmcp\` asks the page's own registered tool to run itself.
- \`sightkick runtime\` — emit the runtime bundle.
- \`sightkick skills install\` — install the sightkick and sightmap agent skills.

## Install

\`\`\`sh
npm install -g @sightmap/sightmap @sightmap/sightkick
sightkick skills install
\`\`\`

## Next

- [Homepage](${SITE_URL}/)
- [The Building](${SITE_URL}/building)
- [Documentation](https://docs.sightmap.org/sightkick)
- [GitHub](https://github.com/sightmap/sightkick)
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
