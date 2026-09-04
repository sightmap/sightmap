import { useEffect, useRef, useState } from 'react'

interface Props {
  /** The exact text placed on the clipboard. */
  value: string
  /** Button face before a copy. Defaults to the value, which suits a short command. */
  label?: React.ReactNode
  /** Face after a successful copy. */
  done?: React.ReactNode
  className?: string
  title?: string
}

// Copy-to-clipboard button. navigator.clipboard is unavailable on insecure
// origins and can reject even on https (permissions, a headless capture
// context), so every path falls back to the execCommand textarea trick and a
// failed copy leaves the label untouched rather than claiming success.
export default function CopyButton({ value, label, done = 'Copied', className, title }: Props) {
  const [copied, setCopied] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  // The label reverts on a timer, so a click-then-unmount would otherwise set
  // state on a gone component.
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current) }, [])

  const flash = () => {
    setCopied(true)
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => setCopied(false), 2000)
  }

  const fallback = () => {
    try {
      const ta = document.createElement('textarea')
      ta.value = value
      ta.setAttribute('readonly', '')
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
      flash()
    } catch {
      /* no clipboard available — leave the label alone */
    }
  }

  const copy = () => {
    try {
      if (navigator.clipboard?.writeText) {
        navigator.clipboard.writeText(value).then(flash, fallback)
      } else {
        fallback()
      }
    } catch {
      fallback()
    }
  }

  return (
    <button
      type="button"
      onClick={copy}
      className={className}
      title={title ?? `Copy: ${value}`}
      aria-live="polite"
    >
      {copied ? done : (label ?? value)}
    </button>
  )
}
