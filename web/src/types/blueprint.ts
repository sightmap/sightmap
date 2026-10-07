// A listing's building, derived from its scan and review. Pure data, no
// three.js import, so the build can write it as JSON and the prerender can
// read counts from it without loading WebGL.
//
// The metaphor is the one the building page already draws: a page is a
// floor, a tool is a room, a suggested journey is a walk. Everything here is
// a deterministic function of the listing, seeded from its slug, so the same
// listing renders the same building everywhere and a rescan changes the
// floors but never the lot.

/** Room kinds are the building page's own vocabulary, so its materials apply unchanged. */
export type BlueprintKind = 'nav' | 'form' | 'content' | 'action' | 'data'

/** The persona a category maps to. Each has three variants for variety. */
export type Archetype =
  | 'office' // fallback
  | 'storefront' // commerce, marketplaces
  | 'bank' // finance
  | 'workshop' // devtools, infrastructure
  | 'theatre' // media, entertainment
  | 'terminal' // travel, logistics
  | 'clinic' // health
  | 'library' // data, reference, productivity

export type RoofStyle = 'flat' | 'gable' | 'hip' | 'mansard' | 'sawtooth' | 'dome' | 'parapet'

export interface BlueprintRoom {
  /** The tool's name, shown as the room label. */
  name: string
  x: number
  z: number
  w: number
  d: number
  h: number
  kind: BlueprintKind
  /** Number of declared input parameters (0 when the schema declares none). */
  params: number
}

export interface BlueprintFloor {
  /** Page title, or the path when the page had none. */
  name: string
  /** Page path. */
  route: string
  rooms: BlueprintRoom[]
  /** Axis-aligned circulation polylines; first vertex of the first lane is the core door. */
  lanes: [number, number][][]
}

export interface BlueprintWalk {
  name: string
  who: 'agent' | 'test'
  /** [floor index, room name] pairs, in visit order. */
  stops: [number, string][]
  /** Seconds before the first departure, so walks are staggered. */
  delay: number
}

export interface BlueprintFacade {
  archetype: Archetype
  /** 0, 1 or 2: which of the archetype's three looks this building wears. */
  variant: number
  roof: RoofStyle
  /** Index into the city palette for this archetype. */
  palette: number
  /** Text on the sign: the listing name. */
  sign: string
  /** The one visible mark for a site whose tools were compiled with Sightkick. */
  sightkick: boolean
}

export interface Blueprint {
  v: 1
  slug: string
  name: string
  host: string
  category: string
  /** FNV-1a of the slug; the only source of variation. */
  seed: number
  floors: BlueprintFloor[]
  walks: BlueprintWalk[]
  facade: BlueprintFacade
  stats: {
    pages: number
    tools: number
    kinds: Record<BlueprintKind, number>
  }
}
