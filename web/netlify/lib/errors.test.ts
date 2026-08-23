import { describe, expect, it } from 'vitest'
import { methodNotAllowedError, notAcceptableBody, notFoundError } from './errors'

describe('notFoundError', () => {
  it('returns a JSON body with code, message, hint, and status', () => {
    const body = notFoundError('/nope')
    expect(body.error.code).toBe('not_found')
    expect(body.error.status).toBe(404)
    expect(body.error.message).toContain('/nope')
    expect(body.error.hint).toContain('https://sightmap.org/llms.txt')
    expect(body.error.hint).toContain('https://sightmap.org/openapi.json')
    expect(body.error.hint).toContain('https://docs.sightmap.org')
  })
})

describe('methodNotAllowedError', () => {
  it('tells the client the API is read-only', () => {
    const body = methodNotAllowedError('POST', '/api/atlas')
    expect(body.error.code).toBe('method_not_allowed')
    expect(body.error.status).toBe(405)
    expect(body.error.hint).toContain('GET')
  })
})

describe('notAcceptableBody', () => {
  it('lists the representations the server can produce', () => {
    const text = notAcceptableBody(['text/html', 'text/markdown'], 'application/pdf')
    expect(text).toContain('- text/html')
    expect(text).toContain('- text/markdown')
    expect(text).toContain('application/pdf')
  })
})
