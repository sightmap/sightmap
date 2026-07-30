// netlify/edge-functions/geo-region.ts
//
// Injects the visitor's consent region into app HTML by replacing the
// <!--CC_REGION--> placeholder. Runs on HTML documents only; everything else
// passes through untouched. If this never runs, the placeholder stays and the
// inline gate treats the region as REQUIRED (fail closed).
import type { Context } from '@netlify/edge-functions'
import { classifyRegion } from '../lib/regions.ts'

export default async function handler(
  _request: Request,
  context: Context
): Promise<Response | undefined> {
  const res = await context.next()
  const contentType = res.headers.get('content-type') || ''
  if (!contentType.includes('text/html')) return res

  const html = await res.text()
  if (!html.includes('<!--CC_REGION-->')) return new Response(html, res)

  const region = classifyRegion(context.geo?.country?.code)
  const injected = html.replace(
    '<!--CC_REGION-->',
    `<script>window.__CC_REGION__=${JSON.stringify(region)}</script>`
  )
  return new Response(injected, res)
}
