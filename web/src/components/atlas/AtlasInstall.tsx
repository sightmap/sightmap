import { useEffect, useRef, useState } from 'react'
import { copyText } from '@/lib/clipboard'

/**
 * The detail page's call to action: the one command that pulls this entry down
 * locally, with a copy button.
 *
 * `sightmap add` ships in sightmap/sightmap#168, which was still in review when
 * this page was built — the command is rendered anyway, on the handoff's
 * instruction, because it is the page's whole point. If that PR lands under a
 * different verb, this is the only place the string is written.
 */
export const installCommand = (slug: string): string => `sightmap add ${slug}`

export default function AtlasInstall({ slug }: { slug: string }) {
  const command = installCommand(slug)
  const [copied, setCopied] = useState(false)
  const resetTimer = useRef<number | undefined>(undefined)

  useEffect(() => () => window.clearTimeout(resetTimer.current), [])

  const handleCopy = async () => {
    if (!(await copyText(command))) return
    setCopied(true)
    window.clearTimeout(resetTimer.current)
    resetTimer.current = window.setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="atlas-install" data-component="AtlasInstall">
      <code className="atlas-install__cmd">
        <span className="atlas-install__prompt" aria-hidden="true">
          $
        </span>
        {command}
      </code>
      <button
        type="button"
        className="atlas-install__copy"
        onClick={handleCopy}
        aria-label={`Copy "${command}" to the clipboard`}
      >
        {copied ? 'Copied' : 'Copy'}
      </button>
    </div>
  )
}
