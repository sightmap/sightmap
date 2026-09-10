// The record behind an unlisted launch card at /try/<host>: what the submit
// function stores once a claim is verified, and what the runner adds after a
// scan. Kept apart from SubmissionRecord because the card must never see the
// submitter's details, only the host, the dates, and the observed tools.
//
// Nothing here is an endorsement. A record proves that someone controlling
// the host asked to be scanned and, once `scan` is present, what the scanner
// observed on a given date. Admission to the Atlas is a separate, reviewed
// step; the card redirects there when it happens.
import type { ScanStatus, ToolKind } from '../../src/types/directory'

export const TRY_STORE = 'atlas-try'
export const QUARANTINE_STORE = 'atlas-quarantine'

/** How long a card lives without a fresh scan. */
export const TRY_TTL_MS = 30 * 24 * 60 * 60 * 1000

export interface TryTool {
  name: string
  description: string
  kind: ToolKind
  page: string
}

export interface TryScan {
  scannedAt: string
  status: ScanStatus
  pages: number
  tools: TryTool[]
}

export interface TryRecord {
  v: 1
  host: string
  url: string
  submissionId: string
  claimedAt: string
  expiresAt: string
  scan?: TryScan
}

export interface QuarantineRecord {
  at: string
  reason?: string
}

export function expiresAfter(fromIso: string, ttlMs = TRY_TTL_MS): string {
  return new Date(new Date(fromIso).getTime() + ttlMs).toISOString()
}

export function isExpired(record: TryRecord, now = Date.now()): boolean {
  return new Date(record.expiresAt).getTime() <= now
}
