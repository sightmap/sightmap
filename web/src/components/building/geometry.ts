// Geometry derived from the model: the white line work drawn on each
// blueprint sheet, and the walking paths a journey follows through the
// building. Kept apart from the React components so the maths is testable
// and so the same path feeds both the crowd and the trajectory highlight.
import * as THREE from 'three'
import {
  CORE,
  CORE_DOOR,
  FLOORS,
  FLOOR_D,
  FLOOR_W,
  LANES,
  SHEET,
  findRoom,
  roomStand,
  surfaceAt,
  type Journey,
} from './model'
import { DRAWN, furnish } from './furnish'

type P3 = [number, number, number]

/** Pairs of points (LineSegments layout) drawing one floor plan on its sheet. */
export function sheetLinePoints(floorIndex: number): P3[] {
  const y = 0.012
  const pts: P3[] = []
  const rect = (x: number, z: number, w: number, d: number) => {
    const a: P3 = [x - w / 2, y, z - d / 2]
    const b: P3 = [x + w / 2, y, z - d / 2]
    const c: P3 = [x + w / 2, y, z + d / 2]
    const e: P3 = [x - w / 2, y, z + d / 2]
    pts.push(a, b, b, c, c, e, e, a)
  }
  // Sheet border and the footprint of the floor.
  rect(0, 0, SHEET.w - 0.3, SHEET.d - 0.3)
  rect(0, 0, FLOOR_W, FLOOR_D)
  // Title block in the sheet's front-right corner.
  const tbx = SHEET.w / 2 - 0.15 - 1.4
  const tbz = -SHEET.d / 2 + 0.15 + 0.32
  rect(tbx, tbz, 2.8, 0.64)
  pts.push([tbx - 1.4, y, tbz], [tbx + 1.4, y, tbz])
  pts.push([tbx - 0.4, y, tbz - 0.32], [tbx - 0.4, y, tbz + 0.32])
  // Core and its door.
  rect(CORE.x, CORE.z, CORE.w, CORE.d)
  pts.push([CORE.x + CORE.w / 2, y, CORE_DOOR.z], [CORE_DOOR.x, y, CORE_DOOR.z])
  // Door swing on the core.
  const arc = (cx: number, cz: number, rad: number, a0: number, a1: number) => {
    const n = 6
    for (let k = 0; k < n; k++) {
      const t0 = a0 + ((a1 - a0) * k) / n
      const t1 = a0 + ((a1 - a0) * (k + 1)) / n
      pts.push([cx + Math.cos(t0) * rad, y, cz + Math.sin(t0) * rad], [cx + Math.cos(t1) * rad, y, cz + Math.sin(t1) * rad])
    }
  }
  arc(CORE.x + CORE.w / 2, CORE_DOOR.z, 0.45, 0, Math.PI / 2)
  // Rooms, with the footprints of their furniture drawn lighter in spirit:
  // the same layout the built floor gets, so the drawing is the plan.
  for (const r of FLOORS[floorIndex].rooms) {
    const blocks = r.blocks ?? [{ x: r.x, z: r.z, w: r.w, d: r.d }]
    for (const b of blocks) rect(b.x, b.z, b.w, b.d)
    for (const it of furnish(r)) {
      if (!DRAWN.includes(it.type)) continue
      const c = Math.cos(it.ry)
      const s = Math.sin(it.ry)
      const hw = it.sx / 2
      const hd = it.sz / 2
      const corners: [number, number][] = [
        [-hw, -hd],
        [hw, -hd],
        [hw, hd],
        [-hw, hd],
      ].map(([dx, dz]) => [it.x + dx * c - dz * s, it.z + dx * s + dz * c])
      for (let k = 0; k < 4; k++) {
        const a = corners[k]
        const b2 = corners[(k + 1) % 4]
        pts.push([a[0], y, a[1]], [b2[0], y, b2[1]])
      }
    }
  }
  // Dimension ticks along the front edge of the sheet.
  for (let x = -FLOOR_W / 2; x <= FLOOR_W / 2 + 0.01; x += 1) {
    pts.push([x, y, FLOOR_D / 2 + 0.25], [x, y, FLOOR_D / 2 + 0.45])
  }
  pts.push([-FLOOR_W / 2, y, FLOOR_D / 2 + 0.35], [FLOOR_W / 2, y, FLOOR_D / 2 + 0.35])
  return pts
}

export interface Path {
  points: THREE.Vector3[]
  /** Cumulative length at each point. */
  cum: number[]
  /** Index into points for each stop of the journey. */
  stops: number[]
  length: number
}

type P2 = { x: number; z: number }

const EPS = 1e-4

interface Graph {
  nodes: Map<string, P2>
  adj: Map<string, { to: string; w: number }[]>
}

const quant = (n: number): number => Math.round(n * 1000) / 1000
const nid = (x: number, z: number): string => `${quant(x)},${quant(z)}`

function addNode(g: Graph, x: number, z: number): string {
  const k = nid(x, z)
  if (!g.nodes.has(k)) {
    g.nodes.set(k, { x: quant(x), z: quant(z) })
    g.adj.set(k, [])
  }
  return k
}

function addEdge(g: Graph, a: string, b: string): void {
  if (a === b) return
  const pa = g.nodes.get(a)!
  const pb = g.nodes.get(b)!
  const w = Math.hypot(pa.x - pb.x, pa.z - pb.z)
  const ea = g.adj.get(a)!
  if (!ea.some((e) => e.to === b)) ea.push({ to: b, w })
  const eb = g.adj.get(b)!
  if (!eb.some((e) => e.to === a)) eb.push({ to: a, w })
}

function between(t: number, a: number, b: number): boolean {
  return t >= Math.min(a, b) - EPS && t <= Math.max(a, b) + EPS
}

function laneSegments(floor: number): { a: P2; b: P2 }[] {
  const segs: { a: P2; b: P2 }[] = []
  for (const lane of LANES[floor]) {
    for (let i = 0; i < lane.length - 1; i++) {
      segs.push({ a: { x: lane[i][0], z: lane[i][1] }, b: { x: lane[i + 1][0], z: lane[i + 1][1] } })
    }
  }
  return segs
}

function isH(s: { a: P2; b: P2 }): boolean {
  return Math.abs(s.a.z - s.b.z) < EPS
}

function isV(s: { a: P2; b: P2 }): boolean {
  return Math.abs(s.a.x - s.b.x) < EPS
}

function splitPoints(s: { a: P2; b: P2 }, others: { a: P2; b: P2 }[]): P2[] {
  const pts: P2[] = [{ ...s.a }, { ...s.b }]
  for (const o of others) {
    if (isH(s) && isV(o)) {
      const z = s.a.z
      const x = o.a.x
      if (between(x, s.a.x, s.b.x) && between(z, o.a.z, o.b.z)) pts.push({ x, z })
    } else if (isV(s) && isH(o)) {
      const x = s.a.x
      const z = o.a.z
      if (between(z, s.a.z, s.b.z) && between(x, o.a.x, o.b.x)) pts.push({ x, z })
    }
    if (isH(s) && isH(o) && Math.abs(s.a.z - o.a.z) < EPS) {
      if (between(o.a.x, s.a.x, s.b.x)) pts.push({ x: o.a.x, z: s.a.z })
      if (between(o.b.x, s.a.x, s.b.x)) pts.push({ x: o.b.x, z: s.a.z })
    }
    if (isV(s) && isV(o) && Math.abs(s.a.x - o.a.x) < EPS) {
      if (between(o.a.z, s.a.z, s.b.z)) pts.push({ x: s.a.x, z: o.a.z })
      if (between(o.b.z, s.a.z, s.b.z)) pts.push({ x: s.a.x, z: o.b.z })
    }
  }
  return pts
}

const graphs: Graph[] = []

function graphFor(floor: number): Graph {
  const cached = graphs[floor]
  if (cached) return cached
  const g: Graph = { nodes: new Map(), adj: new Map() }
  const raw = laneSegments(floor)
  for (const s of raw) {
    const pts = splitPoints(s, raw)
    if (isH(s)) pts.sort((a, b) => a.x - b.x)
    else pts.sort((a, b) => a.z - b.z)
    const uniq: P2[] = []
    for (const p of pts) {
      const last = uniq[uniq.length - 1]
      if (!last || Math.hypot(p.x - last.x, p.z - last.z) > EPS) uniq.push(p)
    }
    for (let i = 0; i < uniq.length - 1; i++) {
      addEdge(g, addNode(g, uniq[i].x, uniq[i].z), addNode(g, uniq[i + 1].x, uniq[i + 1].z))
    }
  }
  graphs[floor] = g
  return g
}

function cloneGraph(src: Graph): Graph {
  const g: Graph = { nodes: new Map(), adj: new Map() }
  for (const [k, p] of src.nodes) g.nodes.set(k, { ...p })
  for (const [k, edges] of src.adj) g.adj.set(k, edges.map((e) => ({ ...e })))
  return g
}

function projectToGraph(g: Graph, x: number, z: number): { x: number; z: number; a: string; b: string } {
  let best = { d: Infinity, x, z, a: '', b: '' }
  for (const [ak, nbrs] of g.adj) {
    const A = g.nodes.get(ak)!
    for (const e of nbrs) {
      if (e.to <= ak) continue
      const B = g.nodes.get(e.to)!
      const vx = B.x - A.x
      const vz = B.z - A.z
      const len2 = vx * vx + vz * vz
      const t = len2 < EPS ? 0 : Math.max(0, Math.min(1, ((x - A.x) * vx + (z - A.z) * vz) / len2))
      const px = A.x + vx * t
      const pz = A.z + vz * t
      const d = Math.hypot(x - px, z - pz)
      if (d < best.d) best = { d, x: px, z: pz, a: ak, b: e.to }
    }
  }
  return best
}

function dijkstra(g: Graph, start: string, end: string): string[] {
  const dist = new Map<string, number>([[start, 0]])
  const prev = new Map<string, string>()
  const pq: { k: string; d: number }[] = [{ k: start, d: 0 }]
  while (pq.length) {
    let best = 0
    for (let i = 1; i < pq.length; i++) if (pq[i].d < pq[best].d) best = i
    const { k, d } = pq.splice(best, 1)[0]
    if (d !== dist.get(k)) continue
    if (k === end) break
    for (const e of g.adj.get(k) ?? []) {
      const nd = d + e.w
      if (nd < (dist.get(e.to) ?? Infinity)) {
        dist.set(e.to, nd)
        prev.set(e.to, k)
        pq.push({ k: e.to, d: nd })
      }
    }
  }
  const path = [end]
  while (path[0] !== start) {
    const p = prev.get(path[0])
    if (!p) break
    path.unshift(p)
  }
  return path
}

/** Axis-aligned lane route on one floor, including the start and end stands. */
export function routeOnFloor(floor: number, ax: number, az: number, bx: number, bz: number): P2[] {
  if (Math.hypot(ax - bx, az - bz) < EPS) return [{ x: ax, z: az }]
  const g = cloneGraph(graphFor(floor))
  const pa = projectToGraph(g, ax, az)
  const pb = projectToGraph(g, bx, bz)
  const sa = addNode(g, pa.x, pa.z)
  const sb = addNode(g, pb.x, pb.z)
  addEdge(g, sa, pa.a)
  addEdge(g, sa, pa.b)
  addEdge(g, sb, pb.a)
  addEdge(g, sb, pb.b)
  const ids = dijkstra(g, sa, sb)
  const out: P2[] = []
  const push = (p: P2) => {
    const last = out[out.length - 1]
    if (!last || Math.hypot(p.x - last.x, p.z - last.z) > EPS) out.push(p)
  }
  push({ x: ax, z: az })
  if (Math.hypot(ax - pa.x, az - pa.z) > EPS) push({ x: pa.x, z: pa.z })
  for (const k of ids) push(g.nodes.get(k)!)
  if (Math.hypot(bx - pb.x, bz - pb.z) > EPS) push({ x: pb.x, z: pb.z })
  push({ x: bx, z: bz })
  return out
}

/**
 * The walk for a journey: room to room along that floor's lanes, or
 * room → door → up the shaft → out the door → room when the floor changes.
 * `lift` raises the whole path (the trajectory ribbon floats a little).
 */
export function buildPath(journey: Journey, lift = 0, healShift = 0): Path {
  const points: THREE.Vector3[] = []
  const stops: number[] = []
  const push = (p: P3) => {
    const v = new THREE.Vector3(p[0], p[1] + lift, p[2])
    const last = points[points.length - 1]
    if (!last || last.distanceToSquared(v) > 1e-6) points.push(v)
    return points.length - 1
  }
  const follow = (floor: number, route: { x: number; z: number }[], skipFirst: boolean) => {
    for (const p of skipFirst ? route.slice(1) : route) {
      push([p.x, surfaceAt(floor, p.x, p.z), p.z])
    }
  }
  let last: P3 | null = null
  for (let k = 0; k < journey.stops.length; k++) {
    const [f, name] = journey.stops[k]
    const room = findRoom(f, name)
    const stand = roomStand(f, room, room.alt ? healShift : 0)
    if (!last) {
      stops.push(push(stand))
      last = stand
      continue
    }
    const [pf] = journey.stops[k - 1]
    if (pf !== f) {
      follow(pf, routeOnFloor(pf, last[0], last[2], CORE_DOOR.x, CORE_DOOR.z), true)
      push([CORE.x, surfaceAt(pf, CORE.x, CORE.z), CORE.z])
      push([CORE.x, surfaceAt(f, CORE.x, CORE.z), CORE.z])
      follow(f, routeOnFloor(f, CORE_DOOR.x, CORE_DOOR.z, stand[0], stand[2]), false)
    } else {
      follow(f, routeOnFloor(f, last[0], last[2], stand[0], stand[2]), true)
    }
    stops.push(points.length - 1)
    last = stand
  }
  const cum = [0]
  for (let i = 1; i < points.length; i++) cum.push(cum[i - 1] + points[i].distanceTo(points[i - 1]))
  return { points, cum, stops, length: cum[cum.length - 1] }
}

/** Point at arc-length `d` along the path, written into `out`. */
export function pointAt(path: Path, d: number, out: THREE.Vector3): THREE.Vector3 {
  const { points, cum } = path
  if (d <= 0) return out.copy(points[0])
  if (d >= path.length) return out.copy(points[points.length - 1])
  let i = 1
  while (i < cum.length && cum[i] < d) i++
  const seg = cum[i] - cum[i - 1]
  const t = seg > 0 ? (d - cum[i - 1]) / seg : 0
  return out.copy(points[i - 1]).lerp(points[i], t)
}
