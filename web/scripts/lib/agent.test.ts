import { describe, expect, it } from 'vitest'
import {
  buildAtlasApiIndex,
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
