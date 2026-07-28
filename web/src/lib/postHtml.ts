// Bridges prerendered post HTML to the client.
//
// scripts/prerender.tsx writes the post body into a JSON script tag on the
// page. Reading it synchronously on first render is what lets hydration match
// the server markup. Client-side navigation to a different post finds no tag
// and falls back to a dynamic import, which keeps post bodies out of the main
// bundle instead of growing it with every post we publish.

const TAG_ID = '__SIGHTMAP_POST__'

export function readPostHtml(slug: string): string | null {
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
