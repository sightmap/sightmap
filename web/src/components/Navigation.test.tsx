// @vitest-environment happy-dom
//
// The body-scroll lock in Navigation is driven by React `open` state, while the
// panel/toggle visibility is driven by a CSS `@media (max-width: 820px)` rule.
// index.css owns that rule, so the only way to verify the lock releases on a
// desktop resize across the breakpoint is to drive a fake MediaQueryList by
// hand. happy-dom gives us a DOM to mount into; the matchMedia stub below gives
// us the breakpoint transitions we need without a real layout engine.
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Navigation from './Navigation'

// `act` only flushes effects when React knows it is running inside an act
// scope; flip the global flag so the warnings stay out of the test output.
;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

// A controllable stand-in for MediaQueryList. The component subscribes to its
// `change` event; tests call `fire(matches)` to simulate the viewport crossing
// the 821px boundary.
interface FakeMQL {
  matches: boolean
  media: string
  addEventListener(type: 'change', cb: (e: { matches: boolean }) => void): void
  removeEventListener(type: 'change', cb: (e: { matches: boolean }) => void): void
}

function createFakeMQL(media: string, matches: boolean) {
  const listeners = new Set<(e: { matches: boolean }) => void>()
  const mql: FakeMQL & {
    fire(next: boolean): void
    listenerCount: () => number
  } = {
    matches,
    media,
    addEventListener: (_t, cb) => void listeners.add(cb),
    removeEventListener: (_t, cb) => void listeners.delete(cb),
    fire: (next) => {
      mql.matches = next
      for (const cb of listeners) cb({ matches: next })
    },
    listenerCount: () => listeners.size,
  }
  return mql
}

let root: ReturnType<typeof createRoot> | null = null
let container: HTMLElement | null = null
let mql: ReturnType<typeof createFakeMQL> | null = null
let matchMediaSpy: ReturnType<typeof vi.fn> | null = null

function mount() {
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)
  act(() => {
    root!.render(<Navigation />)
  })
}

function unmount() {
  if (root) {
    act(() => root!.unmount())
  }
  if (container && container.parentNode) container.parentNode.removeChild(container)
  root = null
  container = null
}

function toggle(): HTMLButtonElement {
  return container!.querySelector<HTMLButtonElement>('button.nav-toggle')!
}

beforeEach(() => {
  // Start each test with a clean body and a fresh "mobile" breakpoint so the
  // hamburger toggle is the actionable surface, mirroring how the bug is
  // actually triggered: open the menu, then widen the window.
  document.body.style.overflow = ''
  mql = createFakeMQL('(max-width: 820px)', true)
  matchMediaSpy = vi.fn(() => mql)
  window.matchMedia = matchMediaSpy as unknown as typeof window.matchMedia
})

afterEach(() => {
  unmount()
  vi.restoreAllMocks()
})

describe('Navigation mobile menu scroll lock', () => {
  it('subscribes to the exact 820px breakpoint used by index.css', () => {
    mount()
    expect(matchMediaSpy!).toHaveBeenCalledWith('(max-width: 820px)')
  })

  it('locks body scroll while the menu is open', () => {
    mount()
    expect(document.body.style.overflow).toBe('')
    act(() => toggle().click())
    expect(document.body.style.overflow).toBe('hidden')
    expect(toggle().getAttribute('aria-expanded')).toBe('true')
  })

  it('releases the lock when the toggle is clicked closed', () => {
    mount()
    act(() => toggle().click())
    act(() => toggle().click())
    expect(document.body.style.overflow).toBe('')
    expect(toggle().getAttribute('aria-expanded')).toBe('false')
  })

  it('releases the lock on Escape', () => {
    mount()
    act(() => toggle().click())
    expect(document.body.style.overflow).toBe('hidden')
    act(() => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })))
    expect(document.body.style.overflow).toBe('')
    expect(toggle().getAttribute('aria-expanded')).toBe('false')
  })

  it('releases the lock when the viewport widens past the breakpoint (the bug)', () => {
    mount()
    act(() => toggle().click())
    expect(document.body.style.overflow).toBe('hidden')
    // Desktop resize: '(max-width: 820px)' stops matching. Before the fix this
    // left body.overflow = 'hidden' with no menu visible and no `open` reset.
    act(() => mql!.fire(false))
    expect(document.body.style.overflow).toBe('')
    expect(toggle().getAttribute('aria-expanded')).toBe('false')
  })

  it('stays closed when shrinking back to mobile after a widen-while-open', () => {
    mount()
    act(() => toggle().click())
    act(() => mql!.fire(false))
    // Crossing back into mobile must not spontaneously reopen the menu.
    act(() => mql!.fire(true))
    expect(document.body.style.overflow).toBe('')
    expect(toggle().getAttribute('aria-expanded')).toBe('false')
  })

  it('removes the matchMedia listener on unmount', () => {
    mount()
    expect(mql!.listenerCount()).toBe(1)
    act(() => toggle().click())
    expect(document.body.style.overflow).toBe('hidden')
    // Unmount with the menu open runs the [open] cleanup and the breakpoint
    // listener cleanup; a later resize must be a no-op.
    unmount()
    expect(mql!.listenerCount()).toBe(0)
    expect(document.body.style.overflow).toBe('')
    expect(() => act(() => mql!.fire(false))).not.toThrow()
  })
})
