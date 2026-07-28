import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Light as SyntaxHighlighter } from 'react-syntax-highlighter'
import atomOneDarkImport from 'react-syntax-highlighter/dist/esm/styles/hljs/atom-one-dark'
import bashImport from 'react-syntax-highlighter/dist/esm/languages/hljs/bash'
import yamlImport from 'react-syntax-highlighter/dist/esm/languages/hljs/yaml'
import plaintextImport from 'react-syntax-highlighter/dist/esm/languages/hljs/plaintext'

// Ported from the Subtext marketing site's <CodeBlock>. That site switches
// the hljs theme with useTheme(); this site is light-only and the syntax
// panel is meant to always read as a dark, terminal-like surface (same as
// `.smap-widget__tree` / `.smap-svs`), so the theme hook is dropped entirely
// and atomOneDark is the only style.
//
// scripts/prerender.tsx runs this component through Node directly (via tsx,
// no Vite bundler in between). These dist/esm/* files have no "type":
// "module" in their package and no __esModule marker, so under tsx's
// require/import interop a default import resolves to the whole module
// object (`{ default: theRealThing }`) instead of theRealThing itself —
// Vite's bundler unwraps these correctly, so the break is prerender-only.
// Unwrap defensively so both environments end up with the real value.
function unwrapDefault<T>(mod: T | { default: T }): T {
  return mod !== null && typeof mod === 'object' && 'default' in mod
    ? (mod as { default: T }).default
    : (mod as T)
}
const atomOneDark = unwrapDefault(atomOneDarkImport)
const bash = unwrapDefault(bashImport)
const yaml = unwrapDefault(yamlImport)
const plaintext = unwrapDefault(plaintextImport)

// react-syntax-highlighter uses highlight.js under the hood. We use the
// "Light" build and register only the languages this blog's fenced code
// blocks actually use (bash, yaml) plus a plaintext fallback, to keep the
// bundle lean — unlike upstream, which registers a much larger set for a
// site with many more post authors and languages.
const LANGUAGES: Record<string, unknown> = {
  bash,
  yaml,
  plaintext,
}
for (const [name, mod] of Object.entries(LANGUAGES)) {
  SyntaxHighlighter.registerLanguage(name, mod)
}

// Map common fence aliases onto a registered hljs language.
const LANG_ALIASES: Record<string, string> = {
  sh: 'bash',
  shell: 'bash',
  zsh: 'bash',
  console: 'bash',
  yml: 'yaml',
  text: 'plaintext',
}

function resolveLanguage(language?: string): string {
  if (!language) return 'plaintext'
  const lower = language.toLowerCase()
  if (LANGUAGES[lower]) return lower
  return LANG_ALIASES[lower] ?? 'plaintext'
}

async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // fall through to the legacy path
  }
  try {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.style.position = 'fixed'
    textarea.style.top = '-9999px'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    const ok = document.execCommand('copy')
    textarea.remove()
    return ok
  } catch {
    return false
  }
}

const iconProps = {
  width: 16,
  height: 16,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 2,
  strokeLinecap: 'round' as const,
  strokeLinejoin: 'round' as const,
}

const CopyIcon = () => (
  <svg {...iconProps} aria-hidden="true">
    <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
  </svg>
)
const CheckIcon = () => (
  <svg {...iconProps} aria-hidden="true">
    <polyline points="20 6 9 17 4 12" />
  </svg>
)
const ExpandIcon = () => (
  <svg {...iconProps} aria-hidden="true">
    <polyline points="15 3 21 3 21 9" />
    <polyline points="9 21 3 21 3 15" />
    <line x1="21" y1="3" x2="14" y2="10" />
    <line x1="3" y1="21" x2="10" y2="14" />
  </svg>
)
const CloseIcon = () => (
  <svg {...iconProps} aria-hidden="true">
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

interface CodeBlockProps {
  code: string
  language?: string
}

export default function CodeBlock({ code, language }: CodeBlockProps) {
  const [copied, setCopied] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const resetTimer = useRef<number | undefined>(undefined)

  const label = language || 'code'
  const hlLanguage = resolveLanguage(language)

  useEffect(() => () => window.clearTimeout(resetTimer.current), [])

  // The overlay itself only ever mounts when `expanded` is true (see the JSX
  // below), so this effect — like all effects — never runs during SSR
  // (renderToString doesn't execute effects), and the `document`/`window`
  // references inside it are never reached during the initial server render.
  useEffect(() => {
    if (!expanded) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setExpanded(false)
    }
    document.addEventListener('keydown', onKey)
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = prevOverflow
    }
  }, [expanded])

  const handleCopy = async () => {
    const ok = await copyText(code)
    if (!ok) return
    setCopied(true)
    window.clearTimeout(resetTimer.current)
    resetTimer.current = window.setTimeout(() => setCopied(false), 2000)
  }

  // Line numbers are rendered by react-syntax-highlighter as part of each
  // line's box (showLineNumbers), so they share the code's exact
  // line-height/grid.
  const highlighter = (fontSize: string) => (
    <SyntaxHighlighter
      language={hlLanguage}
      style={atomOneDark}
      showLineNumbers
      customStyle={{
        margin: 0,
        padding: '1rem 1.25rem',
        background: 'transparent',
        border: 'none',
        borderRadius: 0,
        fontSize,
        lineHeight: 1.7,
      }}
      codeTagProps={{
        style: {
          fontFamily: 'var(--mono)',
          fontSize,
          lineHeight: 1.7,
          background: 'transparent',
        },
      }}
      lineNumberStyle={{
        minWidth: '2.5em',
        paddingRight: '1em',
        marginRight: '1em',
        borderRight: '1px solid var(--code-border)',
        color: 'var(--text-dim)',
        opacity: 0.55,
        textAlign: 'right',
        userSelect: 'none',
        fontFamily: 'var(--mono)',
      }}
    >
      {code}
    </SyntaxHighlighter>
  )

  const controls = (onSecondary: () => void, secondary: 'expand' | 'close') => (
    <div className="blog-code__controls">
      <button
        type="button"
        className={`blog-code__btn${copied ? ' is-copied' : ''}`}
        aria-label={copied ? 'Copied' : 'Copy code'}
        onClick={handleCopy}
      >
        {copied ? <CheckIcon /> : <CopyIcon />}
      </button>
      <button
        type="button"
        className="blog-code__btn"
        aria-label={secondary === 'expand' ? 'Expand code' : 'Close expanded code'}
        onClick={onSecondary}
      >
        {secondary === 'expand' ? <ExpandIcon /> : <CloseIcon />}
      </button>
    </div>
  )

  return (
    <>
      <div className="blog-code" data-component="CodeBlock">
        <div className="blog-code__header">
          <span className="blog-code__lang">{label}</span>
          {controls(() => setExpanded(true), 'expand')}
        </div>
        <div className="blog-code__body">{highlighter('0.8125rem')}</div>
      </div>

      {expanded &&
        createPortal(
          <div
            className="blog-code-overlay"
            role="dialog"
            aria-modal="true"
            aria-label="Expanded code block"
          >
            <div className="blog-code-overlay__backdrop" onClick={() => setExpanded(false)} />
            <div className="blog-code blog-code-overlay__panel">
              <div className="blog-code__header">
                <span className="blog-code__lang">{label}</span>
                {controls(() => setExpanded(false), 'close')}
              </div>
              <div className="blog-code__body">{highlighter('0.9375rem')}</div>
            </div>
          </div>,
          document.body
        )}
    </>
  )
}
