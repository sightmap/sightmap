// A blueprint by hand, for the tests around here and for anything that needs
// a second building without a scan behind it. Deliberately small: two floors,
// one of them an annex, every room kind, two walks and a facade.
import type { Blueprint } from '@/types/blueprint'

export const FIXTURE_BLUEPRINT: Blueprint = {
  v: 1,
  slug: 'fixture-shop',
  name: 'Fixture Shop',
  host: 'fixture.example',
  category: 'shopping',
  seed: 0x1f2e3d4c,
  floors: [
    {
      name: 'Home',
      route: '/',
      rooms: [
        { name: 'browse_catalog', x: -2.4, z: -2.2, w: 2.3, d: 1.7, h: 0.5, kind: 'content', params: 2 },
        { name: 'search_products', x: 0.6, z: -2.2, w: 2.65, d: 1.95, h: 0.75, kind: 'form', params: 3 },
        { name: 'open_help', x: 3.2, z: -2.2, w: 1.6, d: 1.2, h: 0.45, kind: 'nav', params: 0 },
      ],
      lanes: [
        [
          [-2.65, -2.5],
          [-1.9, -2.5],
          [-1.9, 3.45],
        ],
        [
          [-4.4, 3.45],
          [4.4, 3.45],
        ],
      ],
    },
    {
      name: 'Checkout',
      route: '/checkout',
      rooms: [
        { name: 'add_to_cart', x: -1.8, z: -2.0, w: 2.65, d: 1.95, h: 1.0, kind: 'action', params: 3 },
        { name: 'read_order', x: 1.9, z: -2.0, w: 2.3, d: 1.7, h: 0.65, kind: 'data', params: 2 },
      ],
      lanes: [
        [
          [-2.65, -2.5],
          [-2.2, -2.5],
          [-2.2, 3.45],
        ],
        [
          [-4.4, 3.45],
          [4.4, 3.45],
        ],
      ],
    },
  ],
  walks: [
    {
      name: 'Find and buy a product',
      who: 'agent',
      stops: [
        [0, 'search_products'],
        [0, 'browse_catalog'],
        [1, 'add_to_cart'],
      ],
      delay: 0,
    },
    {
      name: 'Check an order',
      who: 'test',
      stops: [
        [1, 'read_order'],
        [0, 'open_help'],
      ],
      delay: 2,
    },
  ],
  facade: {
    archetype: 'storefront',
    variant: 1,
    roof: 'gable',
    palette: 2,
    sign: 'Fixture Shop',
    sightkick: true,
  },
  stats: {
    pages: 2,
    tools: 5,
    kinds: { nav: 1, form: 1, content: 1, action: 1, data: 1 },
  },
}
