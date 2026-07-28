// Bridges prerendered post HTML to the client.
//
// scripts/prerender.tsx writes the post body into a JSON script tag on the
// page. Reading it synchronously on first render is what lets hydration match
// the server markup. Client-side navigation to a different post finds no tag
// and falls back to a dynamic import, which keeps post bodies out of the main
// bundle instead of growing it with every post we publish.

const TAG_ID = '__SIGHTMAP_POST__'

// Server side of the same bridge. During `renderToString` there is no
// `document`, so the tag lookup below can't run and the prerendered page would
// ship a post page with no article body at all — the exact thing the prerender
// exists to produce.
//
// A module-level slot the prerender sets around each render, rather than a
// React context, because BlogPost must read the body inside a `useState` lazy
// initializer for hydration to match. A context would have to be provided on
// the client too, and the only value the client could provide is the DOM
// lookup this module already does — so the context would add a provider to the
// shipped tree and still route back here. One module, one lookup, no provider.
//
// Module state is safe here: `scripts/prerender.tsx` is single-threaded and
// `renderToString` is synchronous, so the set/render/clear sequence can't
// interleave with another post.
let serverPost: { slug: string; html: string } | null = null

export function setServerPostHtml(slug: string, html: string): void {
  serverPost = { slug, html }
}

export function clearServerPostHtml(): void {
  serverPost = null
}

export function readPostHtml(slug: string): string | null {
  if (serverPost) return serverPost.slug === slug ? serverPost.html : null
  if (typeof document === 'undefined') return null
  const el = document.getElementById(TAG_ID)
  if (!el?.textContent) return null
  try {
    const data = JSON.parse(el.textContent) as { slug: string; html: string }
    return data.slug === slug ? data.html : null
  } catch {
    return null
  }
}

export async function loadPostHtml(slug: string): Promise<string> {
  const mod = (await import(`../generated/blog-posts/${slug}.ts`)) as { html: string }
  return mod.html
}
