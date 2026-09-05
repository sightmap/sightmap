// The story, one chapter per scroll section, and the scene parameters each
// chapter drives toward. Scene.tsx blends neighbouring chapters by scroll
// position and damps toward the result, so every number here is a target, not
// a keyframe. Copy uses `backticks` for inline code; the card renders them.
import { COUNTS, JOURNEYS, RISERS, TOOLS } from './model'

export interface SceneParams {
  /** Blueprint sheets fanned across the table (1) vs stacked (0). */
  spread: number
  /** How much of the white line work is drawn. */
  draw: number
  /** Sheets lifted into floors and rooms extruded. */
  rise: number
  /** Back walls, windows and roof. */
  walls: number
  /** Floor directory labels. */
  labels: number
  /** Room tags and memory notes. */
  tags: number
  /** Crowd presence (scale of walkers). */
  agents: number
  /** Self-healing demo running. */
  heal: number
  /** Trajectory highlight. */
  trajectory: number
  /** Front desk with MCP tools. */
  desk: number
  /** Nightfall. */
  night: number
  /** Camera azimuth (deg), elevation (deg), zoom multiplier, look-at height. */
  az: number
  el: number
  zoom: number
  lookY: number
}

export interface Chapter {
  id: string
  eyebrow: string
  /** Rail label. */
  short: string
  title: string
  body: string
  badge?: string
  ctas?: { label: string; href: string; primary?: boolean; external?: boolean }[]
  hud: { title: string; rows: [string, string][] }
  /** Which journey the crowd is reduced to, if any. */
  focus?: string
  scene: SceneParams
}

const base: SceneParams = {
  spread: 0,
  draw: 1,
  rise: 1,
  walls: 1,
  labels: 0,
  tags: 0,
  agents: 0,
  heal: 0,
  trajectory: 0,
  desk: 0,
  night: 0,
  az: 42,
  el: 27,
  zoom: 1,
  lookY: 6.2,
}

export const CHAPTERS: Chapter[] = [
  {
    id: 'arrival',
    short: 'Arrival',
    eyebrow: '00 · Arrival',
    title: 'Every app is a building.',
    body:
      'The code is the blueprint. The running app is the building. The people inside are your users and your agents. Sightmap is the wayfinding that lets an agent find its way around without reading the blueprints first.',
    hud: {
      title: 'The model',
      rows: [
        ['Views', String(COUNTS.views)],
        ['Components', String(COUNTS.components)],
        ['Requests', String(COUNTS.requests)],
      ],
    },
    scene: { ...base, spread: 1, draw: 1, rise: 0, walls: 0, az: 38, el: 46, zoom: 1.05, lookY: 0.4 },
  },
  {
    id: 'blueprint',
    short: 'Blueprint',
    eyebrow: '01 · The blueprint',
    title: 'Code describes every wall.',
    body:
      'Source files are the drawings the building was built from: components, routes, API handlers. They are complete and exact, and almost useless to someone standing in the lobby. Nothing on a blueprint says where the front desk is or which door sticks.',
    hud: {
      title: 'The drawing set',
      rows: [
        ['Sheets', String(COUNTS.views)],
        ['Source files', '41'],
        ['Says where the desk is', 'no'],
      ],
    },
    scene: { ...base, spread: 0, draw: 1, rise: 0, walls: 0, az: 34, el: 54, zoom: 1.3, lookY: 0.3 },
  },
  {
    id: 'building',
    short: 'Building',
    eyebrow: '02 · The building',
    title: 'The running app is the building.',
    body:
      'Deploy the code and the drawings become floors. Each view is a floor with its own route. Components are the rooms on that floor. API requests are the service risers running up the core, carrying data between the floors and the basement.',
    hud: {
      title: 'The building',
      rows: [
        ['Floors', String(COUNTS.views)],
        ['Rooms', String(COUNTS.components)],
        ['Risers', String(RISERS.length)],
      ],
    },
    scene: { ...base, rise: 1, walls: 1, az: 42, el: 27, zoom: 1.0, lookY: 6.2 },
  },
  {
    id: 'wayfinding',
    short: 'Wayfinding',
    eyebrow: '03 · The wayfinding',
    title: 'A sightmap is the signage.',
    body:
      'Real buildings do not hand visitors blueprints. They put a directory in the lobby, numbers on the doors, and a note on the door that sticks. A `.sightmap/` does the same for agents: names for every floor, room and riser, links back to the drawings, and memory for the quirks the drawings never recorded.',
    hud: {
      title: 'The map',
      rows: [
        ['Named', `${COUNTS.components}/${COUNTS.components}`],
        ['Orphans', '0'],
        ['Memory notes', String(COUNTS.memory)],
      ],
    },
    scene: { ...base, labels: 1, tags: 1, az: 46, el: 24, zoom: 1.02, lookY: 6.6 },
  },
  {
    id: 'people',
    short: 'People',
    eyebrow: '04 · The people',
    title: 'Every day, people move through it.',
    body:
      'Users book a flight. Agents reproduce a bug, verify a fix, run a checkout. Each trip is a journey through rooms and floors, and Subtext records those journeys against the map, so a session reads as `FlightSearch → FareCard → PaymentForm` instead of a list of divs.',
    hud: {
      title: 'Today',
      rows: [
        ['Journeys', String(JOURNEYS.length)],
        ['Steps named', '100%'],
        ['Floors visited', String(COUNTS.views)],
      ],
    },
    scene: { ...base, labels: 0.55, agents: 1, az: 40, el: 26, zoom: 1.02, lookY: 6.0 },
  },
  {
    id: 'self-healing',
    short: 'Self-healing',
    eyebrow: '05 · Built on top',
    badge: 'exploratory',
    title: 'Tests that find the moved door.',
    body:
      'When a room moves, a test written against selectors walks into a wall. A test written against the map asks the building where the room went. The selector changes, the name does not, and the run finishes.',
    hud: {
      title: 'Self-healing run',
      rows: [
        ['Room moved', 'ContinueButton'],
        ['Selectors updated', '1'],
        ['Runs passed', '128 / 128'],
      ],
    },
    scene: { ...base, labels: 0.4, agents: 0, heal: 1, az: 50, el: 22, zoom: 1.22, lookY: 8.2 },
  },
  {
    id: 'trajectories',
    short: 'Trajectories',
    eyebrow: '06 · Built on top',
    badge: 'exploratory',
    title: 'Journeys you can name and rerun.',
    body:
      'Codify a journey on top of the primitives: the floors it visits, the rooms it touches, the requests it expects on the way. A trajectory is a route through the building any agent can follow, and any run can be checked against.',
    hud: {
      title: 'Trajectory',
      rows: [
        ['Route', 'BookFlight'],
        ['Stops', String(JOURNEYS[0].stops.length)],
        ['Requests expected', '3'],
      ],
    },
    focus: 'BookFlight',
    scene: { ...base, labels: 0.5, agents: 0.6, trajectory: 1, az: 44, el: 26, zoom: 1.05, lookY: 5.8 },
  },
  {
    id: 'web-mcp',
    short: 'Web MCP',
    eyebrow: '07 · Sightkick',
    title: 'A front desk for agents.',
    body:
      'Once the building knows its own rooms, it can offer services at the door. Sightkick compiles the map into WebMCP tools — `search_flights`, `select_fare`, `create_booking` — each backed by a real view and a real request. An agent walks up to the counter and asks, instead of wandering the halls.',
    ctas: [{ label: 'Meet sightkick →', href: '/sightkick' }],
    hud: {
      title: 'Front desk',
      rows: [
        ['Tools', String(TOOLS.length)],
        ['Backed by views', '3'],
        ['Backed by requests', '2'],
      ],
    },
    scene: { ...base, labels: 0.3, agents: 0.5, desk: 1, az: 52, el: 21, zoom: 1.2, lookY: 3.4 },
  },
  {
    id: 'nightfall',
    short: 'Nightfall',
    eyebrow: '08 · Nightfall',
    title: 'Hand your agent the map.',
    body:
      'A sightmap is YAML in your repo. Install the CLI, let an agent walk the building, and commit the wayfinding alongside the code.',
    ctas: [
      { label: 'Get started', href: '/#start', primary: true },
      { label: 'Read the docs →', href: 'https://docs.sightmap.org', external: true },
      { label: 'View on GitHub', href: 'https://github.com/sightmap/sightmap', external: true },
    ],
    hud: {
      title: 'Nightfall',
      rows: [
        ['Windows lit', String(COUNTS.components)],
        ['Still walking', String(JOURNEYS.length)],
        ['Map committed', 'yes'],
      ],
    },
    scene: { ...base, labels: 0.5, agents: 1, night: 1, az: 36, el: 25, zoom: 0.98, lookY: 6.4 },
  },
]

export const PARAM_KEYS = Object.keys(base) as (keyof SceneParams)[]

export const smoothstep = (t: number): number => {
  const x = Math.min(1, Math.max(0, t))
  return x * x * (3 - 2 * x)
}

/** Scene target for a continuous chapter position (0 … CHAPTERS.length-1). */
export function paramsAt(p: number, out: SceneParams): SceneParams {
  const n = CHAPTERS.length
  const c = Math.min(Math.max(p, 0), n - 1)
  const i = Math.min(Math.floor(c), n - 2)
  const t = smoothstep(c - i)
  const a = CHAPTERS[i].scene
  const b = CHAPTERS[i + 1].scene
  for (const k of PARAM_KEYS) out[k] = a[k] + (b[k] - a[k]) * t
  return out
}

export const defaultParams = (): SceneParams => ({ ...CHAPTERS[0].scene })
