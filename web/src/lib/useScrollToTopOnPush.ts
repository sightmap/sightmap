import { useEffect } from 'react'
import { useLocation, useNavigationType } from 'react-router'

/**
 * Sends the window back to the top when a link click lands on a new page.
 *
 * React Router leaves the scroll offset alone across a client-side navigation,
 * so a click from halfway down an index page opens the article halfway down.
 * Only PUSH is reset — POP is the back button and the initial load, where the
 * browser's own scroll restoration is the right answer — and a hash is left
 * alone, since it means the visitor asked for a specific heading.
 *
 * Keyed on `pathname` rather than on the caller's slug: both callers reuse one
 * component instance when navigating between siblings (post to post, entry to
 * entry) instead of remounting, so mount alone is not enough to catch every
 * arrival. It also deliberately ignores the query string, so a page that keeps
 * state in the URL — /atlas writes its category and search filters there —
 * doesn't yank the visitor upward every time a filter changes.
 *
 * For detail pages only. An index page that pushes query-string state would
 * additionally re-run this whenever `navigationType` flips, which for /atlas
 * would mean a chip click scrolling the grid out from under the chips.
 */
export function useScrollToTopOnPush(): void {
  const navigationType = useNavigationType()
  const { pathname, hash } = useLocation()

  useEffect(() => {
    if (navigationType !== 'PUSH' || hash) return
    // 'instant', not the default: `html { scroll-behavior: smooth }` would
    // otherwise animate the whole page back up.
    window.scrollTo({ top: 0, left: 0, behavior: 'instant' })
  }, [pathname, navigationType, hash])
}
