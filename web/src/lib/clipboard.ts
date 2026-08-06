/**
 * Copy `text` to the clipboard, reporting whether it worked.
 *
 * The async Clipboard API is the happy path; the off-screen-textarea +
 * execCommand fallback covers non-secure contexts (a plain-http preview) and
 * browsers that reject the permission, where `navigator.clipboard` either
 * doesn't exist or throws. Callers use the boolean to avoid flashing a
 * "Copied" state for a copy that silently didn't happen.
 *
 * Lives here rather than in the component that first needed it because both
 * blog code blocks and the atlas install block copy a command on click.
 */
export async function copyText(text: string): Promise<boolean> {
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
