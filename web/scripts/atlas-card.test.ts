// The pure half of `pnpm atlas:card`: the arguments, and the record it would
// write. The Blobs call itself is a five-line wrapper and is not tested here.

import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { parseArgs, readReport, run, scanFromReport, withScan } from './atlas-card'
import { TRY_TTL_MS, type TryRecord } from '../netlify/lib/try-record'
import type { ScanReport } from '../src/types/directory'

const SCAN = path.resolve(__dirname, 'lib/__fixtures__/directory/scans/alpha-tools/2026-09-08.json')
const report = (over: Partial<ScanReport> = {}): ScanReport => ({ ...readReport(SCAN), ...over })

const record = (over: Partial<TryRecord> = {}): TryRecord => ({
  v: 1,
  host: 'alpha.example.org',
  url: 'https://alpha.example.org/',
  submissionId: 'abc',
  claimedAt: '2026-09-01T00:00:00.000Z',
  expiresAt: '2026-10-01T00:00:00.000Z',
  ...over,
})

describe('parseArgs', () => {
  it('reads the host and the scan path', () => {
    expect(parseArgs(['--host', 'example.org', '--scan', 'scans/x/2026-09-08.json'])).toEqual({
      host: 'example.org',
      scan: 'scans/x/2026-09-08.json',
    })
  })

  it('needs a scan report', () => {
    expect(() => parseArgs(['--host', 'example.org'])).toThrow(/usage/)
    expect(() => parseArgs(['--nope'])).toThrow(/unknown argument/)
  })
})

describe('scanFromReport', () => {
  it('carries what the card shows and nothing else', () => {
    const scan = scanFromReport(report())
    const source = report()

    expect(scan.scannedAt).toBe(source.scannedAt)
    expect(scan.status).toBe(source.status)
    expect(scan.pages).toBe(source.counts.pages)
    expect(scan.tools).toHaveLength(source.tools.length)
    expect(scan.tools[0]).toEqual({
      name: source.tools[0]!.name,
      description: source.tools[0]!.description.trim(),
      // The Atlas kind, the same key KIND_LABEL is written in.
      kind: source.tools[0]!.risk,
      page: source.tools[0]!.page,
    })
    // No input schemas, no checks, no scanner identity: a card is not a listing.
    expect(Object.keys(scan.tools[0]!).sort()).toEqual(['description', 'kind', 'name', 'page'])
  })

  it('records an empty scan as an empty scan, not as no scan', () => {
    const scan = scanFromReport(report({ tools: [], status: 'api-absent' }))
    expect(scan.tools).toEqual([])
    expect(scan.status).toBe('api-absent')
  })
})

describe('withScan', () => {
  it('adds the scan and runs the card thirty days from the scan date', () => {
    const updated = withScan(record(), scanFromReport(report({ scannedAt: '2026-09-08T12:00:00.000Z' })))
    expect(updated.scan?.scannedAt).toBe('2026-09-08T12:00:00.000Z')
    expect(new Date(updated.expiresAt).getTime()).toBe(new Date('2026-09-08T12:00:00.000Z').getTime() + TRY_TTL_MS)
  })

  it('replaces a previous scan and leaves the claim alone', () => {
    const first = withScan(record(), scanFromReport(report({ scannedAt: '2026-09-08T00:00:00.000Z' })))
    const second = withScan(first, scanFromReport(report({ scannedAt: '2026-10-08T00:00:00.000Z', tools: [] })))

    expect(second.scan?.tools).toEqual([])
    expect(second.claimedAt).toBe(record().claimedAt)
    expect(second.submissionId).toBe(record().submissionId)
    expect(new Date(second.expiresAt).getTime()).toBeGreaterThan(new Date(first.expiresAt).getTime())
  })
})

describe('readReport', () => {
  it('refuses a file that is not a scan report', () => {
    const notAReport = path.resolve(__dirname, '../package.json')
    expect(fs.existsSync(notAReport)).toBe(true)
    expect(() => readReport(notAReport)).toThrow(/not a scan report/)
  })
})

describe('run', () => {
  it('refuses a --host that is not the host the report describes', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'atlas-card-'))
    const file = path.join(dir, 'scan.json')
    fs.writeFileSync(file, JSON.stringify({ ...report(), host: 'example.org' }))
    await expect(run(['--host', 'victim.example', '--scan', file])).rejects.toThrow(/does not match/)
  })

  it('refuses a report without a usable scan date', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'atlas-card-'))
    const file = path.join(dir, 'scan.json')
    const bad = { ...report(), scannedAt: 'never' }
    fs.writeFileSync(file, JSON.stringify(bad))
    await expect(run(['--host', bad.host, '--scan', file])).rejects.toThrow(/scannedAt/)
  })
})
