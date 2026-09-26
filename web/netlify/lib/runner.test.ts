// Tests for the runner half of the submission endpoint: the daily ceiling, the
// prompt, the two trigger payloads. Split out of submit.test.ts alongside the
// code. Every assertion that the email address does not travel lives here,
// because this is the only side that talks to anything outside the function.

import { describe, expect, it, vi } from 'vitest'
import { MAX_INTENT_LENGTH } from '../../src/lib/submit-types.ts'
import { buildRecord, type ValidSubmission } from './submit'
import {
  buildGithubDispatchRequest,
  buildNetlifyRunnerRequest,
  buildRunnerPrompt,
  clampPrompt,
  dailyRunCeiling,
  dailyRunsKey,
  DEFAULT_DAILY_RUNS,
  cardCommand,
  intakeCommand,
  MAX_PROMPT_LENGTH,
  parseRunCount,
  runnerInput,
  runnerKind,
  shellQuote,
  shouldTriggerRunner,
  submittedBy,
  submitSalt,
  triggerRunner,
  type RunnerPromptInput,
} from './runner'

const EMAIL = 'chip@example.com'

const VALUE: ValidSubmission = {
  url: 'https://example.org/pricing',
  host: 'example.org',
  email: EMAIL,
  emailNormalized: EMAIL,
  owner: true,
  sightkick: true,
  intent: 'Find the pricing page',
  nominate: false,
  rescan: false,
}

function record() {
  return buildRecord(VALUE, {
    id: 'abc123',
    receivedAt: '2026-09-10T12:00:00.000Z',
    emailHash: 'deadbeef',
    ipHash: 'cafef00d',
    userAgent: 'Mozilla/5.0',
    runner: { kind: 'netlify' },
  })
}

// ---------------------------------------------------------------------------

const PROMPT_INPUT: RunnerPromptInput = {
  id: 'abc123',
  url: 'https://example.org/pricing',
  host: 'example.org',
  intent: 'Find the pricing page',
  owner: true,
  sightkick: true,
  nominate: false,
  rescan: false,
  emailHash: 'deadbeef',
}

describe('buildRunnerPrompt', () => {
  const prompt = buildRunnerPrompt(PROMPT_INPUT)

  it('points the agent at the pipeline document', () => {
    expect(prompt).toContain('web/ATLAS_PIPELINE.md')
  })

  it('carries the submission and the intake command', () => {
    expect(prompt).toContain('abc123')
    expect(prompt).toContain('https://example.org/pricing')
    expect(prompt).toContain('owner=true sightkick=true nominate=false rescan=false')
    expect(prompt).toContain('deadbeef')
    expect(prompt).toContain('pnpm atlas:intake')
    expect(prompt).toContain("--submission-id 'abc123'")
    expect(prompt).toContain("--intent 'Find the pricing page'")
    expect(prompt).toContain('--sightkick')
    expect(prompt).toContain("--submitted-by 'owner'")
  })

  it('updates the launch card straight after the intake', () => {
    expect(prompt).toContain("pnpm atlas:card --host 'example.org'")
    // Only the intake knows where it filed the report, so the prompt says so
    // rather than guessing a path.
    expect(prompt).toContain('the scan report path intake printed')
    expect(prompt).toContain('Skip it when the intake wrote no scan report.')
    expect(prompt.indexOf('atlas:card')).toBeGreaterThan(prompt.indexOf('atlas:intake'))
  })

  it('asks for tests, a build, and a PR it must not merge', () => {
    expect(prompt).toContain('pnpm test')
    expect(prompt).toContain('pnpm build')
    expect(prompt).toContain('atlas: list example.org')
    expect(prompt).toContain('intake summary')
    expect(prompt).toMatch(/never publish|Never publish/)
    expect(prompt).toContain('do not merge')
  })

  it('shell-quotes the card command the same way', () => {
    const command = cardCommand({ host: "ex'ample.org" }, "/tmp/a b/scan'.json")
    expect(command).toBe("pnpm atlas:card --host 'ex'\\''ample.org' --scan '/tmp/a b/scan'\\''.json'")
  })

  it('never contains the email address', () => {
    expect(prompt).not.toContain(EMAIL)
    expect(prompt).not.toContain('@example.com')
  })

  it('stays plain text and under the size budget, even with a maximal intent', () => {
    expect(prompt.length).toBeLessThan(MAX_PROMPT_LENGTH)
    const big = buildRunnerPrompt({ ...PROMPT_INPUT, intent: 'x'.repeat(MAX_INTENT_LENGTH) })
    expect(big.length).toBeLessThanOrEqual(MAX_PROMPT_LENGTH)
    expect(big).not.toContain('```')
    // The budget exists to bound the prompt, not to trim the paragraph that
    // keeps the agent from publishing.
    expect(big).not.toContain('[truncated]')
    expect(big).toContain('do not merge')
  })

  it('clamps a prompt that outgrows the budget', () => {
    expect(clampPrompt('y'.repeat(50), 20)).toHaveLength(20)
    expect(clampPrompt('y'.repeat(50), 20)).toContain('[truncated]')
  })

  it('marks a nominated site as nominator and omits --sightkick when false', () => {
    const command = intakeCommand({ ...PROMPT_INPUT, owner: false, nominate: true, sightkick: false, intent: '' })
    expect(command).toContain("--submitted-by 'nominator'")
    expect(command).not.toContain('--sightkick')
    expect(command).not.toContain('--intent')
  })
})

// ---------------------------------------------------------------------------

describe('intakeCommand shell safety', () => {
  const HOSTILE: RunnerPromptInput = {
    ...PROMPT_INPUT,
    url: 'https://example.org/p?q=$(id)',
    intent: 'run $(id) `whoami` now',
  }

  it('leaves no shell metacharacter unquoted', () => {
    const command = intakeCommand(HOSTILE)
    // A double-quoted shell word still expands $(...), `...` and \ — the value
    // has to arrive inside single quotes for the runner to paste it safely.
    expect(command).not.toContain('"')
    expect(command).toContain(`--url 'https://example.org/p?q=$(id)'`)
    expect(command).toContain(`--intent 'run $(id) ` + '`whoami`' + ` now'`)
  })

  it("closes, escapes and reopens each single quote ('\\'')", () => {
    // Pasted into sh, each of these is exactly one word.
    expect(shellQuote("it's")).toBe(String.raw`'it'\''s'`)
    expect(shellQuote("a'; rm -rf /; echo '")).toBe(String.raw`'a'\''; rm -rf /; echo '\'''`)
  })

  it('quotes an empty string as an empty word rather than nothing', () => {
    expect(shellQuote('')).toBe("''")
  })
})

describe('submittedBy', () => {
  it('is nominator unless the owner box was ticked', () => {
    expect(submittedBy({ owner: true, nominate: false })).toBe('owner')
    expect(submittedBy({ owner: true, nominate: true })).toBe('owner')
    expect(submittedBy({ owner: false, nominate: true })).toBe('nominator')
    // Neither box: the submitter has not claimed the site, so it is a
    // nomination — never 'maintainer', which is a hand-run intake only.
    expect(submittedBy({ owner: false, nominate: false })).toBe('nominator')
  })

  it('never claims ownership on an unticked web submission', () => {
    const command = intakeCommand({ ...PROMPT_INPUT, owner: false, nominate: false })
    expect(command).toContain("--submitted-by 'nominator'")
    expect(command).not.toContain('owner')
  })
})

describe('buildNetlifyRunnerRequest', () => {
  it('posts to the agent_runners endpoint the CLI uses', () => {
    const spec = buildNetlifyRunnerRequest({ siteId: 'site-1', token: 'tok', prompt: 'hello' })
    expect(spec.url).toBe('https://api.netlify.com/api/v1/agent_runners?site_id=site-1')
    expect(spec.init.method).toBe('POST')
    expect(spec.init.headers).toMatchObject({
      Authorization: 'Bearer tok',
      'Content-Type': 'application/json',
    })
    expect(JSON.parse(String(spec.init.body))).toEqual({ prompt: 'hello', agent: 'claude' })
  })

  it('includes branch and model only when they are configured', () => {
    const spec = buildNetlifyRunnerRequest({
      siteId: 'site-1',
      token: 'tok',
      prompt: 'hello',
      branch: 'main',
      model: 'claude-opus-4',
    })
    expect(JSON.parse(String(spec.init.body))).toEqual({
      prompt: 'hello',
      agent: 'claude',
      branch: 'main',
      model: 'claude-opus-4',
    })
  })
})

describe('buildGithubDispatchRequest', () => {
  it('dispatches atlas-submission with a payload that carries no email', () => {
    const spec = buildGithubDispatchRequest({ repo: 'sightmap/sightmap', token: 'gh', input: PROMPT_INPUT })
    expect(spec.url).toBe('https://api.github.com/repos/sightmap/sightmap/dispatches')
    expect(spec.init.headers).toMatchObject({
      Authorization: 'Bearer gh',
      Accept: 'application/vnd.github+json',
    })
    const body = JSON.parse(String(spec.init.body))
    expect(body.event_type).toBe('atlas-submission')
    expect(body.client_payload).toEqual({
      submission_id: 'abc123',
      url: 'https://example.org/pricing',
      intent: 'Find the pricing page',
      owner: true,
      sightkick: true,
      nominate: false,
      rescan: false,
      email_hash: 'deadbeef',
    })
    expect(String(spec.init.body)).not.toContain(EMAIL)
    expect(Object.keys(body.client_payload)).not.toContain('email')
  })
})

describe('triggerRunner', () => {
  it('records the runner id and state returned by Netlify', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ id: 'run_9', state: 'queued' }), { status: 201 }))
    const info = await triggerRunner(PROMPT_INPUT, { ATLAS_RUNNER: 'netlify', NETLIFY_AGENT_TOKEN: 't', SITE_ID: 'site-2' }, fetchMock as unknown as typeof fetch)
    expect(info).toEqual({ kind: 'netlify', id: 'run_9', state: 'queued' })
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toContain('site_id=site-2')
    expect(String(init.body)).not.toContain(EMAIL)
  })

  it('prefers NETLIFY_SITE_ID over the runtime SITE_ID', async () => {
    const fetchMock = vi.fn(async () => new Response('{}', { status: 201 }))
    await triggerRunner(
      PROMPT_INPUT,
      { ATLAS_RUNNER: 'netlify', NETLIFY_AGENT_TOKEN: 't', NETLIFY_SITE_ID: 'explicit', SITE_ID: 'runtime' },
      fetchMock as unknown as typeof fetch
    )
    expect(String((fetchMock.mock.calls[0] as unknown as [string])[0])).toContain('site_id=explicit')
  })

  it('records a failure instead of throwing when the API rejects the call', async () => {
    const fetchMock = vi.fn(async () => new Response('nope', { status: 401 }))
    const info = await triggerRunner(PROMPT_INPUT, { ATLAS_RUNNER: 'netlify', NETLIFY_AGENT_TOKEN: 't', SITE_ID: 's' }, fetchMock as unknown as typeof fetch)
    expect(info.kind).toBe('netlify')
    expect(info.error).toContain('HTTP 401')
    expect(info.id).toBeUndefined()
  })

  it('records a network failure', async () => {
    const fetchMock = vi.fn(async () => {
      throw new Error('socket hang up')
    })
    const info = await triggerRunner(PROMPT_INPUT, { ATLAS_RUNNER: 'netlify', NETLIFY_AGENT_TOKEN: 't', SITE_ID: 's' }, fetchMock as unknown as typeof fetch)
    expect(info.error).toBe('socket hang up')
  })

  it('reports missing credentials without calling out', async () => {
    const fetchMock = vi.fn()
    const info = await triggerRunner(PROMPT_INPUT, { ATLAS_RUNNER: 'netlify', SITE_ID: 's' }, fetchMock as unknown as typeof fetch)
    expect(info.error).toContain('NETLIFY_AGENT_TOKEN')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('dispatches to GitHub, defaulting the repo', async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }))
    const info = await triggerRunner(PROMPT_INPUT, { ATLAS_RUNNER: 'github', ATLAS_GITHUB_TOKEN: 'gh' }, fetchMock as unknown as typeof fetch)
    expect(info).toEqual({ kind: 'github', state: 'dispatched' })
    expect(String((fetchMock.mock.calls[0] as unknown as [string])[0])).toBe(
      'https://api.github.com/repos/sightmap/sightmap/dispatches'
    )
  })

  it('stores only when the runner is queue or unset', async () => {
    const fetchMock = vi.fn()
    expect(await triggerRunner(PROMPT_INPUT, {}, fetchMock as unknown as typeof fetch)).toEqual({ kind: 'queue' })
    expect(await triggerRunner(PROMPT_INPUT, { ATLAS_RUNNER: 'queue' }, fetchMock as unknown as typeof fetch)).toEqual({
      kind: 'queue',
    })
    expect(await triggerRunner(PROMPT_INPUT, { ATLAS_RUNNER: 'nonsense' }, fetchMock as unknown as typeof fetch)).toEqual({
      kind: 'queue',
    })
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('reads the runner kind and salt from the environment', () => {
    expect(runnerKind({ ATLAS_RUNNER: ' Netlify ' })).toBe('netlify')
    expect(runnerKind({})).toBe('queue')
    expect(submitSalt({})).toBe('atlas')
    expect(submitSalt({ ATLAS_SUBMIT_SALT: ' pepper ' })).toBe('pepper')
  })

  it('never puts the email in the runner input', () => {
    expect(JSON.stringify(runnerInput(record()))).not.toContain(EMAIL)
  })
})
// ---------------------------------------------------------------------------

describe('daily runner ceiling', () => {
  it('files the counter by UTC day, whatever the local zone', () => {
    expect(dailyRunsKey('2026-09-10T23:59:59.000Z')).toBe('runs/2026-09-10')
    expect(dailyRunsKey(new Date('2026-09-11T00:00:01.000Z'))).toBe('runs/2026-09-11')
  })

  it('reads the ceiling from the environment and falls back to the default', () => {
    expect(dailyRunCeiling({})).toBe(DEFAULT_DAILY_RUNS)
    expect(dailyRunCeiling({ ATLAS_DAILY_RUNS: ' 3 ' })).toBe(3)
    expect(dailyRunCeiling({ ATLAS_DAILY_RUNS: '0' })).toBe(0)
    expect(dailyRunCeiling({ ATLAS_DAILY_RUNS: '7.9' })).toBe(7)
    expect(dailyRunCeiling({ ATLAS_DAILY_RUNS: 'plenty' })).toBe(DEFAULT_DAILY_RUNS)
    expect(dailyRunCeiling({ ATLAS_DAILY_RUNS: '-1' })).toBe(DEFAULT_DAILY_RUNS)
  })

  it('triggers up to the ceiling and then stops', () => {
    expect(shouldTriggerRunner(0, 2)).toBe(true)
    expect(shouldTriggerRunner(1, 2)).toBe(true)
    expect(shouldTriggerRunner(2, 2)).toBe(false)
    expect(shouldTriggerRunner(99, 2)).toBe(false)
    expect(shouldTriggerRunner(0, 0)).toBe(false)
  })

  it('fails open when the counter could not be read', () => {
    expect(shouldTriggerRunner(null, 2)).toBe(true)
    expect(shouldTriggerRunner(Number.NaN, 2)).toBe(true)
  })

  it('never triggers at a ceiling of 0, counter or no counter', () => {
    // ATLAS_DAILY_RUNS=0 is how runs are turned off. An unreadable counter is
    // not a reason to start one anyway.
    expect(shouldTriggerRunner(null, 0)).toBe(false)
    expect(shouldTriggerRunner(Number.NaN, 0)).toBe(false)
    expect(shouldTriggerRunner(0, 0)).toBe(false)
  })

  it('defaults the ceiling to 20 when none is passed', () => {
    expect(shouldTriggerRunner(19)).toBe(true)
    expect(shouldTriggerRunner(20)).toBe(false)
  })

  it('treats unreadable stored counters as unknown', () => {
    expect(parseRunCount({ count: 4 })).toBe(4)
    expect(parseRunCount(4)).toBe(4)
    expect(parseRunCount(null)).toBeNull()
    expect(parseRunCount('garbage')).toBeNull()
    expect(parseRunCount({ count: 'x' })).toBeNull()
  })

  it('marks a ceiling-skipped submission queued, with no runner id', () => {
    const queued = buildRecord(VALUE, {
      id: 'abc123',
      receivedAt: '2026-09-10T12:00:00.000Z',
      emailHash: 'deadbeef',
      ipHash: 'cafef00d',
      userAgent: 'Mozilla/5.0',
      runner: { kind: 'netlify', skipped: 'daily-ceiling' },
      state: 'queued',
    })
    expect(queued.state).toBe('queued')
    expect(queued.runner).toEqual({ kind: 'netlify', skipped: 'daily-ceiling' })
    // The default is unchanged: a submission whose runner fired is still 'new'.
    expect(record().state).toBe('new')
  })
})
