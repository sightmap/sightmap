import { useFrame } from '@react-three/fiber'
import { useEffect, useMemo, useRef } from 'react'
import * as THREE from 'three'
import { FLOORS, JOURNEYS, TRAVELLER_COLORS, type Journey } from './model'
import { buildPath, pointAt, type Path } from './geometry'
import { furnish } from './furnish'
import { floorToWorld, makeFloorPlace, placeFloor } from './floors'
import { smoothstep } from './chapters'
import { useShared, type PersonSlot } from './state'
import { PART_KEYS, RIG, gaitPose, makeGait, makePose, seatPose, standPose, stepGait, type Gait, type PartPose } from './people'

// Everybody in the building, drawn by six instanced meshes — one per body
// part. A person is six matrix writes a frame, so walkers and the scripted
// demo figures cost the same six draw calls however many of them there are.
// This replaces the old per-walker mesh tree (three meshes plus a drei Trail
// each) in Agents.tsx.
//
// One walker per journey, looping through its stops and riding the core
// between floors. Users are pink, coding agents green, tests gold. Everyone
// else is an occupant of a room — at a desk, on a sofa, behind a counter —
// placed by furnish.ts and drawn here rather than as a pair of furniture
// primitives that could never move.
const WALK = 1.75
const DWELL = 0.9
const RESET = 1.4

/**
 * Population budget. Instances are allocated for the desktop figure; the
 * per-frame count decides who fills them — walkers first, because they are
 * what the chapter is about, then the scripted vignettes, then residents.
 */
const MAX_PEOPLE = 50
const MAX_PEOPLE_MOBILE = 25

/** How many people the last frame actually drew, for Stats / __bldStats. */
export const crowd = { drawn: 0 }

/** The rig is ~1 unit tall; a person in this model is a little over 0.8. */
const WALKER_SCALE = 0.82

/** Below this a figure is a speck, so it is skipped rather than drawn. */
const MIN_SCALE = 0.02

const WALKER_SKIN = '#f3ece1'

const UP = new THREE.Vector3(0, 1, 0)
const RIGHT = new THREE.Vector3(1, 0, 0)
const ONE = new THREE.Vector3(1, 1, 1)

interface Runner {
  d: number
  dwell: number
  next: number
  wait: number
  fade: number
  /** Seconds since this leg started, for the accelerate-out-of-a-stop ease. */
  walkT: number
  /** Last heading, held through the stops where the tangent is degenerate. */
  yaw: number
}

interface Walker {
  journey: Journey
  path: Path
  run: Runner
  gait: Gait
  colors: Palette
}

/** The three colours a figure is built from. */
interface Palette {
  shirt: THREE.Color
  trousers: THREE.Color
  skin: THREE.Color
}

function palette(shirt: string, skin: string): Palette {
  const c = new THREE.Color(shirt)
  return { shirt: c, trousers: c.clone().multiplyScalar(0.55), skin: new THREE.Color(skin) }
}

/** Someone who lives in a room, in the coordinates of the floor they are on. */
interface Resident {
  floor: number
  x: number
  y: number
  z: number
  ry: number
  seated: boolean
  colors: Palette
}

/**
 * Every room occupant in the building, taken a floor at a time in rotation so
 * that a truncated budget still spreads people up the whole tower instead of
 * crowding them onto the lowest floors.
 */
function residents(): Resident[] {
  const byFloor = FLOORS.map((f, floor) =>
    f.rooms.flatMap((room) =>
      furnish(room).people.map((p) => ({
        floor,
        x: p.x,
        y: p.y,
        z: p.z,
        ry: p.ry,
        seated: p.seated,
        colors: palette(p.shirt, p.skin),
      }))
    )
  )
  const out: Resident[] = []
  const deepest = Math.max(...byFloor.map((list) => list.length))
  for (let i = 0; i < deepest; i++) {
    for (const list of byFloor) if (i < list.length) out.push(list[i])
  }
  return out
}

/** Slot colours are fixed per vignette but not known until one mounts. */
const SLOT_PALETTES = new Map<string, Palette>()

function slotPalette(color: string): Palette {
  let p = SLOT_PALETTES.get(color)
  if (!p) {
    p = palette(color, WALKER_SKIN)
    SLOT_PALETTES.set(color, p)
  }
  return p
}

export default function People() {
  const s = useShared()
  const meshes = useRef<(THREE.InstancedMesh | null)[]>([])

  const walkers = useMemo<Walker[]>(
    () =>
      JOURNEYS.map((journey, i) => ({
        journey,
        path: buildPath(journey),
        run: { d: 0, dwell: DWELL, next: 1, wait: journey.delay, fade: 0, walkT: 0, yaw: 0 },
        // Stagger the starting phases so the crowd does not march in lockstep.
        gait: makeGait(i * 1.7),
        colors: palette(TRAVELLER_COLORS[journey.who], WALKER_SKIN),
      })),
    []
  )

  const rig = useMemo(() => {
    const torso = new THREE.CapsuleGeometry(RIG.torsoR, RIG.torsoLen, 3, 8)
    torso.scale(1, 1, RIG.torsoFlat)
    const head = new THREE.SphereGeometry(RIG.headR, 9, 7)
    const arm = new THREE.CapsuleGeometry(RIG.armR, RIG.armLen, 2, 6)
    const leg = new THREE.CapsuleGeometry(RIG.legR, RIG.legLen, 2, 6)
    const material = new THREE.MeshStandardMaterial({ color: '#ffffff', roughness: 0.78 })
    // Index order matches PART_KEYS: torso, head, armL, armR, legL, legR. The
    // two arms and the two legs share a geometry; only the matrices differ.
    return { parts: [torso, head, arm, arm, leg, leg], own: [torso, head, arm, leg], material }
  }, [])
  useEffect(
    () => () => {
      for (const g of rig.own) g.dispose()
      rig.material.dispose()
    },
    [rig]
  )

  // Slots come and go with their vignettes, so their gait state is keyed off
  // the slot itself rather than an index that would drift.
  const slotGaits = useMemo(() => new WeakMap<PersonSlot, Gait>(), [])

  const locals = useMemo(residents, [])
  const places = useMemo(() => FLOORS.map(() => makeFloorPlace()), [])

  const k = useMemo(
    () => ({
      pose: makePose(),
      here: new THREE.Vector3(),
      ahead: new THREE.Vector3(),
      pos: new THREE.Vector3(),
      off: new THREE.Vector3(),
      size: new THREE.Vector3(),
      body: new THREE.Quaternion(),
      limb: new THREE.Quaternion(),
      base: new THREE.Matrix4(),
      local: new THREE.Matrix4(),
      world: new THREE.Matrix4(),
    }),
    []
  )

  /** Write one person's six part matrices and colours at instance `i`. */
  function draw(i: number, x: number, y: number, z: number, ry: number, scale: number, pose: PartPose[], colors: Palette) {
    k.base.compose(k.pos.set(x, y, z), k.body.setFromAxisAngle(UP, ry), k.size.setScalar(scale))
    for (let p = 0; p < PART_KEYS.length; p++) {
      const mesh = meshes.current[p]
      if (!mesh) continue
      const part = pose[p]
      k.local.compose(k.off.set(part.x, part.y, part.z), k.limb.setFromAxisAngle(RIGHT, part.rx), ONE)
      k.world.multiplyMatrices(k.base, k.local)
      mesh.setMatrixAt(i, k.world)
      const key = PART_KEYS[p]
      const c = key === 'head' ? colors.skin : key === 'legL' || key === 'legR' ? colors.trousers : colors.shirt
      mesh.setColorAt(i, c)
    }
  }

  /** Advance one walker's journey and hand back the scale it should draw at. */
  function advance(w: Walker, dt: number, rise: number): number {
    const r = w.run
    if (s.reduced) {
      r.fade = 1
    } else if (r.wait > 0) {
      r.wait -= dt
      r.fade = Math.max(0, r.fade - dt * 2)
    } else if (r.dwell > 0) {
      r.dwell -= dt
      r.fade = Math.min(1, r.fade + dt * 2.5)
    } else if (r.next < w.path.stops.length) {
      // Ease speed up from zero rather than snapping to WALK the instant the
      // dwell ends, so a walker accelerates out of a stop.
      r.walkT += dt
      const target = w.path.cum[w.path.stops[r.next]]
      r.d = Math.min(target, r.d + WALK * smoothstep(r.walkT / DWELL) * dt)
      if (r.d >= target - 1e-4) {
        r.next += 1
        r.dwell = DWELL
        r.walkT = 0
      }
    } else {
      // Journey complete: fade out, then start it again.
      r.fade = Math.max(0, r.fade - dt * 2.5)
      if (r.fade <= 0) {
        r.d = 0
        r.next = 1
        r.dwell = DWELL
        r.wait = RESET
        r.walkT = 0
      }
    }
    const focus = s.focus && s.focus !== w.journey.name ? 0.3 : 1
    return s.cur.agents * focus * smoothstep(r.fade) * rise * WALKER_SCALE
  }

  useFrame((_, delta) => {
    const dt = Math.min(delta, 0.2)
    const rise = smoothstep(Math.min(1, s.cur.rise * 1.3 - 0.3))
    const cap = s.mobile ? MAX_PEOPLE_MOBILE : MAX_PEOPLE
    const walking = s.mobile ? 5 : walkers.length
    let n = 0

    for (let i = 0; i < walking && n < cap; i++) {
      const w = walkers[i]
      const scale = advance(w, dt, rise)
      if (scale <= MIN_SCALE) continue
      pointAt(w.path, w.run.d, k.here)
      // Heading follows the path tangent: sample a little further along the
      // (Catmull-Rom-rounded) path and face that.
      pointAt(w.path, w.run.d + 0.05, k.ahead)
      if (k.ahead.distanceToSquared(k.here) > 1e-8) {
        w.run.yaw = Math.atan2(k.ahead.x - k.here.x, k.ahead.z - k.here.z)
      }
      stepGait(w.gait, k.here.x, k.here.z, dt, s.reduced)
      gaitPose(w.gait.phase, w.gait.amp, k.pose)
      draw(n, k.here.x, k.here.y, k.here.z, w.run.yaw, scale, k.pose, w.colors)
      n++
    }

    // Figures a vignette drives itself (the front desk, the self-healing test).
    for (const slot of s.slots) {
      if (n >= cap) break
      if (!slot.visible || slot.scale <= MIN_SCALE) continue
      let gait = slotGaits.get(slot)
      if (!gait) {
        gait = makeGait()
        slotGaits.set(slot, gait)
      }
      stepGait(gait, slot.x, slot.z, dt, s.reduced)
      gaitPose(gait.phase, gait.amp, k.pose)
      draw(n, slot.x, slot.y, slot.z, slot.ry, slot.scale * WALKER_SCALE, k.pose, slotPalette(slot.color))
      n++
    }

    // The people who work here. They ride their floor up out of the fanned
    // sheets, so each one is placed through the same transform Tower gives
    // that floor, and grows in with the room around them.
    for (let i = 0; i < places.length; i++)
      placeFloor(i, s.cur.rise, s.cur.spread, s.cur.walls, places[i])
    for (const r of locals) {
      if (n >= cap) break
      const place = places[r.floor]
      const scale = place.fill * WALKER_SCALE
      if (scale <= MIN_SCALE) continue
      floorToWorld(place, r.x, r.y, r.z, k.here)
      // Seated or standing, both at rest: same six meshes, gait amplitude 0.
      if (r.seated) seatPose(k.pose)
      else standPose(k.pose)
      draw(n, k.here.x, k.here.y, k.here.z, place.ry + r.ry, scale, k.pose, r.colors)
      n++
    }

    crowd.drawn = n

    for (const mesh of meshes.current) {
      if (!mesh) continue
      mesh.count = n
      mesh.instanceMatrix.needsUpdate = true
      if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true
    }
  })

  return (
    <group>
      {PART_KEYS.map((key, i) => (
        <instancedMesh
          key={key}
          ref={(m) => {
            meshes.current[i] = m
          }}
          args={[rig.parts[i], rig.material, MAX_PEOPLE]}
          count={0}
          castShadow
          frustumCulled={false}
        />
      ))}
    </group>
  )
}
