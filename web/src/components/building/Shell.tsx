import { Html, Instance, Instances } from '@react-three/drei'
import { useMemo } from 'react'
import * as THREE from 'three'
import { FLOOR_D, FLOOR_W, SLAB_T, type Facade } from './model'
import {
  ACCENTS,
  groundFloorParts,
  paletteColor,
  roofParts,
  rooftopItem,
  shellHeight,
  windowGrid,
  windowMullions,
  type FacadePart,
  type PartTone,
} from './facade'
import RoofShape from './RoofShape'
import { useBuildingModel } from './context'

// The building with its walls on: what the city sees from the street and what
// the listing page shows before you step inside. Four walls, a seeded window
// grid, the archetype's roof and one sign — plain meshes, no textures, no
// per-frame work, so a city can afford a few hundred of them.

const WALL_T = 0.18
const unitBox = new THREE.BoxGeometry(1, 1, 1)
const cylGeom = new THREE.CylinderGeometry(0.5, 0.5, 1, 14)

const DEFAULT_FACADE: Facade = {
  archetype: 'office',
  variant: 0,
  roof: 'flat',
  palette: 0,
  sign: '',
  sightkick: false,
}

type Mats = Record<PartTone, THREE.Material> & { sign: THREE.Material }

/** Draws a part list from facade.ts. One mesh per part; the city instances. */
function Parts({ parts, mats, y = 0 }: { parts: FacadePart[]; mats: Mats; y?: number }) {
  return (
    <group position={[0, y, 0]}>
      {parts.map((part, k) =>
        part.kind === 'box' ? (
          <mesh
            key={k}
            geometry={unitBox}
            material={mats[part.tone]}
            position={[part.x, part.y, part.z]}
            rotation={part.tilt ? [part.tilt, 0, 0] : undefined}
            scale={[part.w, part.h, part.d]}
            castShadow
            receiveShadow
          />
        ) : (
          <mesh
            key={k}
            geometry={cylGeom}
            material={mats[part.tone]}
            position={[part.x, part.y, part.z]}
            rotation={part.axis === 'x' ? [0, 0, Math.PI / 2] : part.axis === 'z' ? [Math.PI / 2, 0, 0] : undefined}
            scale={[part.r * 2, part.h, part.r * 2]}
            castShadow
          />
        )
      )}
    </group>
  )
}

export default function Shell() {
  const model = useBuildingModel()
  const facade = model.facade ?? DEFAULT_FACADE
  const n = Math.max(1, model.floors.length)
  const H = shellHeight(n)

  const mats = useMemo(() => {
    const wall = new THREE.Color(paletteColor(facade.archetype, facade.palette))
    const roof = wall.clone().multiplyScalar(0.62)
    const tones: Record<PartTone, THREE.MeshStandardMaterial> = {
      wall: new THREE.MeshStandardMaterial({ color: wall, roughness: 0.95 }),
      roof: new THREE.MeshStandardMaterial({ color: roof, roughness: 0.9 }),
      trim: new THREE.MeshStandardMaterial({ color: '#f2ece3', roughness: 0.85 }),
      dark: new THREE.MeshStandardMaterial({ color: '#2b2d33', roughness: 0.7 }),
      glass: new THREE.MeshStandardMaterial({ color: '#8fa9c4', roughness: 0.25, metalness: 0.1 }),
      lit: new THREE.MeshStandardMaterial({
        color: '#ffd58a',
        emissive: new THREE.Color('#ffc36a'),
        emissiveIntensity: 0.55,
        roughness: 0.3,
      }),
      accent: new THREE.MeshStandardMaterial({ color: ACCENTS[facade.archetype] ?? ACCENTS.office, roughness: 0.8 }),
    }
    return { ...tones, sign: new THREE.MeshStandardMaterial({ color: wall.clone().multiplyScalar(0.5), roughness: 0.8 }) }
  }, [facade.archetype, facade.palette])

  const windows = useMemo(() => windowGrid(n, model.seed), [n, model.seed])
  const lit = useMemo(() => windows.filter((w) => w.lit), [windows])
  const dark = useMemo(() => windows.filter((w) => !w.lit), [windows])
  const roof = useMemo(() => roofParts(facade.roof), [facade.roof])
  const mullions = useMemo(() => windowMullions(windows), [windows])
  const street = useMemo(
    () => groundFloorParts(facade.archetype, facade.variant),
    [facade.archetype, facade.variant]
  )
  const rooftop = useMemo(
    () => rooftopItem(facade.archetype, facade.variant),
    [facade.archetype, facade.variant]
  )
  // Where a rooftop item can stand: a roof shape with a deck to stand on.
  // A dome or a row of sawteeth has none, and gets nothing.
  const deckY =
    facade.roof === 'flat' || facade.roof === 'parapet' ? 0.18 : facade.roof === 'mansard' ? 1.28 : null

  const winScale = (face: string): [number, number, number] =>
    face === 'east' || face === 'west' ? [0.1, 0.85, 0.85] : [0.85, 0.85, 0.1]

  return (
    <group>
      {/* Plinth, then the four walls. */}
      <mesh geometry={unitBox} material={mats.roof} position={[0, 0.11, 0]} scale={[FLOOR_W + 0.5, 0.22, FLOOR_D + 0.5]} receiveShadow castShadow />
      <Instances limit={5} range={5} geometry={unitBox} material={mats.wall} castShadow receiveShadow>
        <Instance position={[0, H / 2, FLOOR_D / 2 - WALL_T / 2]} scale={[FLOOR_W, H, WALL_T]} />
        <Instance position={[0, H / 2, -FLOOR_D / 2 + WALL_T / 2]} scale={[FLOOR_W, H, WALL_T]} />
        <Instance position={[FLOOR_W / 2 - WALL_T / 2, H / 2, 0]} scale={[WALL_T, H, FLOOR_D]} />
        <Instance position={[-FLOOR_W / 2 + WALL_T / 2, H / 2, 0]} scale={[WALL_T, H, FLOOR_D]} />
        {/* A cap over the top so the box reads as solid from above. */}
        <Instance position={[0, H - SLAB_T / 2, 0]} scale={[FLOOR_W, SLAB_T, FLOOR_D]} />
      </Instances>

      {dark.length > 0 && (
        <Instances limit={dark.length} range={dark.length} geometry={unitBox} material={mats.glass}>
          {dark.map((w, k) => (
            <Instance key={k} position={[w.x, w.y, w.z]} scale={winScale(w.face)} />
          ))}
        </Instances>
      )}
      {lit.length > 0 && (
        <Instances limit={lit.length} range={lit.length} geometry={unitBox} material={mats.lit}>
          {lit.map((w, k) => (
            <Instance key={k} position={[w.x, w.y, w.z]} scale={winScale(w.face)} />
          ))}
        </Instances>
      )}

      {/* The street level the archetype implies, and the mullions the window
          mask left standing. */}
      <Parts parts={mullions} mats={mats} />
      <Parts parts={street} mats={mats} />
      {deckY !== null && <Parts parts={rooftop.parts} mats={mats} y={H + deckY} />}

      {/* The sign: a board on the street face carrying the listing's name. */}
      <mesh
        geometry={unitBox}
        material={mats.sign}
        position={[0, H - 1.05, FLOOR_D / 2 + 0.06]}
        scale={[Math.min(FLOOR_W * 0.86, 1.6 + facade.sign.length * 0.4), 1.15, 0.16]}
        castShadow
      />
      {facade.sign && (
        <Html
          position={[0, H - 1.05, FLOOR_D / 2 + 0.16]}
          center
          zIndexRange={[6, 0]}
          style={{ pointerEvents: 'none' }}
          // Kept out of the accessibility tree: the sign repeats the listing
          // name, which the page's own heading already carries.
          wrapperClass="bld-sign-anchor"
        >
          <div className="bld-sign bld-sign--shell" aria-hidden="true">
            {facade.sign}
          </div>
        </Html>
      )}

      {/* One mark, beside the door, for a site whose tools ship with Sightkick. */}
      {facade.sightkick && (
        <mesh position={[-2.2, 1.5, FLOOR_D / 2 + 0.06]} rotation={[Math.PI / 2, 0, 0]} castShadow>
          <cylinderGeometry args={[0.26, 0.26, 0.12, 16]} />
          <meshStandardMaterial color="#c9456d" emissive="#c9456d" emissiveIntensity={0.5} roughness={0.4} />
        </mesh>
      )}

      <RoofShape parts={roof} y={H} mat={mats.roof} trim={mats.trim} />
    </group>
  )
}
