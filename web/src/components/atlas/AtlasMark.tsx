import { markColor, markInitial } from '@/lib/atlas'

/**
 * The stand-in for a site's favicon: a monogram tile drawn from the domain.
 * See the note in src/lib/atlas.ts for why the real favicon is not fetched.
 * `aria-hidden` because the domain it encodes is always rendered as text
 * beside it — announcing the letter again would just be noise.
 */
export default function AtlasMark({ domain, className = '' }: { domain: string; className?: string }) {
  return (
    <span
      className={`atlas-mark ${className}`.trim()}
      style={{ '--mark-color': markColor(domain) } as React.CSSProperties}
      aria-hidden="true"
    >
      {markInitial(domain)}
    </span>
  )
}
