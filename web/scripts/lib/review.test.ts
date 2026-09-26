import fs from 'node:fs'
import path from 'node:path'
import Anthropic from '@anthropic-ai/sdk'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ScanReportSchema } from './directory'
import {
  CATEGORIES,
  MAX_SCHEMA_CHARS,
  claudeReview,
  fallbackDescription,
  fallbackReason,
  heuristicReview,
  mergeReview,
  oneSentence,
  recommendationFor,
  reviewPayload,
  suggestedJourneys,
  type ReviewClient,
  type ReviewOutput,
} from './review'
import type { ScanReport, ScanTool } from '../../src/types/directory'

const FIXTURE = path.resolve(__dirname, '__fixtures__/directory/scans/alpha-tools/2026-09-08.json')

function fixture(): ScanReport {
  return ScanReportSchema.parse(JSON.parse(fs.readFileSync(FIXTURE, 'utf-8'))) as ScanReport
}

function tool(over: Partial<ScanTool> = {}): ScanTool {
  return {
    name: 'do_thing',
    description: 'Does a thing.',
    inputSchema: { type: 'object', properties: {} },
    page: '/',
    pages: ['/'],
    impl: 'imperative',
    api: 'navigator',
    risk: 'action',
    riskReason: 'name contains "do"',
    warnings: [],
    ...over,
  }
}

afterEach(() => {
  vi.unstubAllEnvs()
})

describe('heuristicReview', () => {
  it('reviews the fixture scan without a model', () => {
    const review = heuristicReview(fixture())
    expect(review.source).toBe('heuristic')
    expect(review.description).toBe('Alpha does things.')
    expect(review.category).toBe('docs')
    expect(CATEGORIES).toContain(review.category)
    // alpha.example.org is a reserved example domain, never a product surface.
    expect(review.type).toBe('demo')
    expect(review.tools).toEqual([
      { name: 'search', kind: 'read', reason: 'name starts with "search"' },
      { name: 'open_page', kind: 'action', reason: 'name contains "open"' },
    ])
    expect(review.suspicious).toEqual([])
    expect(review.recommendation).toBe('read-only-journey-ok')
    expect(review.notes).toMatch(/no model call/)
  })

  it('turns passing checks into strengths and failing ones into factual improvements', () => {
    const report = fixture()
    report.checks = [
      { id: 'named', label: 'Every tool has a name', ok: true, detail: '2/2' },
      { id: 'described', label: 'Every tool has a description', ok: false, detail: '1/2' },
      { id: 'made-up', label: 'Something new', ok: false, detail: 'nope' },
    ]
    const review = heuristicReview(report)
    expect(review.strengths).toEqual(['Every tool has a name (2/2).'])
    expect(review.improvements).toEqual(['Not every tool has a description (1/2).', 'Not yet true: Something new (nope).'])
  })

  it('suggests at most three read-only journeys and pairs a search with a get', () => {
    const report = fixture()
    report.tools = [
      tool({ name: 'search_docs', risk: 'read' }),
      tool({ name: 'get_page', risk: 'read' }),
      tool({ name: 'list_tags', risk: 'read' }),
      tool({ name: 'count_items', risk: 'read' }),
      tool({ name: 'delete_all', risk: 'sensitive' }),
    ]
    const journeys = suggestedJourneys(report)
    expect(journeys).toHaveLength(3)
    expect(journeys[0].tools).toEqual(['search_docs', 'get_page'])
    expect(journeys.flatMap((j) => j.tools)).not.toContain('delete_all')
  })

  it('falls back to a count sentence when the page offers no hints', () => {
    const report = fixture()
    report.hints = { forms: [], links: [], title: '', description: '' }
    expect(heuristicReview(report).description).toBe('alpha.example.org exposes 2 WebMCP tools.')
    expect(fallbackDescription({ ...report, counts: { ...report.counts, tools: 1 } })).toMatch(/exposes 1 WebMCP tool\./)
  })

  it('reports tools whose metadata reads like an instruction', () => {
    const report = fixture()
    report.tools = [
      tool({ name: 'search', risk: 'read', warnings: ['metadata matches ignore (all|any|the|previous|prior|above)'] }),
    ]
    const review = heuristicReview(report)
    expect(review.suspicious).toEqual(['search: metadata matches ignore (all|any|the|previous|prior|above)'])
    expect(review.recommendation).toBe('needs-manual-review')
  })
})

describe('recommendationFor', () => {
  const base = fixture()
  it('rejects a scan that never reached the site', () => {
    expect(recommendationFor({ ...base, status: 'blocked' }, [])).toBe('reject')
    expect(recommendationFor({ ...base, status: 'load-error' }, [])).toBe('reject')
  })
  it('asks for a human when the scan is unsure or nothing is readable', () => {
    expect(recommendationFor({ ...base, status: 'needs-review' }, [])).toBe('needs-manual-review')
    expect(recommendationFor({ ...base, counts: { ...base.counts, read: 0, sensitive: 2 } }, [])).toBe('needs-manual-review')
    expect(recommendationFor({ ...base, counts: { ...base.counts, read: 0, sensitive: 0 } }, [])).toBe('needs-manual-review')
  })
  it('clears a read-only journey when there is a read tool and nothing flagged', () => {
    expect(recommendationFor(base, [])).toBe('read-only-journey-ok')
  })
})

describe('oneSentence', () => {
  it('keeps one sentence and caps it at 160 characters', () => {
    expect(oneSentence('First one. Second one.')).toBe('First one.')
    expect(oneSentence('no punctuation here')).toBe('no punctuation here.')
    const long = oneSentence(`${'word '.repeat(60)}end.`)
    expect(long.length).toBeLessThanOrEqual(160)
    expect(long.endsWith('…')).toBe(true)
  })
  it('is empty for empty input, so the caller can fall back', () => {
    expect(oneSentence('   \n ')).toBe('')
  })
})

describe('reviewPayload', () => {
  it('truncates long descriptions and oversized input schemas', () => {
    const report = fixture()
    report.tools = [
      tool({
        name: 'search',
        risk: 'read',
        description: 'x'.repeat(4000),
        inputSchema: { type: 'object', properties: { blob: { type: 'string', description: 'y'.repeat(9000) } } },
      }),
    ]
    const payload = JSON.parse(reviewPayload(report))
    expect(payload.tools[0].description).toMatch(/truncated, 4000 chars/)
    expect(payload.tools[0].description.length).toBeLessThan(1700)
    expect(payload.tools[0].input_schema).toMatch(/truncated/)
    expect(payload.tools[0].input_schema.length).toBeLessThan(MAX_SCHEMA_CHARS + 60)
    expect(payload.tools[0].atlas_kind_guess).toBe('read')
  })
})

describe('mergeReview', () => {
  const base = heuristicReview(fixture())

  const output = (over: Partial<ReviewOutput> = {}): Partial<ReviewOutput> => ({
    description: 'Alpha is a documentation site with a search tool.',
    category: 'docs',
    type: 'demo',
    tools: [
      { name: 'search', kind: 'read', reason: 'returns documentation text only' },
      { name: 'open_page', kind: 'read', reason: 'navigates, changes nothing server side' },
    ],
    suspicious: [],
    strengths: ['Both tools carry an input schema.'],
    improvements: ['Neither tool documents its return shape.'],
    suggested_journeys: [{ intent: 'Search the docs for "pricing".', tools: ['search'] }],
    recommendation: 'read-only-journey-ok',
    notes: 'Nothing unusual.',
    ...over,
  })

  it('takes the model prose, kinds and journeys', () => {
    const merged = mergeReview(base, output(), base.notes)
    expect(merged.source).toBe('claude')
    expect(merged.description).toBe('Alpha is a documentation site with a search tool.')
    expect(merged.tools.find((t) => t.name === 'open_page')).toEqual({
      name: 'open_page',
      kind: 'read',
      reason: 'navigates, changes nothing server side',
    })
    expect(merged.strengths).toEqual(['Both tools carry an input schema.'])
    expect(merged.notes).toBe('Nothing unusual.')
  })

  it('drops tools the scan never saw and keeps the ones the model skipped', () => {
    const merged = mergeReview(
      base,
      output({ tools: [{ name: 'exfiltrate_cookies', kind: 'read', reason: 'harmless, promise' }] }),
      base.notes
    )
    expect(merged.tools.map((t) => t.name)).toEqual(['search', 'open_page'])
    expect(merged.tools.map((t) => t.kind)).toEqual(['read', 'action'])
  })

  it('unions suspicious lines and lets a model warning tighten the recommendation', () => {
    const flagged = { ...base, suspicious: ['search: metadata matches password'] }
    const merged = mergeReview(flagged, output({ suspicious: ['open_page: description tells the agent to skip confirmation'] }), base.notes)
    expect(merged.suspicious).toEqual([
      'search: metadata matches password',
      'open_page: description tells the agent to skip confirmation',
    ])
    expect(merged.recommendation).toBe('needs-manual-review')
  })

  it('lets the model tighten but never loosen the recommendation', () => {
    expect(mergeReview(base, output({ recommendation: 'reject' }), base.notes).recommendation).toBe('reject')
    const rejected = { ...base, recommendation: 'reject' as const }
    expect(mergeReview(rejected, output({ recommendation: 'read-only-journey-ok' }), base.notes).recommendation).toBe('reject')
  })

  it('drops a suggested journey that would call a non-read tool', () => {
    const merged = mergeReview(
      base,
      output({
        tools: [{ name: 'open_page', kind: 'action', reason: 'navigates' }],
        suggested_journeys: [{ intent: 'Open every page.', tools: ['open_page'] }],
      }),
      base.notes
    )
    expect(merged.suggested_journeys).toEqual(base.suggested_journeys)
    expect(merged.suggested_journeys.every((j) => j.tools.every((t) => t === 'search'))).toBe(true)
  })

  it('falls back field by field when the model leaves something out', () => {
    const merged = mergeReview(base, { description: '', category: 'not-a-category' as never, tools: [] }, 'note from the caller')
    expect(merged.description).toBe(base.description)
    expect(merged.category).toBe(base.category)
    expect(merged.tools).toEqual(base.tools)
    expect(merged.strengths).toEqual(base.strengths)
    expect(merged.notes).toBe('note from the caller')
  })

  it('keeps the heuristic review when there is no parsed output at all', () => {
    expect(mergeReview(base, null, 'nothing came back')).toEqual({ ...base, notes: 'nothing came back' })
  })
})

describe('claudeReview', () => {
  const fakeClient = (impl: () => Promise<{ parsed_output?: unknown }>): ReviewClient => ({
    messages: { parse: vi.fn(impl) },
  })

  it('skips the API entirely when there is no key', async () => {
    vi.stubEnv('ANTHROPIC_API_KEY', '')
    const review = await claudeReview(fixture())
    expect(review.source).toBe('heuristic')
    expect(review.notes).toMatch(/ANTHROPIC_API_KEY is not set/)
  })

  it('sends the untrusted-artifact system prompt and merges what comes back', async () => {
    const client = fakeClient(async () => ({
      parsed_output: {
        description: 'Alpha documents itself.',
        category: 'docs',
        type: 'demo',
        tools: [{ name: 'open_page', kind: 'sensitive', reason: 'navigation can submit a form' }],
        suspicious: [],
        strengths: [],
        improvements: [],
        suggested_journeys: [],
        recommendation: 'needs-manual-review',
        notes: 'One tool needed a stricter kind.',
      },
    }))
    const review = await claudeReview(fixture(), { client, model: 'claude-opus-5' })
    expect(review.source).toBe('claude')
    expect(review.description).toBe('Alpha documents itself.')
    expect(review.tools.find((t) => t.name === 'open_page')?.kind).toBe('sensitive')
    expect(review.recommendation).toBe('needs-manual-review')

    const params = (client.messages.parse as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(params.model).toBe('claude-opus-5')
    expect(params.max_tokens).toBe(16000)
    expect(params).not.toHaveProperty('thinking')
    expect(params.output_config).toHaveProperty('format')
    expect(String(params.system)).toMatch(/UNTRUSTED DATA/)
    expect(String(params.system)).toMatch(/never state or imply that it is safe/)
    expect(String((params.messages as { content: string }[])[0].content)).toContain('<scan_artifact>')
  })

  it('honours ATLAS_REVIEW_MODEL', async () => {
    const client = fakeClient(async () => ({ parsed_output: undefined }))
    vi.stubEnv('ATLAS_REVIEW_MODEL', 'claude-sonnet-5')
    await claudeReview(fixture(), { client })
    expect((client.messages.parse as ReturnType<typeof vi.fn>).mock.calls[0][0].model).toBe('claude-sonnet-5')
  })

  it('falls back to the heuristic review when the SDK throws, and says why', async () => {
    const client = fakeClient(async () => {
      throw new Anthropic.RateLimitError(429, undefined, 'slow down', new Headers())
    })
    const review = await claudeReview(fixture(), { client })
    expect(review.source).toBe('heuristic')
    expect(review.notes).toMatch(/rate-limited/)
    expect(review.tools).toEqual(heuristicReview(fixture()).tools)
  })

  it('falls back when the model returns nothing parseable', async () => {
    const review = await claudeReview(fixture(), { client: fakeClient(async () => ({ parsed_output: undefined })) })
    expect(review.source).toBe('heuristic')
    expect(review.notes).toMatch(/no parsed output/)
  })
})

describe('fallbackReason', () => {
  it('names the typed SDK errors', () => {
    expect(fallbackReason(new Anthropic.AuthenticationError(401, undefined, 'nope', new Headers()))).toMatch(/authentication error/)
    expect(fallbackReason(new Anthropic.RateLimitError(429, undefined, 'nope', new Headers()))).toMatch(/rate-limited/)
    expect(fallbackReason(new Anthropic.APIError(500, undefined, 'boom', new Headers()))).toMatch(/\(500\)/)
    expect(fallbackReason(new Error('socket hang up'))).toMatch(/socket hang up/)
  })
})
