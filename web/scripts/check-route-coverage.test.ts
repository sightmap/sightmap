import { describe, it, expect } from 'vitest'
import { extractRoutePaths, checkCoverage } from './check-route-coverage'

const APP = `
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/blog" element={<BlogIndex />} />
        <Route path="/blog/:slug" element={<BlogPost />} />
        <Route path="/atlas" element={<AtlasIndex />} />
        <Route path="/atlas/:slug" element={<AtlasEntry />} />
        <Route path="/developers" element={<Developers />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
`

describe('extractRoutePaths', () => {
  it('pulls every declared route path', () => {
    expect(extractRoutePaths(APP)).toEqual([
      '/',
      '/blog',
      '/blog/:slug',
      '/atlas',
      '/atlas/:slug',
      '/developers',
      '*',
    ])
  })

  it('returns nothing when the markup stops matching', () => {
    // main() treats this as a build failure rather than "all covered" — an
    // empty result means the regex broke, not that the app has no routes.
    expect(extractRoutePaths('<Route path={ROUTES.home} />')).toEqual([])
  })
})

describe('checkCoverage', () => {
  const declared = extractRoutePaths(APP)

  it('passes when every route has a page', () => {
    const out = checkCoverage(declared, [
      '/',
      '/blog',
      '/blog/sightmap',
      '/atlas',
      '/atlas/airbnb',
      '/developers',
    ])
    expect(out.uncovered).toEqual([])
    expect(out.emptyDynamic).toEqual([])
  })

  it('flags a static route with no prerendered page', () => {
    const out = checkCoverage(declared, ['/', '/blog', '/blog/sightmap', '/atlas/airbnb', '/developers'])
    expect(out.uncovered).toEqual(['/atlas'])
  })

  it('never flags the catch-all, which dist/404.html answers', () => {
    const out = checkCoverage(['*'], [])
    expect(out.uncovered).toEqual([])
  })

  it('reports a dynamic route with no instances without failing the build', () => {
    // A production build excludes drafts, so a blog with nothing published
    // legitimately has zero /blog/<slug> pages — and those URLs *should* 404.
    const out = checkCoverage(declared, ['/', '/blog', '/atlas', '/atlas/airbnb', '/developers'])
    expect(out.uncovered).toEqual([])
    expect(out.emptyDynamic).toEqual(['/blog/:slug'])
  })

  it('does not count the index page as an instance of its dynamic child', () => {
    const out = checkCoverage(['/blog', '/blog/:slug'], ['/blog'])
    expect(out.uncovered).toEqual([])
    expect(out.emptyDynamic).toEqual(['/blog/:slug'])
  })
})
