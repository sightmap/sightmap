import { useEffect, useRef, useState } from 'react'
import { copyText } from '@/lib/clipboard'

/**
 * The detail page's call to action: the one command that pulls this entry down
 * locally, with a copy button.
 *
 * The verb is namespaced because `sightmap search` already means the local
 * corpus, so sightmap/sightmap#168 settled on `sightmap atlas list` / `find` /
 * `add`. This is the only place the string is written; the sibling verbs are
 * spelled out in the llms.txt atlas section (scripts/generate-feeds.ts).
 */
export const installCommand = (slug: string): string => `sightmap atlas add ${slug}`

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
