// The fictional app the building is modelled on: a flight-booking site, the
// same running example the homepage's spec section uses. Everything the scene
// draws — floors, rooms, risers, journeys — derives from this one corpus so
// the metaphor stays literal: a view is a floor, a component is a room, a
// request is a riser in the service core, a journey is a walk through them.
//
// Units are metres-ish world units, +Y up. The camera sits at +X +Z looking
// toward the origin, so the -X and -Z sides are the building's back walls and
// the +X +Z faces are the open "dollhouse" cut. Screen-right is (+X, -Z).
//
// Pure data — no three.js import — so scripts/prerender.tsx can pull the
// chapter copy that depends on these counts without loading WebGL code.

export type Kind = 'nav' | 'form' | 'content' | 'action' | 'data'

export interface Block {
  x: number
  z: number
  w: number
  d: number
}

export interface Room {
  name: string
  x: number
  z: number
  w: number
  d: number
  h: number
  kind: Kind
  /** Height of whatever this room sits on (a parent platform). */
  base?: number
  /** Several tiles that together are one component (a card list, a grid). */
  blocks?: Block[]
  /** Show a wayfinding tag over this room in the wayfinding chapter. */
  tag?: boolean
  /** A memory note pinned to the room in the wayfinding chapter. */
  memory?: string
  /** Where the room moves to in the self-healing chapter. */
  alt?: { x: number; z: number }
  /** Override the walker's stand [x, z] when the geometric front is blocked. */
  stand?: [number, number]
}

export interface Floor {
  name: string
  route: string
  source: string
  rooms: Room[]
}

export interface Riser {
  name: string
  method: 'GET' | 'POST'
  route: string
  /** Floor indices this request is called from. */
  floors: number[]
  color: string
}

export type Traveller = 'user' | 'agent' | 'test'

export interface Journey {
  name: string
  who: Traveller
  /** [floor index, room name] pairs, in visit order. */
  stops: [number, string][]
  /** Seconds before the first departure, so the crowd is staggered. */
  delay: number
}

export const FLOOR_W = 10
export const FLOOR_D = 7.5
export const FLOOR_H = 2.2
export const SLAB_T = 0.18
export const WALL_T = 0.12
/** Thickness of a component's floor zone (its carpet). */
export const PLATE = 0.07
/** Height of the kiosk that stands for an action component. */
export const KIOSK_H = 1.0

/** Service core: elevator shaft + risers, in the back corner. */
export const CORE = { x: -3.9, z: -2.9, w: 1.6, d: 1.5 }
/** Where a walker steps out of the core onto a floor. */
export const CORE_DOOR = { x: -2.65, z: -2.5 }
/** Open +Z dollhouse face — the shared front aisle on every floor. */
export const AISLE_Z = FLOOR_D / 2 - 0.3

/**
 * Per-floor circulation: axis-aligned polylines. Walkers project onto the
 * nearest lane and route along the graph rather than cutting through rooms.
 * First vertex of the trunk is CORE_DOOR so elevator hops connect cleanly.
 */
export const LANES: [number, number][][][] = [
  [
    [[-2.65, -2.5], [-1.9, -2.5], [-1.9, 0.85], [-1.9, 3.45]],
    [[-4.4, 0.85], [4.4, 0.85]],
    [[-4.4, 3.45], [4.4, 3.45]],
  ],
  [
    [[-2.65, -2.5], [-2.85, -2.5], [-2.85, 0.3], [-0.4, 0.3], [-0.4, 3.45]],
    [[-4.4, 0.3], [4.4, 0.3]],
    [[-4.4, 3.45], [4.4, 3.45]],
  ],
  [
    [[-2.65, -2.5], [-2.75, -2.5], [-2.75, 3.45]],
    [[-2.75, -2.5], [3.0, -2.5]],
    [[-2.75, 1.24], [4.4, 1.24]],
    [[-4.4, 3.45], [4.4, 3.45]],
  ],
  [
    [[-2.65, -2.5], [-2.2, -2.5], [-2.2, 1.35], [3.0, 1.35], [3.0, 3.45]],
    [[-4.4, 1.35], [4.4, 1.35]],
    [[-4.4, 3.45], [4.4, 3.45]],
  ],
  [
    [[-2.65, -2.5], [-2.3, -2.5], [-2.3, 0.75], [1.7, 0.75], [1.7, 3.45]],
    [[-3.0, 0.75], [4.4, 0.75]],
    [[-4.4, 3.45], [4.4, 3.45]],
  ],
  [
    [[-2.65, -2.5], [3.7, -2.5], [3.7, 3.45]],
    [[-4.4, 0.85], [0.12, 0.85], [0.12, 3.45]],
    [[-4.4, 3.45], [4.4, 3.45]],
  ],
]

export const TABLE = { w: 17.5, d: 14.5, t: 0.36 }
export const SHEET = { w: 10.9, d: 8.4 }

export const FLOORS: Floor[] = [
  {
    name: 'Home',
    route: '/',
    source: 'src/pages/Home.tsx',
    rooms: [
      { name: 'SiteHeader', x: 0.9, z: -3.15, w: 7.6, d: 0.7, h: 0.45, kind: 'nav' },
      { name: 'HeroSearch', x: 0.4, z: -1.0, w: 4.8, d: 2.2, h: 0.75, kind: 'form', tag: true },
      { name: 'PromoStrip', x: -3.6, z: -0.3, w: 1.8, d: 1.6, h: 0.5, kind: 'content' },
      {
        name: 'DealsGrid',
        x: -0.9,
        z: 2.1,
        w: 5.7,
        d: 1.7,
        h: 0.5,
        kind: 'content',
        blocks: [
          { x: -2.9, z: 2.1, w: 1.7, d: 1.7 },
          { x: -0.9, z: 2.1, w: 1.7, d: 1.7 },
          { x: 1.1, z: 2.1, w: 1.7, d: 1.7 },
        ],
      },
      { name: 'FooterNav', x: 3.6, z: 2.3, w: 2.2, d: 1.5, h: 0.4, kind: 'nav' },
    ],
  },
  {
    name: 'FlightSearch',
    route: '/search',
    source: 'src/pages/FlightSearch.tsx',
    rooms: [
      { name: 'FlightSearchForm', x: 0.7, z: -1.6, w: 7.6, d: 2.7, h: 0.25, kind: 'form' },
      {
        name: 'DepartureDatePicker',
        x: -1.5,
        z: -1.6,
        w: 1.9,
        d: 1.5,
        h: 0.9,
        kind: 'form',
        base: 0.25,
        tag: true,
        memory: 'Accepts typed YYYY-MM-DD, skips the calendar',
      },
      { name: 'PassengerPicker', x: 0.7, z: -1.6, w: 1.7, d: 1.5, h: 0.8, kind: 'form', base: 0.25 },
      { name: 'SearchButton', x: 3.1, z: -1.6, w: 1.5, d: 1.1, h: 0.8, kind: 'action', base: 0.25, tag: true },
      { name: 'RecentSearches', x: -2.6, z: 1.9, w: 3.8, d: 2.2, h: 0.55, kind: 'content', tag: true },
      { name: 'PromoBanner', x: 2.2, z: 2.1, w: 4.6, d: 1.6, h: 0.45, kind: 'content' },
    ],
  },
  {
    name: 'Results',
    route: '/results',
    source: 'src/pages/Results.tsx',
    rooms: [
      { name: 'FilterRail', x: -3.9, z: 0.7, w: 1.6, d: 5.4, h: 0.85, kind: 'form' },
      { name: 'SortMenu', x: 1.0, z: -3.0, w: 4.0, d: 0.8, h: 0.5, kind: 'nav' },
      { name: 'ResultsList', x: 1.0, z: 0.4, w: 6.8, d: 5.2, h: 0.2, kind: 'content' },
      {
        name: 'FareCard',
        x: 0.6,
        z: 0.4,
        w: 5.2,
        d: 4.4,
        h: 0.6,
        kind: 'content',
        base: 0.2,
        stand: [0.6, 1.24],
        blocks: [
          { x: 0.6, z: -1.2, w: 5.2, d: 1.25 },
          { x: 0.6, z: 0.4, w: 5.2, d: 1.25 },
          { x: 0.6, z: 2.0, w: 5.2, d: 1.25 },
        ],
      },
      { name: 'SelectFareButton', x: 3.7, z: 0.4, w: 1.0, d: 0.8, h: 0.85, kind: 'action', base: 0.2 },
    ],
  },
  {
    name: 'Checkout',
    route: '/checkout',
    source: 'src/pages/Checkout.tsx',
    rooms: [
      { name: 'PassengerForm', x: -2.2, z: -0.5, w: 4.6, d: 3.0, h: 0.75, kind: 'form' },
      { name: 'PaymentForm', x: 2.4, z: -1.3, w: 4.4, d: 3.4, h: 0.8, kind: 'form', tag: true },
      { name: 'CardFieldset', x: 2.4, z: -1.6, w: 3.4, d: 1.4, h: 0.9, kind: 'form', base: 0.8 },
      { name: 'PromoCode', x: -2.6, z: 2.4, w: 3.6, d: 1.6, h: 0.55, kind: 'content' },
      { name: 'OrderSummary', x: 1.1, z: 2.4, w: 3.4, d: 1.7, h: 0.65, kind: 'data' },
      {
        name: 'ContinueButton',
        x: 3.9,
        z: 2.5,
        w: 1.4,
        d: 1.0,
        h: 0.95,
        kind: 'action',
        tag: true,
        memory: 'Stays disabled until the CVC validates',
        alt: { x: 3.9, z: 0.9 },
      },
    ],
  },
  {
    name: 'BookingConfirmation',
    route: '/bookings/*',
    source: 'src/pages/Booking.tsx',
    rooms: [
      { name: 'ConfirmationCard', x: 0.6, z: -1.2, w: 6.6, d: 2.9, h: 0.85, kind: 'data' },
      { name: 'HelpLink', x: -3.9, z: 0.4, w: 1.4, d: 1.6, h: 0.45, kind: 'nav' },
      { name: 'Itinerary', x: -1.3, z: 2.2, w: 5.2, d: 2.0, h: 0.6, kind: 'content' },
      { name: 'ShareButtons', x: 3.4, z: 2.3, w: 2.6, d: 1.5, h: 0.7, kind: 'action' },
    ],
  },
  {
    name: 'Account',
    route: '/account',
    source: 'src/pages/Account.tsx',
    rooms: [
      { name: 'ProfileCard', x: -2.2, z: -0.8, w: 4.4, d: 2.6, h: 0.75, kind: 'content' },
      { name: 'TripsList', x: 2.4, z: -0.2, w: 4.4, d: 5.8, h: 0.55, kind: 'data' },
      { name: 'Preferences', x: -2.2, z: 2.3, w: 4.4, d: 2.2, h: 0.65, kind: 'form' },
    ],
  },
]

export const RISERS: Riser[] = [
  { name: 'SearchFlights', method: 'POST', route: '/api/flights/search', floors: [1, 2], color: '#6b8aed' },
  { name: 'FareRules', method: 'GET', route: '/api/fares/:id/rules', floors: [2], color: '#9b7ae8' },
  { name: 'CreateBooking', method: 'POST', route: '/api/bookings', floors: [3], color: '#e26a8d' },
  { name: 'GetBooking', method: 'GET', route: '/api/bookings/:id', floors: [4], color: '#4aa87b' },
  { name: 'GetTrips', method: 'GET', route: '/api/trips', floors: [5], color: '#d9a52a' },
]

export const JOURNEYS: Journey[] = [
  {
    name: 'BookFlight',
    who: 'user',
    delay: 0,
    stops: [
      [0, 'HeroSearch'],
      [1, 'DepartureDatePicker'],
      [1, 'SearchButton'],
      [2, 'SelectFareButton'],
      [3, 'PaymentForm'],
      [3, 'ContinueButton'],
      [4, 'ConfirmationCard'],
    ],
  },
  {
    name: 'BrowseDeals',
    who: 'user',
    delay: 3.5,
    stops: [
      [0, 'DealsGrid'],
      [0, 'HeroSearch'],
      [1, 'FlightSearchForm'],
      [2, 'FilterRail'],
      [2, 'FareCard'],
    ],
  },
  {
    name: 'ManageTrips',
    who: 'user',
    delay: 7,
    stops: [
      [0, 'SiteHeader'],
      [5, 'TripsList'],
      [4, 'Itinerary'],
      [5, 'Preferences'],
    ],
  },
  {
    name: 'VerifyFix',
    who: 'agent',
    delay: 1.5,
    stops: [
      [1, 'DepartureDatePicker'],
      [1, 'SearchButton'],
      [2, 'ResultsList'],
      [2, 'SortMenu'],
    ],
  },
  {
    name: 'ReproBug',
    who: 'agent',
    delay: 5,
    stops: [
      [3, 'PromoCode'],
      [3, 'OrderSummary'],
      [3, 'ContinueButton'],
      [3, 'PaymentForm'],
    ],
  },
  {
    name: 'Regression',
    who: 'test',
    delay: 9,
    stops: [
      [0, 'HeroSearch'],
      [1, 'SearchButton'],
      [2, 'SelectFareButton'],
      [3, 'ContinueButton'],
      [4, 'ConfirmationCard'],
    ],
  },
  {
    name: 'Support',
    who: 'user',
    delay: 12,
    stops: [
      [0, 'FooterNav'],
      [4, 'ShareButtons'],
      [0, 'PromoStrip'],
    ],
  },
]

/** Tools the front desk offers in the Web MCP chapter, each backed by the map. */
export const TOOLS = [
  { name: 'search_flights', args: 'origin, destination, date', via: 'FlightSearch · POST /api/flights/search' },
  { name: 'select_fare', args: 'fare_id', via: 'Results · FareCard' },
  { name: 'create_booking', args: 'passenger, payment', via: 'Checkout · POST /api/bookings' },
]

export const KIND_COLORS: Record<Kind, string> = {
  nav: '#a58ae8',
  form: '#8ea4ee',
  content: '#e0d3c0',
  action: '#d4577c',
  data: '#5cb08a',
}

export const TRAVELLER_COLORS: Record<Traveller, string> = {
  user: '#d4577c',
  agent: '#3fa477',
  test: '#d9a52a',
}

export const floorY = (i: number): number => i * FLOOR_H

export function findRoom(floor: number, name: string): Room {
  const room = FLOORS[floor]?.rooms.find((r) => r.name === name)
  if (!room) throw new Error(`model: no room ${name} on floor ${floor}`)
  return room
}

/** Deck height at a point: slab plus the tallest zone carpet covering it. */
export function surfaceAt(floor: number, x: number, z: number): number {
  let lift = 0
  for (const room of FLOORS[floor].rooms) {
    const blocks = room.blocks ?? [{ x: room.x, z: room.z, w: room.w, d: room.d }]
    if (!blocks.some((b) => Math.abs(x - b.x) <= b.w / 2 && Math.abs(z - b.z) <= b.d / 2)) continue
    const top = (room.base ? PLATE : 0) + PLATE
    if (top > lift) lift = top
  }
  return floorY(floor) + SLAB_T + lift
}

/** Where a walker stands when visiting a room: just outside the front of
 *  its zone, on whatever carpet is actually underfoot. */
export function roomStand(floor: number, room: Room, shift = 0): [number, number, number] {
  const rx = room.alt ? room.x + (room.alt.x - room.x) * shift : room.x
  const rz = room.alt ? room.z + (room.alt.z - room.z) * shift : room.z
  const x = room.stand ? room.stand[0] : rx
  const z = room.stand ? room.stand[1] : Math.min(rz + room.d / 2 + (room.kind === 'action' ? 0.35 : 0.25), AISLE_Z)
  return [x, surfaceAt(floor, x, z), z]
}

/** Top of a room's tallest element, for hanging a label over it. */
export function roomTop(room: Room): number {
  const lift = (room.base ? PLATE : 0) + PLATE
  if (room.kind === 'action') return lift + KIOSK_H
  if (!room.blocks && room.w >= 2.4 && room.d >= 1.4) return lift + 1.15
  return lift + 0.9
}

export const COUNTS = {
  views: FLOORS.length,
  components: FLOORS.reduce((n, f) => n + f.rooms.length, 0),
  requests: RISERS.length,
  memory: FLOORS.reduce((n, f) => n + f.rooms.filter((r) => r.memory).length, 0) + 5,
  journeys: JOURNEYS.length,
}
