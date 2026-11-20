// Every sign in the city on one texture. A building's name is a quad that
// samples its own region of a single canvas, so a street of signs costs one
// draw call and one texture rather than a DOM node per building — which is
// the whole reason the city can draw what the listing page draws with an
// HTML label.
export interface SignItem {
  text: string
  /** Measured width and height of the drawn plate, in texels. */
  width: number
  height: number
}

export interface SignRegion extends SignItem {
  x: number
  y: number
  /** The region in texture coordinates, ready for an instance attribute. */
  u: number
  v: number
  uw: number
  vh: number
}

export interface SignAtlas {
  width: number
  height: number
  regions: SignRegion[]
  /** Region index by sign text, for the buildings that share a name. */
  index: Map<string, number>
}

export interface PackOptions {
  /** Texture width; shelves wrap at it. */
  width?: number
  /** Texels of air around each plate, so filtering cannot bleed a neighbour. */
  pad?: number
  /** Textures stay powers of two; a taller pack than this is refused a shelf. */
  maxHeight?: number
}

const nextPow2 = (n: number): number => {
  let p = 1
  while (p < n) p *= 2
  return p
}

/**
 * Shelf-packs the signs in the order given. Order is stable on purpose: a
 * listing added today must not move another listing's sign, for the same
 * reason it must not move its lot.
 */
export function packSigns(items: SignItem[], opts: PackOptions = {}): SignAtlas {
  const width = opts.width ?? 1024
  const pad = opts.pad ?? 2
  const maxHeight = opts.maxHeight ?? 2048
  const regions: SignRegion[] = []
  const index = new Map<string, number>()
  let x = pad
  let y = pad
  let shelf = 0
  let used = pad

  for (const item of items) {
    const w = Math.min(Math.ceil(item.width), width - pad * 2)
    const h = Math.ceil(item.height)
    if (x + w + pad > width) {
      x = pad
      y += shelf + pad
      shelf = 0
    }
    if (y + h + pad > maxHeight) break
    const region: SignRegion = {
      text: item.text,
      width: w,
      height: h,
      x,
      y,
      u: 0,
      v: 0,
      uw: 0,
      vh: 0,
    }
    regions.push(region)
    if (!index.has(item.text)) index.set(item.text, regions.length - 1)
    x += w + pad
    if (h > shelf) shelf = h
    used = Math.max(used, y + h + pad)
  }

  const height = nextPow2(Math.max(used, 8))
  for (const r of regions) {
    r.u = r.x / width
    // Canvas y runs down and the texture is sampled with the default flip, so
    // a region's v is measured from the bottom of the image.
    r.v = 1 - (r.y + r.height) / height
    r.uw = r.width / width
    r.vh = r.height / height
  }
  return { width, height, regions, index }
}

/** The part of a 2D context the atlas draws with; a stub implements it. */
export interface SignContext {
  fillStyle: CanvasRenderingContext2D['fillStyle']
  font: string
  textAlign: CanvasTextAlign
  textBaseline: CanvasTextBaseline
  clearRect(x: number, y: number, w: number, h: number): void
  fillRect(x: number, y: number, w: number, h: number): void
  fillText(text: string, x: number, y: number): void
}

export interface SignStyle {
  font: string
  plate: string
  ink: string
}

export function drawSignAtlas(ctx: SignContext, atlas: SignAtlas, style: SignStyle): void {
  ctx.clearRect(0, 0, atlas.width, atlas.height)
  ctx.font = style.font
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  for (const r of atlas.regions) {
    ctx.fillStyle = style.plate
    ctx.fillRect(r.x, r.y, r.width, r.height)
    ctx.fillStyle = style.ink
    ctx.fillText(r.text, r.x + r.width / 2, r.y + r.height / 2 + 1)
  }
}
