import { useFrame } from '@react-three/fiber'
import { Html, Instance, Instances, Line, RoundedBox } from '@react-three/drei'
import { useMemo, useRef, type ComponentRef } from 'react'
import * as THREE from 'three'
import {
  FLOOR_D,
  FLOOR_W,
  KIND_COLORS,
  KIOSK_H,
  PLATE,
  SHEET,
  SLAB_T,
  WALL_T,
  floorHeight,
  floorY,
  type BuildingModel,
  type Kind,
  type Room,
} from './model'
import { furnish, type Item, type ItemType } from './furnish'
import { personaFurnish } from './persona'
import { paletteColor, roofParts } from './facade'
import RoofShape from './RoofShape'
import RoomLabels from './RoomLabels'
import { sheetLinePoints } from './geometry'
import { smoothstep } from './chapters'
import { useBuildingModel } from './context'
import { useShared } from './state'
import Shell from './Shell'

// Each floor starts life as a blueprint sheet lying on the table. As `rise`
// climbs, the sheet lifts to its floor height, fades into a slab, and the
// floor's zones, furniture, glass and people grow out of the drawing. The
// curtain walls, roof garden and structural frame follow.

type LineRef = ComponentRef<typeof Line>

const STEEL = '#26272c'

/** Where sheet i lies when fanned across a table of n sheets. */
function fan(i: number, n: number): { x: number; z: number; r: number } {
  const k = i - (n - 1) / 2
  return { x: k * 1.05 - 0.3, z: -k * 0.85 + 0.3, r: k * 0.09 }
}

const stagger = (rise: number, i: number): number => smoothstep(THREE.MathUtils.clamp(rise * 1.6 - i * 0.11, 0, 1))

// ---------------------------------------------------------------------------
// Shared materials, retinted at nightfall.

interface Mats {
  kinds: Record<Kind, THREE.MeshStandardMaterial>
  slab: THREE.MeshStandardMaterial
  steel: THREE.MeshStandardMaterial
  glass: THREE.MeshStandardMaterial
  spandrel: THREE.MeshStandardMaterial
  furniture: Record<ItemType, THREE.MeshStandardMaterial>
}

function useMaterials(): Mats {
  const s = useShared()
  const mats = useMemo<Mats>(() => {
    const kinds = {} as Record<Kind, THREE.MeshStandardMaterial>
    for (const k of Object.keys(KIND_COLORS) as Kind[]) {
      kinds[k] = new THREE.MeshStandardMaterial({
        color: KIND_COLORS[k],
        roughness: 0.92,
        emissive: new THREE.Color(KIND_COLORS[k]),
        emissiveIntensity: 0,
      })
    }
    const plain = (opts: THREE.MeshStandardMaterialParameters = {}) =>
      new THREE.MeshStandardMaterial({ color: '#ffffff', roughness: 0.85, ...opts })
    const furniture: Record<ItemType, THREE.MeshStandardMaterial> = {
      desk: plain({ roughness: 0.7 }),
      monitor: plain({ roughness: 0.4, emissive: new THREE.Color('#bcd4ff'), emissiveIntensity: 0.25 }),
      chair: plain(),
      sofa: plain(),
      table: plain({ roughness: 0.6 }),
      pot: plain(),
      leaf: plain({ roughness: 1 }),
      shelf: plain(),
      book: plain(),
      screen: plain({ roughness: 0.3, emissive: new THREE.Color('#9fc2ff'), emissiveIntensity: 0.3 }),
      body: plain(),
      head: plain(),
      partition: plain({ transparent: true, opacity: 0.22, roughness: 0.1, depthWrite: false }),
      rail: plain({ roughness: 0.5 }),
      counter: plain({ roughness: 0.6 }),
      awning: plain({ roughness: 0.8 }),
      column: plain({ roughness: 0.85 }),
      crate: plain({ roughness: 0.95 }),
      bed: plain({ roughness: 0.8 }),
    }
    return {
      kinds,
      slab: new THREE.MeshStandardMaterial({
        color: '#f2ece3',
        roughness: 0.92,
        emissive: new THREE.Color('#ffc36a'),
        emissiveIntensity: 0,
      }),
      steel: new THREE.MeshStandardMaterial({ color: STEEL, roughness: 0.55, metalness: 0.2 }),
      glass: new THREE.MeshStandardMaterial({
        color: '#cfe3f5',
        transparent: true,
        opacity: 0.22,
        roughness: 0.1,
        metalness: 0.1,
        depthWrite: false,
        emissive: new THREE.Color('#ffc36a'),
        emissiveIntensity: 0,
        side: THREE.DoubleSide,
      }),
      spandrel: new THREE.MeshStandardMaterial({ color: '#e9e2d6', roughness: 0.95 }),
      furniture,
    }
  }, [])
  const col = useMemo(
    () => ({
      slabDay: new THREE.Color('#f2ece3'),
      slabNight: new THREE.Color('#7b86ad'),
      spDay: new THREE.Color('#e9e2d6'),
      spNight: new THREE.Color('#5d6a94'),
      glassDay: new THREE.Color('#cfe3f5'),
      glassNight: new THREE.Color('#ffd58a'),
    }),
    []
  )
  useFrame(() => {
    const n = s.cur.night
    mats.slab.color.copy(col.slabDay).lerp(col.slabNight, n)
    mats.spandrel.color.copy(col.spDay).lerp(col.spNight, n)
    mats.glass.color.copy(col.glassDay).lerp(col.glassNight, n)
    mats.glass.emissiveIntensity = n * 0.9
    mats.glass.opacity = THREE.MathUtils.lerp(0.22, 0.42, n)
    mats.slab.emissiveIntensity = n * 0.14
    mats.furniture.monitor.emissiveIntensity = THREE.MathUtils.lerp(0.25, 1.55, n)
    mats.furniture.screen.emissiveIntensity = THREE.MathUtils.lerp(0.3, 1.45, n)
    for (const k of Object.keys(mats.kinds) as Kind[]) mats.kinds[k].emissiveIntensity = n * 0.5
  })
  return mats
}

// ---------------------------------------------------------------------------
// Furniture: one instanced mesh per item type per floor.

const unitBox = new THREE.BoxGeometry(1, 1, 1)
const unitCyl = new THREE.CylinderGeometry(0.5, 0.5, 1, 14)
const unitSphere = new THREE.SphereGeometry(0.5, 12, 10)
const unitCapsule = new THREE.CapsuleGeometry(0.5, 0.6, 4, 10)
const GEOM: Record<ItemType, THREE.BufferGeometry> = {
  desk: unitBox,
  monitor: unitBox,
  chair: unitBox,
  sofa: unitBox,
  table: unitCyl,
  pot: unitCyl,
  leaf: unitSphere,
  shelf: unitBox,
  book: unitBox,
  screen: unitBox,
  body: unitCapsule,
  head: unitSphere,
  partition: unitBox,
  rail: unitBox,
  counter: unitBox,
  awning: unitBox,
  column: unitCyl,
  crate: unitBox,
  bed: unitBox,
}
const NO_SHADOW: ItemType[] = ['partition', 'leaf', 'book']

function Furniture({ items, mats }: { items: Item[]; mats: Mats }) {
  const groups = useMemo(() => {
    const by = new Map<ItemType, Item[]>()
    for (const it of items) {
      const list = by.get(it.type)
      if (list) list.push(it)
      else by.set(it.type, [it])
    }
    return [...by.entries()]
  }, [items])
  return (
    <>
      {groups.map(([type, list]) => (
        <Instances
          key={type}
          limit={list.length}
          range={list.length}
          geometry={GEOM[type]}
          material={mats.furniture[type]}
          castShadow={!NO_SHADOW.includes(type)}
          receiveShadow
          frustumCulled={false}
        >
          {list.map((it, k) => (
            <Instance
              key={k}
              position={[it.x, it.y, it.z]}
              rotation={[it.rx ?? 0, it.ry, it.rz ?? 0]}
              scale={[it.sx, it.sy, it.sz]}
              color={it.color}
            />
          ))}
        </Instances>
      ))}
    </>
  )
}

// ---------------------------------------------------------------------------
// Zones and kiosks.

function Kiosk({ room, mats }: { room: Room; mats: Mats }) {
  const c = KIND_COLORS[room.kind]
  return (
    <group position={[0, PLATE, 0]}>
      <RoundedBox args={[0.55, KIOSK_H - 0.1, 0.55]} radius={0.04} position={[0, (KIOSK_H - 0.1) / 2, 0]} castShadow receiveShadow>
        <primitive object={mats.kinds[room.kind]} attach="material" />
      </RoundedBox>
      <mesh position={[0, KIOSK_H - 0.05, 0]}>
        <boxGeometry args={[0.62, 0.1, 0.62]} />
        <meshStandardMaterial color="#fbf8f2" emissive={c} emissiveIntensity={0.6} roughness={0.4} />
      </mesh>
      <mesh position={[0, KIOSK_H / 2, 0.29]}>
        <planeGeometry args={[0.36, 0.5]} />
        <meshStandardMaterial color="#fbf8f2" emissive="#ffffff" emissiveIntensity={0.35} roughness={0.3} />
      </mesh>
    </group>
  )
}

function Zone({ room, mats }: { room: Room; mats: Mats }) {
  const s = useShared()
  const g = useRef<THREE.Group>(null)
  const blocks = room.blocks ?? [{ x: room.x, z: room.z, w: room.w, d: room.d }]
  const lift = room.base ? PLATE : 0
  useFrame(() => {
    if (!room.alt || !g.current) return
    const t = smoothstep(s.healShift)
    g.current.position.set((room.alt.x - room.x) * t, 0, (room.alt.z - room.z) * t)
  })
  return (
    <group ref={g}>
      {blocks.map((b, k) => (
        <RoundedBox
          key={k}
          args={[b.w, PLATE, b.d]}
          radius={0.03}
          smoothness={2}
          position={[b.x, lift + PLATE / 2, b.z]}
          receiveShadow
        >
          <primitive object={mats.kinds[room.kind]} attach="material" />
        </RoundedBox>
      ))}
      {room.kind === 'action' && (
        <group position={[room.x, lift, room.z]}>
          <Kiosk room={room} mats={mats} />
        </group>
      )}
    </group>
  )
}

// ---------------------------------------------------------------------------
// Curtain walls on the two back sides: spandrel, glass, mullions, head beam.

function CurtainWall({ mats, side, wallH }: { mats: Mats; side: 'x' | 'z'; wallH: number }) {
  const len = side === 'x' ? FLOOR_D : FLOOR_W
  const n = Math.round(len / 1.25)
  const mullions = useMemo(() => Array.from({ length: n + 1 }, (_, k) => -len / 2 + (k * len) / n), [len, n])
  const at = (u: number, y: number): [number, number, number] =>
    side === 'x' ? [-FLOOR_W / 2 + WALL_T / 2, y, u] : [u, y, -FLOOR_D / 2 + WALL_T / 2]
  const size = (u: number, y: number, t: number): [number, number, number] => (side === 'x' ? [t, y, u] : [u, y, t])
  return (
    <group>
      <mesh position={at(0, 0.16)} castShadow receiveShadow>
        <boxGeometry args={size(len, 0.32, WALL_T)} />
        <primitive object={mats.spandrel} attach="material" />
      </mesh>
      <mesh position={at(0, 0.32 + (wallH - 0.42) / 2)}>
        <boxGeometry args={size(len, wallH - 0.42, 0.02)} />
        <primitive object={mats.glass} attach="material" />
      </mesh>
      <mesh position={at(0, wallH - 0.05)} castShadow>
        <boxGeometry args={size(len, 0.1, WALL_T + 0.02)} />
        <primitive object={mats.steel} attach="material" />
      </mesh>
      <Instances limit={mullions.length} range={mullions.length} geometry={unitBox} material={mats.steel} castShadow>
        {mullions.map((u) => (
          <Instance key={u} position={at(u, wallH / 2)} scale={size(0.07, wallH, WALL_T + 0.02)} />
        ))}
      </Instances>
    </group>
  )
}

// ---------------------------------------------------------------------------

function FloorUnit({ model, index: i, mats, t0 }: { model: BuildingModel; index: number; mats: Mats; t0: number }) {
  const s = useShared()
  const floor = model.floors[i]
  const g = useRef<THREE.Group>(null)
  const sheet = useRef<THREE.Mesh>(null)
  const sheetMat = useRef<THREE.MeshStandardMaterial>(null)
  const lines = useRef<LineRef>(null)
  const slab = useRef<THREE.Group>(null)
  const rooms = useRef<THREE.Group>(null)
  const walls = useRef<THREE.Group>(null)
  const pts = useMemo(() => sheetLinePoints(model, i), [model, i])
  const items = useMemo(() => {
    const out = floor.rooms.flatMap((r) => furnish(r))
    // Only a building derived from a scan wears a persona; the demo corpus is
    // the office it has always been.
    if (model.derived && model.facade) {
      out.push(
        ...personaFurnish(
          model.facade.archetype,
          model.facade.variant,
          floor,
          i,
          `${model.seed}`,
          floorHeight(model) - SLAB_T
        )
      )
    }
    return out
  }, [floor, i, model])
  // LineSegments2 accumulates dash distance across every segment, so one
  // growing dash draws the sheet in sequence: border, footprint, then rooms.
  const total = useMemo(() => {
    let l = 0
    for (let k = 0; k < pts.length; k += 2) {
      const a = pts[k]
      const b = pts[k + 1]
      l += Math.hypot(a[0] - b[0], a[1] - b[1], a[2] - b[2])
    }
    return l
  }, [pts])
  const pose = fan(i, model.floors.length)

  useFrame(() => {
    const c = s.cur
    const rise = stagger(c.rise, i)
    const flat = 1 - rise
    const y0 = 0.012 + i * 0.014
    if (g.current) {
      g.current.position.set(
        pose.x * c.spread * flat,
        THREE.MathUtils.lerp(y0, floorY(i, floorHeight(model)), rise),
        pose.z * c.spread * flat
      )
      g.current.rotation.y = pose.r * c.spread * flat
    }
    // The line work sketches itself in once, on load, then fades as the
    // sheet becomes a slab.
    const drawT = THREE.MathUtils.clamp((performance.now() - t0 - i * 260) / 3200, 0, 1)
    const sheetA = 1 - smoothstep(rise * 1.7)
    if (sheetMat.current) {
      sheetMat.current.opacity = sheetA
      sheetMat.current.color.set(c.night > 0.5 ? '#1d4a8f' : '#2a5fb3')
    }
    if (sheet.current) sheet.current.visible = sheetA > 0.01
    if (lines.current) {
      const m = lines.current.material as THREE.ShaderMaterial & { dashSize: number; gapSize: number; opacity: number }
      m.dashSize = 0.001 + drawT * total
      m.gapSize = 100000
      const a = c.draw * (1 - smoothstep((rise - 0.3) / 0.45)) * 0.92
      m.opacity = a
      lines.current.visible = a > 0.01
    }
    if (slab.current) {
      slab.current.visible = rise > 0.02
      slab.current.scale.set(1, Math.max(rise, 0.001), 1)
    }
    if (rooms.current) {
      const sy = smoothstep((rise - 0.35) / 0.65)
      rooms.current.scale.y = Math.max(sy, 0.001)
      rooms.current.visible = sy > 0.005
    }
    if (walls.current) {
      const w = c.walls * smoothstep((rise - 0.55) / 0.45)
      walls.current.scale.y = Math.max(w, 0.001)
      walls.current.visible = w > 0.005
    }
  })

  return (
    <group ref={g}>
      <mesh ref={sheet} rotation-x={-Math.PI / 2} receiveShadow>
        <planeGeometry args={[SHEET.w, SHEET.d]} />
        <meshStandardMaterial ref={sheetMat} color="#2a5fb3" roughness={1} transparent />
      </mesh>
      <Line
        ref={lines}
        points={pts}
        segments
        color="#ffffff"
        lineWidth={1.4}
        dashed
        dashSize={0.001}
        gapSize={100000}
        transparent
        opacity={0.9}
        depthWrite={false}
      />
      {/* Slab: dark steel edge with a cream deck on top. */}
      <group ref={slab}>
        <mesh position={[0, SLAB_T / 2 - 0.02, 0]} castShadow receiveShadow>
          <boxGeometry args={[FLOOR_W + 0.06, SLAB_T - 0.04, FLOOR_D + 0.06]} />
          <primitive object={mats.steel} attach="material" />
        </mesh>
        <mesh position={[0, SLAB_T - 0.02, 0]} receiveShadow>
          <boxGeometry args={[FLOOR_W, 0.04, FLOOR_D]} />
          <primitive object={mats.slab} attach="material" />
        </mesh>
      </group>
      <group ref={rooms} position={[0, SLAB_T, 0]}>
        {floor.rooms.map((r) => (
          <Zone key={r.name} room={r} mats={mats} />
        ))}
        <Furniture items={items} mats={mats} />
      </group>
      <group ref={walls} position={[0, SLAB_T, 0]}>
        <CurtainWall mats={mats} side="x" wallH={floorHeight(model) - SLAB_T} />
        <CurtainWall mats={mats} side="z" wallH={floorHeight(model) - SLAB_T} />
      </group>
    </group>
  )
}

// ---------------------------------------------------------------------------
// Roof: garden beds, deck, lounge, solar array, and the structural frame.

/**
 * The sign the building wears at the top of its street face, with the plate
 * under it. Only a derived building has one: the demo corpus is not a
 * listing and has no name to put up.
 */
function Sign({ model, y }: { model: BuildingModel; y: number }) {
  const sign = model.facade?.sign
  if (!sign) return null
  const tools = model.tools ?? model.floors.reduce((n, f) => n + f.rooms.length, 0)
  const w = THREE.MathUtils.clamp(1.6 + sign.length * 0.42, 3.0, FLOOR_W * 0.78)
  return (
    <group position={[0, y, FLOOR_D / 2 + 0.45]}>
      <mesh position={[0, 1.32, 0]} castShadow>
        <boxGeometry args={[w, 0.62, 0.09]} />
        <meshStandardMaterial color="#26272c" roughness={0.7} />
      </mesh>
      {/* Two stays back to the parapet, so the board reads as mounted on the
          building rather than floating in front of it. */}
      {[-1, 1].map((side) => (
        <mesh key={side} position={[(side * (w - 0.3)) / 2, 0.62, -0.2]} rotation={[0.32, 0, 0]} castShadow>
          <boxGeometry args={[0.07, 1.5, 0.07]} />
          <meshStandardMaterial color="#26272c" roughness={0.6} />
        </mesh>
      ))}
      <Html position={[0, 1.32, 0.07]} center zIndexRange={[7, 0]} style={{ pointerEvents: 'none' }} wrapperClass="bld-sign-anchor">
        <div className="bld-sign bld-sign--board" aria-hidden="true">
          {sign}
        </div>
      </Html>
      <mesh position={[0, 0.52, 0.04]} castShadow>
        <boxGeometry args={[2.0, 0.36, 0.08]} />
        <meshStandardMaterial color="#3d3929" roughness={0.85} />
      </mesh>
      <Html position={[0, 0.52, 0.11]} center zIndexRange={[7, 0]} style={{ pointerEvents: 'none' }} wrapperClass="bld-sign-anchor">
        <div className="bld-plate" aria-hidden="true">
          {tools} tool{tools === 1 ? '' : 's'}
        </div>
      </Html>
    </group>
  )
}

function Roof({ mats, n, model }: { mats: Mats; n: number; model: BuildingModel }) {
  const s = useShared()
  const g = useRef<THREE.Group>(null)
  // A derived building wears the roof its facade names. A flat or parapet
  // roof is the one the garden already stands on, so it keeps the garden.
  const facade = model.derived ? model.facade : undefined
  const style = facade?.roof ?? 'flat'
  const shaped = !!facade && style !== 'flat' && style !== 'parapet'
  const shape = useMemo(() => (shaped ? roofParts(style, FLOOR_W - 1.8, FLOOR_D - 1.8) : []), [shaped, style])
  const roofMats = useMemo(() => {
    const base = new THREE.Color(paletteColor(facade?.archetype ?? 'office', facade?.palette ?? 0))
    return {
      roof: new THREE.MeshStandardMaterial({ color: base.clone().multiplyScalar(0.88), roughness: 0.9 }),
      trim: new THREE.MeshStandardMaterial({ color: '#f2ece3', roughness: 0.85 }),
    }
  }, [facade?.archetype, facade?.palette])
  const beds = useMemo(
    () => [
      { x: -2.6, z: 1.6, w: 3.6, d: 1.3 },
      { x: -3.8, z: -0.6, w: 1.4, d: 2.6 },
      { x: 1.2, z: 2.6, w: 2.4, d: 0.9 },
    ],
    []
  )
  const bushes = useMemo(() => {
    const out: { x: number; z: number; r: number; c: string }[] = []
    const greens = ['#4f8a4c', '#67a05a', '#3d7a45', '#7bb06a', '#9ab86b']
    let seed = 7
    const rnd = () => ((seed = (seed * 16807) % 2147483647) / 2147483647)
    for (const b of beds) {
      const n = Math.floor((b.w * b.d) / 0.55)
      for (let k = 0; k < n; k++) {
        out.push({
          x: b.x - b.w / 2 + 0.25 + rnd() * (b.w - 0.5),
          z: b.z - b.d / 2 + 0.25 + rnd() * (b.d - 0.5),
          r: 0.28 + rnd() * 0.3,
          c: greens[Math.floor(rnd() * greens.length)],
        })
      }
    }
    return out
  }, [beds])
  const panels = useMemo(() => {
    const out: [number, number][] = []
    for (let i = 0; i < 3; i++) for (let j = 0; j < 2; j++) out.push([1.0 + i * 1.25, -2.6 + j * 1.0])
    return out
  }, [])
  useFrame(() => {
    if (!g.current) return
    const rise = stagger(s.cur.rise, n) * s.cur.walls
    g.current.visible = rise > 0.01
    const top = floorY(n, floorHeight(model))
    g.current.position.y = THREE.MathUtils.lerp(top - 0.6, top, rise)
    g.current.scale.setScalar(Math.max(rise, 0.001))
  })
  const top = SLAB_T
  return (
    <group ref={g}>
      <mesh position={[0, SLAB_T / 2 - 0.02, 0]} castShadow receiveShadow>
        <boxGeometry args={[FLOOR_W + 0.06, SLAB_T - 0.04, FLOOR_D + 0.06]} />
        <primitive object={mats.steel} attach="material" />
      </mesh>
      <mesh position={[0, SLAB_T - 0.02, 0]} receiveShadow>
        <boxGeometry args={[FLOOR_W, 0.04, FLOOR_D]} />
        <primitive object={mats.slab} attach="material" />
      </mesh>
      {/* Parapet rail. */}
      <Instances limit={4} range={4} geometry={unitBox} material={mats.steel} castShadow>
        <Instance position={[FLOOR_W / 2 - 0.03, top + 0.45, 0]} scale={[0.06, 0.05, FLOOR_D]} />
        <Instance position={[-FLOOR_W / 2 + 0.03, top + 0.45, 0]} scale={[0.06, 0.05, FLOOR_D]} />
        <Instance position={[0, top + 0.45, FLOOR_D / 2 - 0.03]} scale={[FLOOR_W, 0.05, 0.06]} />
        <Instance position={[0, top + 0.45, -FLOOR_D / 2 + 0.03]} scale={[FLOOR_W, 0.05, 0.06]} />
      </Instances>
      <Instances limit={24} range={24} geometry={unitBox} material={mats.steel} castShadow>
        {Array.from({ length: 6 }, (_, k) => -FLOOR_D / 2 + 0.6 + k * 1.26).map((z) => (
          <Instance key={`a${z}`} position={[FLOOR_W / 2 - 0.03, top + 0.23, z]} scale={[0.04, 0.46, 0.04]} />
        ))}
        {Array.from({ length: 8 }, (_, k) => -FLOOR_W / 2 + 0.6 + k * 1.26).map((x) => (
          <Instance key={`b${x}`} position={[x, top + 0.23, FLOOR_D / 2 - 0.03]} scale={[0.04, 0.46, 0.04]} />
        ))}
      </Instances>
      {/* The garden the flat roofs carry. A shaped roof replaces it. */}
      {!shaped && (
        <>
        {/* Deck. */}
        <mesh position={[0.6, top + 0.02, 0.4]} receiveShadow>
          <boxGeometry args={[5.6, 0.04, 3.2]} />
          <meshStandardMaterial color="#d9bf98" roughness={0.9} />
        </mesh>
        {/* Planters and bushes. */}
        <Instances limit={beds.length} range={beds.length} geometry={unitBox} castShadow receiveShadow>
          <meshStandardMaterial color="#8a7862" roughness={0.9} />
          {beds.map((b, k) => (
            <Instance key={k} position={[b.x, top + 0.2, b.z]} scale={[b.w, 0.4, b.d]} />
          ))}
        </Instances>
        <Instances limit={bushes.length} range={bushes.length} geometry={unitSphere} castShadow>
          <meshStandardMaterial color="#ffffff" roughness={1} />
          {bushes.map((b, k) => (
            <Instance key={k} position={[b.x, top + 0.4 + b.r * 0.45, b.z]} scale={[b.r * 2, b.r * 1.6, b.r * 2]} color={b.c} />
          ))}
        </Instances>
        {/* Lounge. */}
        <Instances limit={6} range={6} geometry={unitBox} castShadow receiveShadow>
          <meshStandardMaterial color="#ffffff" roughness={0.85} />
          <Instance position={[0.2, top + 0.22, 0.2]} scale={[1.5, 0.4, 0.62]} color="#e8e1d3" />
          <Instance position={[0.2, top + 0.5, -0.05]} scale={[1.5, 0.28, 0.12]} color="#d9d0bf" />
          <Instance position={[2.2, top + 0.22, 0.2]} scale={[0.62, 0.4, 0.62]} color="#e8e1d3" />
          <Instance position={[2.2, top + 0.5, -0.05]} scale={[0.62, 0.28, 0.12]} color="#d9d0bf" />
          <Instance position={[1.25, top + 0.18, 1.1]} scale={[0.9, 0.32, 0.5]} color="#d8bf9a" />
          <Instance position={[3.8, top + 0.3, 1.2]} scale={[0.3, 0.6, 0.3]} color="#c7b299" />
        </Instances>
        <mesh position={[3.8, top + 0.95, 1.2]} castShadow>
          <sphereGeometry args={[0.42, 12, 10]} />
          <meshStandardMaterial color="#5e9a52" roughness={1} />
        </mesh>
        {/* Solar array. */}
        <Instances limit={panels.length} range={panels.length} geometry={unitBox} castShadow receiveShadow>
          <meshStandardMaterial color="#1f2f6b" roughness={0.25} metalness={0.4} />
          {panels.map(([x, z], k) => (
            <Instance key={k} position={[x, top + 0.3, z]} rotation={[-0.32, 0, 0]} scale={[1.1, 0.04, 0.85]} />
          ))}
        </Instances>
        <Instances limit={panels.length} range={panels.length} geometry={unitBox} material={mats.steel}>
          {panels.map(([x, z], k) => (
            <Instance key={k} position={[x, top + 0.14, z + 0.25]} scale={[0.06, 0.28, 0.06]} />
          ))}
        </Instances>
        </>
      )}
      {shaped && <RoofShape parts={shape} y={SLAB_T} mat={roofMats.roof} trim={roofMats.trim} skipDeck />}
      <Sign model={model} y={SLAB_T} />
    </group>
  )
}

function Frame({ mats, n, floorH }: { mats: Mats; n: number; floorH: number }) {
  const s = useShared()
  const g = useRef<THREE.Group>(null)
  const H = floorY(n, floorH) + SLAB_T
  useFrame(() => {
    if (!g.current) return
    const rise = smoothstep(THREE.MathUtils.clamp(s.cur.rise * 1.6 - 0.2, 0, 1)) * s.cur.walls
    g.current.visible = rise > 0.01
    g.current.scale.set(1, Math.max(rise, 0.001), 1)
  })
  const corners: [number, number][] = [
    [FLOOR_W / 2 - 0.05, FLOOR_D / 2 - 0.05],
    [FLOOR_W / 2 - 0.05, -FLOOR_D / 2 + 0.05],
    [-FLOOR_W / 2 + 0.05, FLOOR_D / 2 - 0.05],
    [-FLOOR_W / 2 + 0.05, -FLOOR_D / 2 + 0.05],
  ]
  return (
    <group ref={g}>
      <Instances limit={4} range={4} geometry={unitBox} material={mats.steel} castShadow>
        {corners.map(([x, z], k) => (
          <Instance key={k} position={[x, H / 2, z]} scale={[0.16, H, 0.16]} />
        ))}
      </Instances>
      {/* Forecourt paving in front of the entrance. */}
      <mesh position={[FLOOR_W / 2 + 1.4, 0.03, -0.6]} receiveShadow>
        <boxGeometry args={[2.6, 0.06, 5.2]} />
        <meshStandardMaterial color="#d3cdc2" roughness={0.95} />
      </mesh>
    </group>
  )
}

/**
 * `dollhouse` is the cutaway the tour draws: every floor, its rooms and the
 * two back curtain walls, open on the camera side. `closed` is the same
 * building seen from the street, with its facade on and no interior — the
 * form the city uses.
 */
export type TowerMode = 'dollhouse' | 'closed'

export default function Tower({ mode = 'dollhouse' }: { mode?: TowerMode }) {
  const model = useBuildingModel()
  const mats = useMaterials()
  const t0 = useMemo(() => performance.now() + 400, [])
  if (mode === 'closed') return <Shell />
  return (
    <group>
      {model.floors.map((_, i) => (
        <FloorUnit key={i} model={model} index={i} mats={mats} t0={t0} />
      ))}
      <Roof mats={mats} n={model.floors.length} model={model} />
      <Frame mats={mats} n={model.floors.length} floorH={floorHeight(model)} />
      {/* A derived building's rooms are named after real tools, so say so.
          The demo has its own signage in the wayfinding chapter. */}
      {model.derived && <RoomLabels />}
    </group>
  )
}
