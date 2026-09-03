import { useFrame } from '@react-three/fiber'
import { Instance, Instances, Line, RoundedBox } from '@react-three/drei'
import { useMemo, useRef, type ComponentRef } from 'react'
import * as THREE from 'three'
import {
  FLOORS,
  FLOOR_D,
  FLOOR_H,
  FLOOR_W,
  KIND_COLORS,
  KIOSK_H,
  PLATE,
  SHEET,
  SLAB_T,
  WALL_T,
  floorY,
  type Kind,
  type Room,
} from './model'
import { furnish, type Item, type ItemType } from './furnish'
import { floorRise, makeFloorPlace, placeFloor } from './floors'
import { sheetLinePoints } from './geometry'
import { smoothstep } from './chapters'
import { useShared } from './state'

// Each floor starts life as a blueprint sheet lying on the table. As `rise`
// climbs, the sheet lifts to its floor height, fades into a slab, and the
// floor's zones, furniture, glass and people grow out of the drawing. The
// curtain walls, roof garden and structural frame follow.

type LineRef = ComponentRef<typeof Line>

const WALL_H = FLOOR_H - SLAB_T
const N = FLOORS.length
const STEEL = '#26272c'

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
      partition: plain({ transparent: true, opacity: 0.22, roughness: 0.1, depthWrite: false }),
      rail: plain({ roughness: 0.5 }),
      counter: plain({ roughness: 0.6 }),
    }
    return {
      kinds,
      slab: new THREE.MeshStandardMaterial({ color: '#f2ece3', roughness: 0.92 }),
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
    mats.glass.opacity = THREE.MathUtils.lerp(0.22, 0.5, n)
    mats.furniture.monitor.emissiveIntensity = THREE.MathUtils.lerp(0.25, 1.3, n)
    mats.furniture.screen.emissiveIntensity = THREE.MathUtils.lerp(0.3, 1.2, n)
    for (const k of Object.keys(mats.kinds) as Kind[]) mats.kinds[k].emissiveIntensity = n * 0.35
  })
  return mats
}

// ---------------------------------------------------------------------------
// Furniture: one instanced mesh per item type per floor.

const unitBox = new THREE.BoxGeometry(1, 1, 1)
const unitCyl = new THREE.CylinderGeometry(0.5, 0.5, 1, 14)
const unitSphere = new THREE.SphereGeometry(0.5, 12, 10)
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
  partition: unitBox,
  rail: unitBox,
  counter: unitBox,
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
              rotation={[0, it.ry, 0]}
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

function CurtainWall({ mats, side }: { mats: Mats; side: 'x' | 'z' }) {
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
      <mesh position={at(0, 0.32 + (WALL_H - 0.42) / 2)}>
        <boxGeometry args={size(len, WALL_H - 0.42, 0.02)} />
        <primitive object={mats.glass} attach="material" />
      </mesh>
      <mesh position={at(0, WALL_H - 0.05)} castShadow>
        <boxGeometry args={size(len, 0.1, WALL_T + 0.02)} />
        <primitive object={mats.steel} attach="material" />
      </mesh>
      <Instances limit={mullions.length} range={mullions.length} geometry={unitBox} material={mats.steel} castShadow>
        {mullions.map((u) => (
          <Instance key={u} position={at(u, WALL_H / 2)} scale={size(0.07, WALL_H, WALL_T + 0.02)} />
        ))}
      </Instances>
    </group>
  )
}

// ---------------------------------------------------------------------------

function FloorUnit({ index: i, mats, t0 }: { index: number; mats: Mats; t0: number }) {
  const s = useShared()
  const floor = FLOORS[i]
  const g = useRef<THREE.Group>(null)
  const sheet = useRef<THREE.Mesh>(null)
  const sheetMat = useRef<THREE.MeshStandardMaterial>(null)
  const lines = useRef<LineRef>(null)
  const slab = useRef<THREE.Group>(null)
  const rooms = useRef<THREE.Group>(null)
  const walls = useRef<THREE.Group>(null)
  const pts = useMemo(() => sheetLinePoints(i), [i])
  // People are not here: the crowd is drawn by People.tsx, which places the
  // occupants of every floor into one set of instanced meshes.
  const items = useMemo(() => floor.rooms.flatMap((r) => furnish(r).items), [floor])
  const place = useMemo(() => makeFloorPlace(), [])
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
  useFrame(() => {
    const c = s.cur
    const p = placeFloor(i, c.rise, c.spread, place)
    const rise = p.rise
    if (g.current) {
      g.current.position.set(p.x, p.y, p.z)
      g.current.rotation.y = p.ry
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
      rooms.current.scale.y = Math.max(p.fill, 0.001)
      rooms.current.visible = p.fill > 0.005
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
        <CurtainWall mats={mats} side="x" />
        <CurtainWall mats={mats} side="z" />
      </group>
    </group>
  )
}

// ---------------------------------------------------------------------------
// Roof: garden beds, deck, lounge, solar array, and the structural frame.

function Roof({ mats }: { mats: Mats }) {
  const s = useShared()
  const g = useRef<THREE.Group>(null)
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
    const rise = floorRise(s.cur.rise, N) * s.cur.walls
    g.current.visible = rise > 0.01
    g.current.position.y = THREE.MathUtils.lerp(floorY(N) - 0.6, floorY(N), rise)
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
    </group>
  )
}

function Frame({ mats }: { mats: Mats }) {
  const s = useShared()
  const g = useRef<THREE.Group>(null)
  const H = floorY(N) + SLAB_T
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

export default function Tower() {
  const s = useShared()
  const mats = useMaterials()
  const t0 = useMemo(() => performance.now() + 400, [])
  // Published on the shared state so the labels can raycast against it: this
  // group is what stands between them and the camera.
  return (
    <group ref={s.tower}>
      {FLOORS.map((_, i) => (
        <FloorUnit key={i} index={i} mats={mats} t0={t0} />
      ))}
      <Roof mats={mats} />
      <Frame mats={mats} />
    </group>
  )
}
