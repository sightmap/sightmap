import { useFrame, useThree } from '@react-three/fiber'
import { Instance, Instances, Line, RoundedBox } from '@react-three/drei'
import { BLOOM_LAYER } from './bloom'
import { mergeParts, roundedBoxGeometry, type Part } from './merge'
import { useEffect, useMemo, useRef, type ComponentRef } from 'react'
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
  type Floor,
  type Kind,
  type Room,
} from './model'
import { furnish, type Item, type ItemType } from './furnish'
import {
  RAMP_DRAWN,
  RAMP_FLOOR,
  SLAB_DRAWN,
  floorGroupMatrix,
  floorRise,
  makeFloorPlace,
  placeFloor,
} from './floors'
import { sheetLinePoints } from './geometry'
import { ARCH, ARCH_TILES, FURNITURE, FURNITURE_TILES, SOFT, SOFT_TILES } from './texture-atlas'
import { applyTiledSurface, loadSurfaceAtlases, type Surfaces } from './surfaces'
import { AO_WHITE_CELL, aoUvArray } from './ao-atlas'
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

/**
 * Make a material's vertex colour drive emissive as well as diffuse.
 *
 * The per-kind materials this stands in for carry `emissive === color`, so a
 * component's carpet glows its own colour at nightfall. One shared material
 * cannot do that from a uniform — but the colour is already in `vColor`, so
 * emissive reads it from there and the night ramp stays a single
 * `emissiveIntensity`. Requires `vertexColors` and a `color` attribute: three
 * only declares the varying to the fragment shader under `USE_COLOR`.
 */
function emissiveFollowsColor(material: THREE.MeshStandardMaterial): void {
  material.onBeforeCompile = (shader) => {
    shader.fragmentShader = shader.fragmentShader.replace(
      '#include <emissivemap_fragment>',
      '#include <emissivemap_fragment>\n\ttotalEmissiveRadiance *= vColor.rgb;'
    )
  }
  // Without this, a materially identical unpatched material would be handed
  // the same compiled program.
  material.customProgramCacheKey = () => 'emissiveFollowsColor'
}

/**
 * Which baked surface every material wears, and how densely it tiles.
 *
 * `repeat` is in tiles per UV unit, and the UV unit here is one face of one
 * primitive — the merge bakes each box's own 0..1 UVs into the floor buffer
 * rather than unwrapping the merged result. So the numbers are chosen per
 * material against the size of the thing that wears it: a 10 m slab deck wants
 * its concrete five times across, a 0.6 m chair wants its upholstery once.
 * Getting this wrong shows up as texel density that jumps between neighbouring
 * objects, which is worse than no texture at all.
 */
function dressSurfaces(mats: Mats, surfaces: Surfaces): void {
  const { arch, furniture, soft } = surfaces.atlases

  // Baked contact occlusion, on the two materials that make up a floor plane:
  // the slab deck and the component carpets resting on it. Both carry a `uv1`
  // that lands on their own floor's cell of the atlas, so one texture and one
  // material serve all six floors and the merge's draw-call win is untouched.
  //
  // Only these two. Occlusion here is a top-down bake of what rests on a
  // horizontal plane, so it has nothing true to say about a wall, a mullion or
  // a chair's own sides — those keep `AO_WHITE_CELL` and stay unoccluded.
  //
  // `mats.kinds` is pointedly absent. Those dress the carpets that *move*
  // during the self-healing chapter, and this bake is nailed to floor
  // coordinates: a carpet that slides two metres would drag a shadow of where
  // its furniture used to be along with it.
  for (const m of [mats.slab, mats.zone]) {
    m.aoMap = surfaces.floorAo
    // Under 1 the shadows read as smudges; over 1 the crowded floors go to
    // ink. The bake already carries its own falloff, so this is a trim.
    m.aoMapIntensity = 0.9
    m.needsUpdate = true
  }

  applyTiledSurface(mats.slab, { atlas: arch, layout: ARCH, tile: ARCH_TILES.concrete, repeat: [5, 4] })
  applyTiledSurface(mats.spandrel, { atlas: arch, layout: ARCH, tile: ARCH_TILES.plaster, repeat: [6, 2] })
  applyTiledSurface(mats.steel, {
    atlas: arch,
    layout: ARCH,
    tile: ARCH_TILES.steel,
    repeat: [2, 2],
    // The one genuinely metallic surface in the scene, and the only one where
    // a varying metalness reads as anything: brushed streaks catching the
    // skylight differently along their length.
    metalness: true,
    normalScale: 0.35,
  })

  // The carpets are the largest textured area in the building, and the one
  // most often seen at grazing angles, so they get the gentlest normal.
  const carpet = { atlas: soft, layout: SOFT, tile: SOFT_TILES.carpet, repeat: [3, 3], normalScale: 0.3 } as const
  applyTiledSurface(mats.zone, carpet)
  for (const k of Object.keys(mats.kinds) as Kind[]) applyTiledSurface(mats.kinds[k], carpet)

  const f = (tile: (typeof FURNITURE_TILES)[keyof typeof FURNITURE_TILES], repeat: readonly [number, number]) => ({
    atlas: furniture,
    layout: FURNITURE,
    tile,
    repeat,
  })
  applyTiledSurface(mats.furniture.desk, f(FURNITURE_TILES.wood, [2, 1]))
  applyTiledSurface(mats.furniture.table, f(FURNITURE_TILES.wood, [1, 1]))
  applyTiledSurface(mats.furniture.shelf, f(FURNITURE_TILES.wood, [1, 1]))
  applyTiledSurface(mats.furniture.counter, f(FURNITURE_TILES.wood, [2, 1]))
  applyTiledSurface(mats.furniture.pot, f(FURNITURE_TILES.plastic, [1, 1]))
  applyTiledSurface(mats.furniture.rail, f(FURNITURE_TILES.plastic, [1, 1]))
  applyTiledSurface(mats.furniture.monitor, f(FURNITURE_TILES.bezel, [1, 1]))
  applyTiledSurface(mats.furniture.screen, f(FURNITURE_TILES.bezel, [1, 1]))
  applyTiledSurface(mats.furniture.book, f(FURNITURE_TILES.board, [1, 1]))

  const upholstery = { atlas: soft, layout: SOFT, tile: SOFT_TILES.upholstery, repeat: [1, 1] } as const
  applyTiledSurface(mats.furniture.chair, upholstery)
  applyTiledSurface(mats.furniture.sofa, { ...upholstery, repeat: [2, 1] })
  applyTiledSurface(mats.furniture.leaf, { atlas: soft, layout: SOFT, tile: SOFT_TILES.leaf, repeat: [1, 1] })

  // Deliberately left bare: glass and the partitions are near-transparent and
  // depth-write off, where a normal map buys nothing but a shimmer; the kiosk
  // cap and screen are emissive panels, and grain on a lit panel reads as
  // dirt on a lightbulb.
}

interface Mats {
  kinds: Record<Kind, THREE.MeshStandardMaterial>
  /** Merged component carpets: one mesh per floor, colour per vertex. */
  zone: THREE.MeshStandardMaterial
  /** Kiosk cap: lit panel, tinted by the one colour every action component shares. */
  kioskCap: THREE.MeshStandardMaterial
  kioskScreen: THREE.MeshStandardMaterial
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
    const zone = new THREE.MeshStandardMaterial({
      color: '#ffffff',
      roughness: 0.92,
      vertexColors: true,
      emissive: new THREE.Color('#ffffff'),
      emissiveIntensity: 0,
    })
    emissiveFollowsColor(zone)
    return {
      kinds,
      zone,
      kioskCap: new THREE.MeshStandardMaterial({
        color: '#fbf8f2',
        emissive: new THREE.Color(KIND_COLORS.action),
        emissiveIntensity: 0.6,
        roughness: 0.4,
      }),
      kioskScreen: new THREE.MeshStandardMaterial({
        color: '#fbf8f2',
        emissive: new THREE.Color('#ffffff'),
        emissiveIntensity: 0.35,
        roughness: 0.3,
      }),
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
  // The atlases arrive well after first paint. Until they do, every material
  // is exactly what it was before this existed — an untextured surface with a
  // uniform roughness — so a slow or failed fetch costs fidelity, never the
  // scene.
  const { gl, invalidate } = useThree()
  useEffect(() => {
    let live = true
    loadSurfaceAtlases(gl)
      .then((surfaces) => {
        if (!live) return
        dressSurfaces(mats, surfaces)
        // Under reduced motion the tour renders on demand and has very likely
        // already settled, exactly as with the environment map next door.
        invalidate()
      })
      .catch((err) => {
        console.error('[building] surface atlases failed to load; materials stay untextured', err)
      })
    return () => {
      live = false
    }
  }, [gl, mats, invalidate])

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
    mats.zone.emissiveIntensity = n * 0.35
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

const CARPET_RADIUS = 0.03
const CARPET_SMOOTHNESS = 2

/** Where a component's carpet sits, given whatever platform it stands on. */
const liftOf = (room: Room): number => (room.base ? PLATE : 0)

/** A floor's component carpets, sorted into what can be baked and what cannot. */
interface FloorCarpets {
  /** Every static carpet on the floor, in one buffer, colour per vertex. */
  merged: THREE.BufferGeometry | null
  /** Carpets the self-healing chapter slides sideways, so they keep their own mesh. */
  moving: Room[]
}

/**
 * Give a floor-space geometry the second UV set `aoMap` reads, aimed at this
 * floor's cell of the occlusion atlas.
 *
 * Derived from the vertex positions rather than authored, which is what lets
 * one bake cover a merged buffer of a few hundred primitives: the unwrap is
 * top-down, so a vertex's texel is simply where it stands on the floor plan.
 */
function setFloorAoUv(geometry: THREE.BufferGeometry, cell: number): THREE.BufferGeometry {
  const pos = geometry.getAttribute('position')
  geometry.setAttribute(
    'uv1',
    new THREE.BufferAttribute(aoUvArray(pos.array as Float32Array, FLOOR_W, FLOOR_D, cell), 2)
  )
  return geometry
}

function floorCarpets(floor: Floor, index: number): FloorCarpets {
  const parts: Part[] = []
  const moving: Room[] = []
  for (const room of floor.rooms) {
    if (room.alt) {
      moving.push(room)
      continue
    }
    const color = new THREE.Color(KIND_COLORS[room.kind])
    const lift = liftOf(room)
    for (const b of room.blocks ?? [{ x: room.x, z: room.z, w: room.w, d: room.d }]) {
      parts.push({
        geometry: roundedBoxGeometry(b.w, PLATE, b.d, CARPET_RADIUS, CARPET_SMOOTHNESS),
        position: [b.x, lift + PLATE / 2, b.z],
        color,
      })
    }
  }
  return { merged: parts.length ? setFloorAoUv(mergeParts(parts), index) : null, moving }
}

/**
 * A component whose carpet moves during the self-healing chapter. Its offset is
 * per-frame, so it cannot be baked into the floor's merged buffer and keeps the
 * per-block meshes the whole floor used to have.
 */
function MovingZone({ room, mats }: { room: Room; mats: Mats }) {
  const s = useShared()
  const g = useRef<THREE.Group>(null)
  const blocks = room.blocks ?? [{ x: room.x, z: room.z, w: room.w, d: room.d }]
  const lift = liftOf(room)
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
    </group>
  )
}

// ---------------------------------------------------------------------------
// Curtain walls on the two back sides: spandrel, glass, mullions, head beam.
//
// Every floor's walls are the same two runs, so the merged buffers are built
// once at module scope and shared by all six floors' meshes. Each floor still
// gets its own mesh, hung under its own `walls` group, so the ramp that grows
// the glazing out of the slab is untouched.

/** Local pose of one mullion, in the curtain wall's space. */
interface Mullion {
  position: [number, number, number]
  scale: [number, number, number]
}

interface CurtainWalls {
  spandrel: THREE.BufferGeometry
  glass: THREE.BufferGeometry
  head: THREE.BufferGeometry
  mullions: Mullion[]
}

function buildCurtainWalls(): CurtainWalls {
  const spandrel: Part[] = []
  const glass: Part[] = []
  const head: Part[] = []
  const mullions: Mullion[] = []
  for (const side of ['x', 'z'] as const) {
    const len = side === 'x' ? FLOOR_D : FLOOR_W
    const n = Math.round(len / 1.25)
    const at = (u: number, y: number): [number, number, number] =>
      side === 'x' ? [-FLOOR_W / 2 + WALL_T / 2, y, u] : [u, y, -FLOOR_D / 2 + WALL_T / 2]
    const size = (u: number, y: number, t: number): [number, number, number] => (side === 'x' ? [t, y, u] : [u, y, t])
    spandrel.push({ geometry: new THREE.BoxGeometry(...size(len, 0.32, WALL_T)), position: at(0, 0.16) })
    glass.push({
      geometry: new THREE.BoxGeometry(...size(len, WALL_H - 0.42, 0.02)),
      position: at(0, 0.32 + (WALL_H - 0.42) / 2),
    })
    head.push({ geometry: new THREE.BoxGeometry(...size(len, 0.1, WALL_T + 0.02)), position: at(0, WALL_H - 0.05) })
    for (let k = 0; k <= n; k++) {
      const u = -len / 2 + (k * len) / n
      mullions.push({ position: at(u, WALL_H / 2), scale: size(0.07, WALL_H, WALL_T + 0.02) })
    }
  }
  return { spandrel: mergeParts(spandrel), glass: mergeParts(glass), head: mergeParts(head), mullions }
}

const CURTAIN_WALLS = buildCurtainWalls()

// ---------------------------------------------------------------------------
// The per-floor reveal ramp.
//
// Three things need a floor's transform each frame: the floor's own group, the
// mullion instances and the kiosk instances. Deriving it once, here, from the
// damped scene parameters is what lets the instanced meshes live outside the
// floor groups without their pose ever drifting from the floor's own.

/** Parks an instance at a degenerate scale: rasterises nothing, hits no ray. */
const HIDDEN = new THREE.Matrix4().makeScale(0, 0, 0)

// ---------------------------------------------------------------------------
// Parts instanced across floors.
//
// Mullions and kiosks are identical from floor to floor, so each floor building
// its own was six times the draw calls for one distinct shape. They cannot just
// be hoisted and left alone, though: the scroll hides and scales each floor
// independently, so every instance composes its own floor's ramp via
// `floorGroupMatrix` and a floor that is not drawn parks its instances.
//
// Parking matters beyond the pixels. The label occlusion raycast targets the
// tower group, so a mullion left standing over a hidden floor would occlude
// labels that ought to be readable.

/** Local matrices of one floor's mullions, in the curtain wall's own space. */
const MULLION_LOCAL = CURTAIN_WALLS.mullions.map((m) =>
  new THREE.Matrix4().compose(new THREE.Vector3(...m.position), new THREE.Quaternion(), new THREE.Vector3(...m.scale))
)

function Mullions({ mats }: { mats: Mats }) {
  const s = useShared()
  const mesh = useRef<THREE.InstancedMesh>(null)
  const per = MULLION_LOCAL.length
  const v = useMemo(
    () => ({
      place: makeFloorPlace(),
      floor: new THREE.Matrix4(),
      scratch: new THREE.Matrix4(),
      out: new THREE.Matrix4(),
    }),
    []
  )
  useFrame(() => {
    const m = mesh.current
    if (!m) return
    let any = false
    for (let i = 0; i < FLOORS.length; i++) {
      const p = placeFloor(i, s.cur.rise, s.cur.spread, s.cur.walls, v.place)
      const drawn = p.walls > RAMP_DRAWN
      if (drawn) {
        any = true
        floorGroupMatrix(p, p.walls, v.floor, v.scratch)
      }
      for (let k = 0; k < per; k++) {
        m.setMatrixAt(i * per + k, drawn ? v.out.multiplyMatrices(v.floor, MULLION_LOCAL[k]) : HIDDEN)
      }
    }
    m.instanceMatrix.needsUpdate = true
    m.visible = any
  })
  // Frustum culling is off on both instanced meshes: the instances move every
  // frame and span the whole tower, so the bounding sphere would have to be
  // recomputed just as often to stay honest.
  return <instancedMesh ref={mesh} args={[unitBox, mats.steel, FLOORS.length * per]} castShadow frustumCulled={false} />
}

/** One kiosk's placement, in its floor's `rooms` group space. */
interface KioskSpec {
  floor: number
  position: [number, number, number]
  /** Where the self-healing chapter slides it to, if it moves at all. */
  alt: [number, number] | null
}

// Every action component is `kind: 'action'`, so all four kiosks share one
// colour and the instances need no per-instance colour at all.
const KIOSKS: KioskSpec[] = FLOORS.flatMap((floor, i) =>
  floor.rooms
    .filter((r) => r.kind === 'action')
    .map((r) => ({
      floor: i,
      position: [r.x, liftOf(r) + PLATE, r.z] as [number, number, number],
      alt: r.alt ? ([r.alt.x, r.alt.z] as [number, number]) : null,
    }))
)

const KIOSK_BODY = roundedBoxGeometry(0.55, KIOSK_H - 0.1, 0.55, 0.04, 2).translate(0, (KIOSK_H - 0.1) / 2, 0)
const KIOSK_CAP = new THREE.BoxGeometry(0.62, 0.1, 0.62).translate(0, KIOSK_H - 0.05, 0)
const KIOSK_SCREEN = new THREE.PlaneGeometry(0.36, 0.5).translate(0, KIOSK_H / 2, 0.29)

/**
 * Every floor's kiosks, as three instanced meshes: body, cap, screen.
 *
 * The one kiosk the self-healing chapter relocates carries its carpet's offset
 * here, since it no longer rides inside that carpet's moving group.
 */
function Kiosks({ mats }: { mats: Mats }) {
  const s = useShared()
  const body = useRef<THREE.InstancedMesh>(null)
  const cap = useRef<THREE.InstancedMesh>(null)
  const screen = useRef<THREE.InstancedMesh>(null)
  const v = useMemo(
    () => ({
      places: FLOORS.map(makeFloorPlace),
      floors: FLOORS.map(() => new THREE.Matrix4()),
      scratch: new THREE.Matrix4(),
      local: new THREE.Matrix4(),
      out: new THREE.Matrix4(),
    }),
    []
  )
  useFrame(() => {
    const meshes = [body.current, cap.current, screen.current]
    if (!body.current || !cap.current || !screen.current) return
    for (let i = 0; i < FLOORS.length; i++) {
      const p = placeFloor(i, s.cur.rise, s.cur.spread, s.cur.walls, v.places[i])
      if (p.fill > RAMP_DRAWN) floorGroupMatrix(p, p.fill, v.floors[i], v.scratch)
    }
    const heal = smoothstep(s.healShift)
    let any = false
    for (let k = 0; k < KIOSKS.length; k++) {
      const spec = KIOSKS[k]
      const drawn = v.places[spec.floor].fill > RAMP_DRAWN
      if (drawn) {
        any = true
        const [x, y, z] = spec.position
        const dx = spec.alt ? (spec.alt[0] - x) * heal : 0
        const dz = spec.alt ? (spec.alt[1] - z) * heal : 0
        v.local.makeTranslation(x + dx, y, z + dz)
        v.out.multiplyMatrices(v.floors[spec.floor], v.local)
      }
      const at = drawn ? v.out : HIDDEN
      body.current.setMatrixAt(k, at)
      cap.current.setMatrixAt(k, at)
      screen.current.setMatrixAt(k, at)
    }
    for (const m of meshes) {
      if (!m) continue
      m.instanceMatrix.needsUpdate = true
      m.visible = any
    }
  })
  const n = KIOSKS.length
  return (
    <>
      <instancedMesh
        ref={body}
        args={[KIOSK_BODY, mats.kinds.action, n]}
        castShadow
        receiveShadow
        frustumCulled={false}
      />
      <instancedMesh ref={cap} args={[KIOSK_CAP, mats.kioskCap, n]} frustumCulled={false} />
      <instancedMesh ref={screen} args={[KIOSK_SCREEN, mats.kioskScreen, n]} frustumCulled={false} />
    </>
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
  const items = useMemo(() => floor.rooms.flatMap((r) => furnish(r, i).items), [floor, i])
  const place = useMemo(() => makeFloorPlace(), [])
  const carpets = useMemo(() => floorCarpets(floor, i), [floor, i])
  // The deck is the other half of the floor plane, and the half the contact
  // shadows mostly land on. Built here rather than as JSX `<boxGeometry>` so
  // it can carry this floor's AO cell in its second UV set.
  const deck = useMemo(() => {
    const g = new THREE.BoxGeometry(FLOOR_W, 0.04, FLOOR_D)
    return setFloorAoUv(g, i)
  }, [i])
  useEffect(() => () => deck.dispose(), [deck])
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
    const p = placeFloor(i, c.rise, c.spread, c.walls, place)
    const rise = p.rise
    if (g.current) {
      g.current.position.set(p.x, p.y, p.z)
      g.current.rotation.y = p.ry
    }
    // The line work sketches itself in once, on load, then fades as the
    // sheet becomes a slab. Reduced motion gets it already drawn: the sketch
    // is exactly the kind of motion the preference asks us to skip, and it
    // runs on wall-clock time, so under frameloop="demand" it would otherwise
    // freeze half-finished when the scene stops drawing.
    const drawT = s.reduced ? 1 : THREE.MathUtils.clamp((performance.now() - t0 - i * 260) / 3200, 0, 1)
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
      slab.current.visible = p.rise > SLAB_DRAWN
      slab.current.scale.set(1, Math.max(p.rise, RAMP_FLOOR), 1)
    }
    if (rooms.current) {
      rooms.current.scale.y = Math.max(p.fill, RAMP_FLOOR)
      rooms.current.visible = p.fill > RAMP_DRAWN
    }
    if (walls.current) {
      walls.current.scale.y = Math.max(p.walls, RAMP_FLOOR)
      walls.current.visible = p.walls > RAMP_DRAWN
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
        <mesh position={[0, SLAB_T - 0.02, 0]} geometry={deck} receiveShadow>
          <primitive object={mats.slab} attach="material" />
        </mesh>
      </group>
      <group ref={rooms} position={[0, SLAB_T, 0]}>
        {carpets.merged && <mesh geometry={carpets.merged} material={mats.zone} receiveShadow />}
        {carpets.moving.map((r) => (
          <MovingZone key={r.name} room={r} mats={mats} />
        ))}
        <Furniture items={items} mats={mats} />
      </group>
      <group ref={walls} position={[0, SLAB_T, 0]}>
        <mesh geometry={CURTAIN_WALLS.spandrel} material={mats.spandrel} castShadow receiveShadow />
        {/* Tagged onto BLOOM_LAYER (see bloom.ts) so NightfallBloom can render
            just this mesh for its bloom source — the only reliable way to
            scope bloom to the windows given monitors and screens read
            brighter than this glass does at night. */}
        <mesh
          ref={(o) => o?.layers.enable(BLOOM_LAYER)}
          geometry={CURTAIN_WALLS.glass}
          material={mats.glass}
        />
        <mesh geometry={CURTAIN_WALLS.head} material={mats.steel} castShadow />
        {/* Mullions are not here: they are instanced across all floors at once,
            by <Mullions />, which composes this group's ramp itself. */}
      </group>
    </group>
  )
}

// ---------------------------------------------------------------------------
// Roof: garden beds, deck, lounge, solar array, and the structural frame.

function Roof({ mats }: { mats: Mats }) {
  const s = useShared()
  const g = useRef<THREE.Group>(null)
  // The roof wears the same material as the floor decks, so it inherits their
  // `aoMap` — and a geometry with no `uv1` samples that map at 0,0, which is
  // the corner of floor 0's cell. Without this the roof garden would wear the
  // ground floor's furniture shadows. Aim it at the white cell instead.
  const roofDeck = useMemo(() => setFloorAoUv(new THREE.BoxGeometry(FLOOR_W, 0.04, FLOOR_D), AO_WHITE_CELL), [])
  useEffect(() => () => roofDeck.dispose(), [roofDeck])
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
      <mesh position={[0, SLAB_T - 0.02, 0]} geometry={roofDeck} receiveShadow>
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
      <Mullions mats={mats} />
      <Kiosks mats={mats} />
      <Roof mats={mats} />
      <Frame mats={mats} />
    </group>
  )
}
