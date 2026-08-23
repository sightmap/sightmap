// Structured JSON errors for the public HTTP API and for 404s negotiated
// as application/json. Agents cannot recover from an HTML error page.

export interface ApiErrorBody {
  error: {
    code: string
    message: string
    hint: string
    status: number
  }
}

export const NOT_FOUND_HINT =
  'See https://sightmap.org/llms.txt for the published Sightmap site map, https://sightmap.org/openapi.json for the HTTP API, https://sightmap.org/developers for Sightmap developer resources, or https://docs.sightmap.org for documentation.'

export function apiError(
  code: string,
  message: string,
  hint: string,
  status: number
): ApiErrorBody {
  return { error: { code, message, hint, status } }
}

export function notFoundError(path: string): ApiErrorBody {
  return apiError(
    'not_found',
    `No Sightmap resource at ${path}.`,
    NOT_FOUND_HINT,
    404
  )
}

export function methodNotAllowedError(method: string, path: string): ApiErrorBody {
  return apiError(
    'method_not_allowed',
    `${method} is not allowed on ${path}.`,
    'This API is read-only. Use GET or HEAD. See https://sightmap.org/openapi.json.',
    405
  )
}

export function notAcceptableBody(available: readonly string[], requested: string): string {
  const listed = available.map((t) => `- ${t}`).join('\n')
  return `This Sightmap resource is available as:\n${listed}\n\nYou requested: ${requested || '(none)'}\n`
}
