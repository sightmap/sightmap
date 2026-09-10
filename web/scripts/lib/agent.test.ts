import { describe, expect, it } from 'vitest'
import {
  buildAtlasApiIndex,
  buildAtlasIndexMarkdown,
  buildDevelopersMarkdown,
  buildHomeMarkdown,
  buildNotFoundMarkdown,
  buildOpenApiSpec,
  buildSiteJsonLd,
  toYaml,
} from './agent'
import { DEVELOPERS_TITLE, SITE_NAME } from './site'

const ATLAS = [
  {
    slug: 'airbnb',
    name: 'Airbnb',
    description: 'Stay search.',
    updated: '2026-08-08',
    domains: ['airbnb.com'],
    last_verified: '2026-08-08',
    stats: { views: 5, components: 53, requests: 16 },
    site_url: 'https://www.airbnb.com/',
    categories: ['travel'],
  },
]

const LISTINGS = [
  {
    slug: 'alpha-tools',
    name: 'Alpha Tools',
    host: 'alpha.example.org',
    description: 'A fixture site with WebMCP tools.',
    type: 'live' as const,
    tool_count: 9,
    updated: '2026-09-08',
  },
]

describe('buildNotFoundMarkdown', () => {
  it('points agents at the sitemap, llms.txt, docs, and developer resources', () => {
    const md = buildNotFoundMarkdown()
    expect(md.startsWith('# Not found\n')).toBe(true)
    expect(md).toContain('https://sightmap.org/llms.txt')
    expect(md).toContain('https://sightmap.org/sitemap.xml')
    expect(md).toContain('https://sightmap.org/developers')
    expect(md).toContain('https://sightmap.org/openapi.json')
    expect(md).toContain('https://docs.sightmap.org')
  })
})

describe('buildHomeMarkdown', () => {
  it('names Sightmap and links developer resources', () => {
    const md = buildHomeMarkdown()
    expect(md.startsWith(`# ${SITE_NAME}\n`)).toBe(true)
    expect(md).toContain('/developers')
    expect(md).toContain('/openapi.json')
  })
})

describe('buildDevelopersMarkdown', () => {
  it('uses the Sightmap developer-resources title and lists the API', () => {
    const md = buildDevelopersMarkdown()
    expect(md).toContain(`# ${DEVELOPERS_TITLE}`)
    expect(md).toContain('/openapi.json')
    expect(md).toContain('/api/atlas')
    expect(md).toContain('no authentication')
  })

  it('names the directory files, the host lookup, and the submit endpoint', () => {
    const md = buildDevelopersMarkdown()
    expect(md).toContain('https://sightmap.org/atlas/directory.json')
    expect(md).toContain('https://sightmap.org/atlas/stats.json')
    expect(md).toContain('/atlas/sites/{slug}/tools.json')
    expect(md).toContain('GET /api/atlas/lookup/{host}')
    expect(md).toContain('POST /api/atlas/submit')
    // The two claims a listing must never be read as making.
    expect(md).toContain('never fetches the host')
    expect(md).toContain('never executes one')
  })
})

describe('buildAtlasIndexMarkdown', () => {
  it('covers both halves of /atlas and points at each machine index', () => {
    const md = buildAtlasIndexMarkdown(ATLAS, LISTINGS)
    expect(md).toContain('## WebMCP listings')
    expect(md).toContain('## Community maps')
    expect(md).toContain('https://sightmap.org/atlas/directory.json')
    expect(md).toContain('https://sightmap.org/atlas/stats.json')
    expect(md).toContain('https://sightmap.org/atlas/index.json')
    expect(md).toContain('POST https://sightmap.org/api/atlas/submit')
    expect(md).toContain(
      '- [Alpha Tools](https://sightmap.org/atlas/alpha-tools.md) (alpha.example.org): A fixture site with WebMCP tools. 9 WebMCP tools, live. JSON: https://sightmap.org/atlas/sites/alpha-tools.json'
    )
    expect(md).toContain('- [Airbnb](https://sightmap.org/atlas/airbnb.md)')
    expect(md).toContain('a listing says nothing about the site beyond that')
  })

  it('says so for each half rather than emitting an empty section', () => {
    const md = buildAtlasIndexMarkdown([], [])
    expect(md).toContain('- No entries published yet.')
    expect(md).toContain('- No listings published yet.')
    expect(md).not.toContain('undefined')
  })
})

describe('buildAtlasApiIndex', () => {
  it('adds machine-twin URLs on every entry', () => {
    const catalog = buildAtlasApiIndex(ATLAS)
    expect(catalog.schema_version).toBe(1)
    expect(catalog.entries[0]).toMatchObject({
      slug: 'airbnb',
      html: 'https://sightmap.org/atlas/airbnb',
      markdown: 'https://sightmap.org/atlas/airbnb.md',
      archive: 'https://sightmap.org/atlas/airbnb.tar.gz',
    })
  })
})

describe('buildOpenApiSpec', () => {
  it('is OpenAPI 3.1 and documents the public Sightmap API plus JSON errors', () => {
    const spec = buildOpenApiSpec()
    expect(spec.openapi).toBe('3.1.0')
    expect((spec.info as { title: string }).title).toContain('Sightmap')
    const paths = spec.paths as Record<string, unknown>
    expect(paths['/openapi.json']).toBeDefined()
    expect(paths['/api/atlas']).toBeDefined()
    expect(paths['/api/atlas/{slug}']).toBeDefined()
    const schemas = (spec.components as { schemas: Record<string, { required?: string[] }> })
      .schemas
    expect(schemas.Error.required).toEqual(['error'])
    const error = (
      schemas.Error as {
        properties: { error: { required: string[] } }
      }
    ).properties.error
    expect(error.required).toEqual(['code', 'message', 'hint', 'status'])
  })
})

describe('buildOpenApiSpec — WebMCP directory', () => {
  it('documents every generated directory document and both API endpoints', () => {
    const spec = buildOpenApiSpec()
    const paths = spec.paths as Record<string, Record<string, unknown>>
    for (const p of [
      '/atlas/directory.json',
      '/atlas/stats.json',
      '/atlas/sites/{slug}.json',
      '/atlas/sites/{slug}/tools.json',
      '/atlas/scans/{slug}.json',
      '/api/atlas/lookup/{host}',
      '/api/atlas/submit',
    ]) {
      expect(paths[p], p).toBeDefined()
    }
    expect(paths['/api/atlas/lookup/{host}'].get).toBeDefined()
    expect(paths['/api/atlas/submit'].post).toBeDefined()
  })

  it('says in the lookup description that it never fetches the host', () => {
    // The whole point of the endpoint: it answers from the stored index, so it
    // can never be used to make the site issue a request on a caller's behalf.
    const spec = buildOpenApiSpec()
    const paths = spec.paths as Record<string, { get: { description: string } }>
    const description = paths['/api/atlas/lookup/{host}'].get.description
    expect(description).toContain('NEVER fetches the host')
    expect(description).toContain('last_scanned')
  })

  it('accepts a submission with 202 and the documented error responses', () => {
    const spec = buildOpenApiSpec()
    const paths = spec.paths as Record<
      string,
      { post: { requestBody: unknown; responses: Record<string, unknown> } }
    >
    const submit = paths['/api/atlas/submit'].post
    expect(submit.requestBody).toMatchObject({
      content: { 'application/json': { schema: { $ref: '#/components/schemas/SubmitRequest' } } },
    })
    expect(Object.keys(submit.responses)).toEqual(['202', '400', '403', '422', '429'])
    const responses = (spec.components as { responses: Record<string, unknown> }).responses
    expect(responses.BadRequest).toBeDefined()
    expect(responses.TooManyRequests).toBeDefined()
    // A failed domain claim is 422, not 400: the request was well formed and
    // nothing was stored, so the same body can be posted again.
    expect(responses.ClaimFailed).toBeDefined()
    expect(responses.Quarantined).toBeDefined()
  })

  it('documents the optional domain claim and the card it earns', () => {
    const spec = buildOpenApiSpec()
    const schemas = (spec.components as {
      schemas: Record<string, { properties: Record<string, { pattern?: string; format?: string }> }>
    }).schemas
    expect(schemas.SubmitRequest.properties.claim?.pattern).toBe('^[0-9a-f]{32}$')
    expect(schemas.SubmitAccepted.properties.card?.format).toBe('uri')
  })

  it('adds the directory component schemas', () => {
    const spec = buildOpenApiSpec()
    const schemas = (spec.components as { schemas: Record<string, { required?: string[] }> })
      .schemas
    for (const name of [
      'DirectoryListing',
      'DirectoryTool',
      'ScanReportSummary',
      'Stats',
      'Lookup',
      'SubmitRequest',
      'SubmitAccepted',
    ]) {
      expect(schemas[name], name).toBeDefined()
    }
    expect(schemas.SubmitRequest.required).toEqual(['url', 'email'])
    expect(schemas.Lookup.required).toEqual(['slug', 'url', 'tool_count', 'last_scanned'])
  })

  it('round-trips the whole spec through the YAML dumper', () => {
    // The directory paths are the first ones with a request body and a
    // multi-segment path template, both of which the hand-rolled dumper has
    // to quote rather than emit bare.
    const yaml = toYaml(buildOpenApiSpec())
    expect(yaml).toContain('"/atlas/sites/{slug}/tools.json"')
    expect(yaml).toContain('"/api/atlas/submit"')
    expect(yaml).not.toContain('[object Object]')
  })
})

describe('buildSiteJsonLd', () => {
  it('advertises the Sightmap name, alternates, and sameAs profiles', () => {
    const json = buildSiteJsonLd() as {
      '@graph': Array<{ name?: string; alternateName?: string[]; sameAs?: string[] }>
    }
    const names = json['@graph'].map((n) => n.name)
    expect(names).toContain('Sightmap')
    const org = json['@graph'].find((n) => n.sameAs)
    expect(org?.alternateName).toContain('Sightmap spec')
    expect(org?.sameAs).toContain('https://github.com/sightmap/sightmap')
    expect(org?.sameAs).toContain('https://docs.sightmap.org')
  })
})

describe('toYaml', () => {
  it('round-trips the OpenAPI document into parseable YAML-shaped text', () => {
    const yaml = toYaml({ openapi: '3.1.0', info: { title: 'Sightmap HTTP API' }, tags: [] })
    expect(yaml).toContain('openapi: 3.1.0')
    expect(yaml).toContain('title: Sightmap HTTP API')
    expect(yaml).toContain('tags: []')
  })

  it('quotes strings that would break YAML', () => {
    expect(toYaml({ path: '/api/atlas/{slug}' })).toContain('"/api/atlas/{slug}"')
  })

  it('emits block-style array objects without extra indent on the first key', () => {
    const yaml = toYaml({ servers: [{ url: 'https://sightmap.org', description: 'prod' }] })
    expect(yaml).toContain('- url: "https://sightmap.org"')
    expect(yaml).toContain('  description: prod')
  })
})
