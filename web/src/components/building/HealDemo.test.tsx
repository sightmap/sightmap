// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act } from 'react'
import * as React from 'react'
import { createRoot } from 'react-dom/client'
import * as THREE from 'three'
import HealDemo from './HealDemo'
import { SharedStateContext, createSharedState } from './state'
import { defaultParams } from './chapters'
import { findRoom, roomStand, surfaceAt } from './model'

// `act` requires the host env to declare itself; silence the act warning.
;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

// Mock @react-three/fiber's useFrame to capture the frame callback (so the test
// can drive it with a fake clock and inspect the real THREE objects the
// callback mutates). The JSX intrinsics (<mesh>, <boxGeometry>, …) fall back
// to inert custom DOM elements in jsdom; refs are re-pointed at real THREE
// objects after render, so the captured callback exercises the genuine body.
const frame = vi.hoisted(() => ({ cb: null as ((args: { clock: { getElapsedTime: () => number } }) => void) | null }))
vi.mock('@react-three/fiber', () => ({
  useFrame: (cb: (args: { clock: { getElapsedTime: () => number } }) => void) => {
    frame.cb = cb
  },
}))
vi.mock('@react-three/drei', () => ({
  Html: ({ children }: { children: React.ReactNode }) => React.createElement('div', null, children),
}))
vi.mock('./Agents', () => ({
  Walker: () => React.createElement('div', { 'data-testid': 'walker-stub' }),
}))

// Override React's `useRef` so the component's refs (walker, ghost, status,
// start, lastPhase — in that call order) resolve to ref objects the test owns,
// which it can then re-point at real THREE objects / a real DOM div. Everything
// else (useMemo, createElement, act, …) is passed through from real react, so
// react-dom/client rendering still works.
const queue = vi.hoisted(() => ({ refs: [] as { current: unknown }[] }))
vi.mock('react', async (importOriginal) => {
  const actual = await importOriginal<typeof React>()
  return {
    ...actual,
    useRef: (init?: unknown) => (queue.refs.length ? (queue.refs.shift() as { current: unknown }) : { current: init }),
  }
})

beforeEach(() => {
  queue.refs.length = 0
})

const FLOOR = 3
const PERIOD = 8.4

function setup(opts: { reduced: boolean; heal: number }) {
  const walkerRef = { current: null as THREE.Group | null }
  const ghostRef = { current: null as THREE.Mesh | null }
  const statusRef = { current: null as HTMLDivElement | null }
  const startRef = { current: null as number | null }
  const lastPhaseRef = { current: 'reset' as string }
  queue.refs.push(walkerRef, ghostRef, statusRef, startRef, lastPhaseRef)

  const shared = createSharedState()
  shared.reduced = opts.reduced
  shared.cur = defaultParams()
  shared.cur.heal = opts.heal

  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  act(() => {
    root.render(
      React.createElement(SharedStateContext.Provider, { value: shared }, React.createElement(HealDemo))
    )
  })

  // Point the component's captured refs at real THREE objects / a real DOM
  // div so the frame callback's body runs against the real types it expects.
  walkerRef.current = new THREE.Group()
  ghostRef.current = new THREE.Mesh(new THREE.BoxGeometry(1, 1, 1), new THREE.MeshStandardMaterial())
  statusRef.current = document.createElement('div')
  document.body.appendChild(statusRef.current)

  const now = { value: 0 }
  const clock = { getElapsedTime: () => now.value }
  const fire = (t: number) => {
    now.value = t
    if (frame.cb) frame.cb({ clock })
  }

  const teardown = () => {
    act(() => root.unmount())
    container.remove()
    if (statusRef.current) statusRef.current.remove()
  }

  return { shared, walkerRef, ghostRef, statusRef, startRef, lastPhaseRef, fire, teardown }
}

describe('HealDemo · reduced-motion anchoring', () => {
  it('anchors the walker to newPos with no bob across every loop phase (G1, G2)', () => {
    const { walkerRef, fire, teardown } = setup({ reduced: true, heal: 1 })
    const newPosArr = roomStand(FLOOR, findRoom(FLOOR, 'ContinueButton'), 1)
    const newPos = new THREE.Vector3(...newPosArr)
    // Sample every phase the non-reduced loop would visit, plus a second
    // period, to prove the wall-clock loop is dead under reduced-motion.
    const times = [0, 0.5, 1.0, 1.5, 2.6, 3.0, 3.9, 4.0, 4.7, 5.5, 6.0, 7.0, 7.8, 8.3, PERIOD + 0.5, PERIOD + 4.0]
    for (const t of times) {
      fire(t)
      const p = walkerRef.current!.position
      expect(p.x, `t=${t} x`).toBeCloseTo(newPos.x, 6)
      expect(p.y, `t=${t} y (no bob)`).toBeCloseTo(newPos.y, 6)
      expect(p.z, `t=${t} z`).toBeCloseTo(newPos.z, 6)
    }
    teardown()
  })

  it('hides the ghost and resets its color to white, even at the would-be fail phase (G3)', () => {
    const { ghostRef, fire, teardown } = setup({ reduced: true, heal: 1 })
    const failTimes = [3.0, 3.5, 3.9, 4.0] // would be 'fail' phase in the live loop
    for (const t of failTimes) {
      fire(t)
      const m = ghostRef.current!.material as THREE.MeshStandardMaterial
      expect(ghostRef.current!.visible, `t=${t} visible`).toBe(false)
      expect(m.opacity, `t=${t} opacity`).toBe(0)
      expect(m.color.getHexString(), `t=${t} color`).toBe('ffffff')
      expect(m.emissive.getHexString(), `t=${t} emissive`).toBe('ffffff')
    }
    teardown()
  })

  it('pins the status HUD to the pass line and does not re-cycle it (G4)', () => {
    const { statusRef, lastPhaseRef, fire, teardown } = setup({ reduced: true, heal: 1 })
    fire(0)
    expect(statusRef.current!.textContent).toBe('✓ ContinueButton clicked · run passed')
    expect(statusRef.current!.className).toContain('bld-status--pass')
    expect(lastPhaseRef.current).toBe('pass')
    // Advance through phases that, in the live loop, would change the text.
    const before = statusRef.current!.textContent
    for (const t of [1.0, 2.6, 3.0, 4.7, 6.0, 7.0, 8.3]) {
      fire(t)
      expect(statusRef.current!.textContent, `t=${t}`).toBe(before)
      expect(statusRef.current!.className).toContain('bld-status--pass')
    }
    teardown()
  })

  it('sets healShift = 1 and never starts the wall-clock accumulator (G5, G6, G1)', () => {
    const { shared, startRef, fire, teardown } = setup({ reduced: true, heal: 1 })
    for (const t of [0, 2.6, 4.0, 7.0, 8.3, PERIOD + 2]) {
      fire(t)
      expect(shared.healShift, `t=${t}`).toBe(1)
      expect(startRef.current, `t=${t} start`).toBeNull()
    }
    teardown()
  })

  it('fades the walker and HUD with cur.heal in the reduced branch (G7, G8)', () => {
    const { walkerRef, statusRef, fire, teardown } = setup({ reduced: true, heal: 0.6 })
    // heal = 0.6 keeps `active` (heal > 0.5) true, so the reduced branch runs.
    fire(0)
    expect(walkerRef.current!.scale.x).toBeCloseTo(0.6, 6)
    expect(walkerRef.current!.visible).toBe(true)
    // jsdom re-serializes CSS opacity (trims trailing zeros), so compare by value.
    expect(Number(statusRef.current!.style.opacity)).toBeCloseTo(0.6, 6)
    expect(statusRef.current!.style.visibility).toBe('visible')
    teardown()
  })

  it('falls back to the !active fade path (not the reduced branch) for heal <= 0.5 and pins at PaymentForm (G9, G10)', () => {
    const from = new THREE.Vector3(...roomStand(FLOOR, findRoom(FLOOR, 'PaymentForm')))
    const newPos = new THREE.Vector3(...roomStand(FLOOR, findRoom(FLOOR, 'ContinueButton'), 1))

    const { shared, walkerRef, ghostRef, statusRef, fire, teardown } = setup({ reduced: true, heal: 0.3 })
    shared.healShift = 0 // start the !active damp from a known value
    fire(0)
    // Walker pins at `from` (PaymentForm), not at newPos — no frozen mid-scene.
    expect(walkerRef.current!.position.x).toBeCloseTo(from.x, 6)
    expect(walkerRef.current!.position.z).toBeCloseTo(from.z, 6)
    expect(Math.abs(walkerRef.current!.position.y - from.y)).toBeLessThan(1e-6) // no bob
    expect([walkerRef.current!.position.x, walkerRef.current!.position.z]).not.toEqual([newPos.x, newPos.z])
    // Ghost hidden and white.
    const m = ghostRef.current!.material as THREE.MeshStandardMaterial
    expect(ghostRef.current!.visible).toBe(false)
    expect(m.opacity).toBe(0)
    expect(m.color.getHexString()).toBe('ffffff')
    // Status HUD dark/hidden.
    expect(Number(statusRef.current!.style.opacity)).toBe(0)
    expect(statusRef.current!.style.visibility).toBe('hidden')
    // healShift damps toward 0 (the !active branch), NOT pinned to 1.
    expect(shared.healShift).toBeLessThan(0.1)
    teardown()
  })

  it('runs the full loop for non-reduced visitors: motion, ghost flash, status cycling (G11)', () => {
    const { walkerRef, ghostRef, statusRef, fire, teardown } = setup({ reduced: false, heal: 1 })
    const from = new THREE.Vector3(...roomStand(FLOOR, findRoom(FLOOR, 'PaymentForm')))
    const newPos = new THREE.Vector3(...roomStand(FLOOR, findRoom(FLOOR, 'ContinueButton'), 1))

    // Prime the wall-clock accumulator at 0 so loop time `t = now - start = now`.
    fire(0)

    // idle: walker at `from`, no bob.
    fire(0.5)
    expect(walkerRef.current!.position.x).toBeCloseTo(from.x, 6)
    expect(walkerRef.current!.position.z).toBeCloseTo(from.z, 6)

    // walk phase (t_loop = 1.5): walker has moved off `from` and is bobbing
    // (position.y above the floor surface at its current xz by the bob amount).
    fire(1.5)
    const p = walkerRef.current!.position
    expect(Math.hypot(p.x - from.x, p.z - from.z)).toBeGreaterThan(1e-3)
    const surfaceY = surfaceAt(FLOOR, p.x, p.z)
    const bob = p.y - surfaceY
    // sin(1.5*14) is large, so bob ≈ 0.042 — clearly non-zero.
    expect(bob).toBeGreaterThan(0.02)

    // fail phase (t_loop = 3.5): walker at oldPos, ghost visible and red.
    fire(3.5)
    const m = ghostRef.current!.material as THREE.MeshStandardMaterial
    expect(ghostRef.current!.visible).toBe(true)
    expect(m.color.getHexString()).toBe('e04d6a')
    expect(statusRef.current!.textContent).toContain('not found')

    // pass phase (t_loop = 7.5): walker at newPos.
    fire(7.5)
    expect(walkerRef.current!.position.x).toBeCloseTo(newPos.x, 5)
    expect(walkerRef.current!.position.z).toBeCloseTo(newPos.z, 5)
    expect(statusRef.current!.className).toContain('bld-status--pass')

    // Status text cycled through several distinct lines over the loop.
    const texts = new Set<string>()
    for (const t of [0.5, 2.0, 3.5, 4.5, 5.5, 7.5]) {
      fire(t)
      texts.add(statusRef.current!.textContent ?? '')
    }
    expect(texts.size).toBeGreaterThanOrEqual(4)
    teardown()
  })
})

