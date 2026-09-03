// Where a floor is, at any point in its journey from a sheet fanned across the
// table to a storey of the tower.
//
// Two things need this answer: Tower.tsx, which parents the floor's contents to
// a group, and People.tsx, whose crowd is drawn by scene-wide instanced meshes
// and so cannot be parented to anything. Both read it from here, because two
// copies of a transform that animates is two copies that drift apart.
import { FLOORS, SLAB_T, floorY } from './model'
import { smoothstep } from './chapters'

const N = FLOORS.length

/** Where sheet `i` lies when fanned across the table. */
export function fanPose(i: number): { x: number; z: number; r: number } {
  const k = i - (N - 1) / 2
  return { x: k * 1.05 - 0.3, z: -k * 0.85 + 0.3, r: k * 0.09 }
}

/** How far floor `i` has risen, given the scene's overall rise. Lower floors
 *  lead, so the tower assembles from the ground up. */
export function floorRise(rise: number, i: number): number {
  return smoothstep(Math.min(1, Math.max(0, rise * 1.6 - i * 0.11)))
}

export interface FloorPlace {
  /** This floor's own rise, 0 (flat sheet) … 1 (in place). */
  rise: number
  /** How far the room contents have grown up out of the slab. */
  fill: number
  x: number
  y: number
  z: number
  ry: number
}

export function makeFloorPlace(): FloorPlace {
  return { rise: 0, fill: 0, x: 0, y: 0, z: 0, ry: 0 }
}

/** Place floor `i` for the given scene rise and fan spread. */
export function placeFloor(i: number, rise: number, spread: number, out: FloorPlace): FloorPlace {
  const r = floorRise(rise, i)
  const flat = (1 - r) * spread
  const pose = fanPose(i)
  out.rise = r
  out.fill = smoothstep((r - 0.35) / 0.65)
  out.x = pose.x * flat
  out.z = pose.z * flat
  out.y = (0.012 + i * 0.014) * (1 - r) + floorY(i) * r
  out.ry = pose.r * flat
  return out
}

/**
 * A point given in floor coordinates — x/z across the plate, y above the slab
 * top — in world space, following the floor through the fan and the rise.
 * `fill` scales y because the room contents grow out of the slab.
 */
export function floorToWorld(
  p: FloorPlace,
  x: number,
  y: number,
  z: number,
  out: { x: number; y: number; z: number }
): void {
  const c = Math.cos(p.ry)
  const s = Math.sin(p.ry)
  out.x = p.x + x * c + z * s
  out.y = p.y + SLAB_T + y * p.fill
  out.z = p.z - x * s + z * c
}
