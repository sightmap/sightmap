import { useEffect, useRef, useState } from 'react'
import Logo from './Logo'

const LINKS: { href: string; label: string; external?: boolean }[] = [
  { href: 'https://docs.sightmap.org', label: 'Docs', external: true },
  { href: '/sightkick', label: 'Sightkick' },
  { href: '/atlas', label: 'Atlas' },
  { href: '/blog', label: 'Blog' },
]

const GH = 'https://github.com/sightmap/sightmap'

function GitHubMark() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
    </svg>
  )
}

export default function Navigation() {
  const [open, setOpen] = useState(false)
  const panel = useRef<HTMLDivElement>(null)

  // The menu covers the page, so Escape and any navigation have to close it,
  // and the body must not scroll behind it while it is open.
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false) }
    document.addEventListener('keydown', onKey)
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    panel.current?.querySelector<HTMLAnchorElement>('a')?.focus()
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = prev
    }
  }, [open])

  return (
    <nav data-component="Navigation">
      <a href="/" className="nav-logo" aria-label="sightmap home"><Logo /></a>

      <div className="nav-links">
        {LINKS.map((l) => (
          <a key={l.href} href={l.href} {...(l.external ? { target: '_blank', rel: 'noreferrer' } : {})}>
            {l.label}
          </a>
        ))}
        <a href={GH} target="_blank" rel="noreferrer" className="nav-gh">
          <GitHubMark />
          GitHub
        </a>
      </div>

      <button
        type="button"
        className="nav-toggle"
        aria-expanded={open}
        aria-controls="nav-menu"
        aria-label={open ? 'Close menu' : 'Open menu'}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="nav-toggle__bar" data-x={open ? 'true' : 'false'}></span>
        <span className="nav-toggle__bar" data-x={open ? 'true' : 'false'}></span>
      </button>

      <div
        id="nav-menu"
        ref={panel}
        className="nav-menu"
        data-open={open ? 'true' : 'false'}
        hidden={!open}
      >
        {LINKS.map((l) => (
          <a
            key={l.href}
            href={l.href}
            onClick={() => setOpen(false)}
            {...(l.external ? { target: '_blank', rel: 'noreferrer' } : {})}
          >
            {l.label}
          </a>
        ))}
        <a href={GH} target="_blank" rel="noreferrer" onClick={() => setOpen(false)}>
          <GitHubMark />
          GitHub
        </a>
      </div>
    </nav>
  )
}
