// One palette for the whole city. The research is blunt about this: when the
// form is procedural, colour is the only channel left, and a tight palette is
// what makes several hundred generated pieces read as one place. So the walls
// come from the building page's archetype palettes, and everything this file
// adds — ground, roads, props, accents — is drawn from the same cream-and-ink
// family the dollhouse already uses.

/** Sky and fog are the same colour, so the city dissolves at its own edge. */
export const SKY = { day: '#dfe7f0', night: '#161d33' }
export const SUN = { day: '#fff2dc', night: '#8fa4e8' }
export const GROUND_TINT = { day: '#ffffff', night: '#6d7ba8' }

export const GROUND = {
  /** Open ground between the blocks. */
  grass: '#d9dcc9',
  /** The paved sheet every block sits on. */
  earth: '#e7e2d6',
  plaza: '#ded7c6',
  park: '#c3d1ab',
  water: '#aac4cf',
  dirt: '#cdc0a9',
  lot: '#e3ded1',
  lotLine: '#b7ae9c',
  shadow: 'rgba(38, 39, 44, 0.26)',
}

/** Carriageway, kerb and markings per tier. Wider and lighter as it climbs. */
export const ROADS = [
  { surface: '#b9b3a6', kerb: '#cfc9bb', mark: '' },
  { surface: '#a8a396', kerb: '#d3cdbf', mark: '' },
  { surface: '#97938a', kerb: '#d8d2c4', mark: '#ece4d2' },
] as const

export const PATH_SURFACE = '#cfc7b4'

/**
 * Eight accents, one per district seed, indexed by `CityDistrict.accent`.
 * They tint kerbs, awnings and props — never a wall, which is what keeps the
 * street reading as a street rather than as a legend.
 */
export const ACCENTS = [
  '#c9456d',
  '#d98a3c',
  '#9b7ae8',
  '#2d8a5e',
  '#6b8aed',
  '#4fa8a0',
  '#b8860b',
  '#8a8272',
]

export const accent = (i: number): string => ACCENTS[((i % ACCENTS.length) + ACCENTS.length) % ACCENTS.length]

/** Neutral massing for a lot no listing has taken. Four tones, hashed. */
export const FILLER_WALLS = ['#ddd6c8', '#d2ccbe', '#c8c3b6', '#e2dccf']

export const PROPS = {
  trunk: '#8c7a5f',
  leaf: '#8fae72',
  leafAlt: '#7d9e66',
  metal: '#8f949c',
  fence: '#b6a98f',
  board: '#ece5d6',
  glass: '#b8cddc',
  beacon: '#c9456d',
}

/** Cars take the building page's own traveller and room colours. */
export const CAR_COLORS = ['#d4577c', '#3fa477', '#d9a52a', '#6b8aed', '#e9e2d6', '#8a8272']
export const PEDESTRIAN_COLOR = '#f2ece3'

export const WINDOW = {
  glass: '#8fa9c4',
  lit: '#ffd58a',
  litEmissive: '#ffc36a',
}
