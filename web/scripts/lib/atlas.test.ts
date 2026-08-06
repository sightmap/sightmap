import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  MAX_CORPUS_FILE_BYTES,
  isLocalUrl,
  isSafeUrl,
  loadAtlas,
  renderAtlasBody,
  resolveCorpus,
  resolveScreenshots,
} from './atlas'

const FIXTURES = path.resolve(__dirname, '__fixtures__/atlas')

// loadAtlas() reports skipped entries through console.warn rather than
// throwing (see the header comment on scripts/lib/atlas.ts), so the tests that
// exercise that path silence it and assert on the calls instead.
let warn: ReturnType<typeof vi.spyOn>
beforeEach(() => {
  warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
})
afterEach(() => {
  warn.mockRestore()
})

describe('renderAtlasBody', () => {
  // Atlas READMEs are community-authored. Every case below is markup that
  // would execute or phone home if it reached the page as HTML.
  it('renders a raw <script> as text, not as markup', async () => {
    const html = await renderAtlasBody('<script>alert(1)</script>')
    expect(html).not.toContain('<script>')
    expect(html).toContain('&lt;script&gt;')
  })

  it('renders raw inline HTML as text', async () => {
    const html = await renderAtlasBody('Hello <img src=x onerror=alert(1)> world')
    expect(html).not.toMatch(/<img[^>]*onerror/)
    expect(html).toContain('&lt;img src=x onerror=alert(1)&gt;')
  })

  it('drops a javascript: link, keeping its text', async () => {
    const html = await renderAtlasBody('[click me](javascript:alert(1))')
    expect(html).not.toContain('javascript:')
    expect(html).not.toContain('<a ')
    expect(html).toContain('click me')
  })

  it('drops a data: link, keeping its text', async () => {
    const html = await renderAtlasBody('[x](data:text/html;base64,PHNjcmlwdD4=)')
    expect(html).not.toContain('<a ')
    expect(html).toContain('x')
  })

  it('marks off-site links noreferrer/nofollow/ugc and opens them in a new tab', async () => {
    const html = await renderAtlasBody('[docs](https://example.com/a)')
    expect(html).toContain('href="https://example.com/a"')
    expect(html).toContain('rel="noreferrer nofollow ugc"')
    expect(html).toContain('target="_blank"')
  })

  it('leaves same-origin links alone', async () => {
    const html = await renderAtlasBody('[home](/atlas)')
    expect(html).toContain('<a href="/atlas">home</a>')
    expect(html).not.toContain('target="_blank"')
  })

  it('treats a protocol-relative URL as off-site', async () => {
    // `//evil.example/x` has no scheme but is absolutely not same-origin — the
    // easiest way to smuggle a third-party fetch past a naive "starts with /"
    // check.
    const html = await renderAtlasBody('![shot](//evil.example/x.png)')
    expect(html).not.toContain('<img')
  })

  it('turns an off-site image into a link rather than a third-party fetch', async () => {
    // Ground rule: no network calls at build or run time. An <img> pointing
    // off-site is a run-time fetch from the visitor's browser.
    const html = await renderAtlasBody('![a screenshot](https://evil.example/track.png)')
    expect(html).not.toContain('<img')
    expect(html).toContain('href="https://evil.example/track.png"')
    expect(html).toContain('a screenshot')
  })

  it('keeps a same-origin image as an image', async () => {
    const html = await renderAtlasBody('![shot](/atlas/screenshots/x/01.webp)')
    expect(html).toContain('<img src="/atlas/screenshots/x/01.webp" alt="shot"')
    expect(html).toContain('loading="lazy"')
  })

  it('wraps tables so a wide one scrolls itself instead of the page', async () => {
    const html = await renderAtlasBody('| A | B |\n|---|---|\n| 1 | 2 |')
    expect(html).toContain('<div class="atlas-table-wrap"><table class="atlas-table">')
    expect(html).toContain('</table></div>')
  })

  it('demotes every heading one level so the page keeps a single h1', async () => {
    const html = await renderAtlasBody('# Title\n\n## Section\n\n### Detail')
    expect(html).not.toContain('<h1>')
    expect(html).toContain('<h2>Title</h2>')
    expect(html).toContain('<h3>Section</h3>')
    expect(html).toContain('<h4>Detail</h4>')
  })

  it('does not demote past h6', async () => {
    const html = await renderAtlasBody('###### Six')
    expect(html).toContain('<h6>Six</h6>')
  })
})

describe('url classification', () => {
  it.each([
    ['/atlas', true],
    ['#anchor', true],
    ['./rel', true],
    ['//evil.example', false],
    ['https://example.com', false],
    ['javascript:alert(1)', false],
  ])('isLocalUrl(%s) === %s', (href, expected) => {
    expect(isLocalUrl(href)).toBe(expected)
  })

  it.each([
    ['/atlas', true],
    ['https://example.com', true],
    ['http://example.com', true],
    ['mailto:a@b.c', true],
    ['javascript:alert(1)', false],
    ['data:text/html,x', false],
    ['vbscript:x', false],
  ])('isSafeUrl(%s) === %s', (href, expected) => {
    expect(isSafeUrl(href)).toBe(expected)
  })
})

describe('resolveScreenshots', () => {
  it('resolves entry-relative paths against the per-slug vendored directory', () => {
    const { urls } = resolveScreenshots(FIXTURES, 'alpha-site', ['screenshots/01-one.webp'])
    expect(urls).toEqual(['/atlas/screenshots/alpha-site/01-one.webp'])
  })

  it('drops a path with no file behind it instead of emitting a broken image', () => {
    const { urls } = resolveScreenshots(FIXTURES, 'alpha-site', ['screenshots/99-missing.webp'])
    expect(urls).toEqual([])
  })

  it('cannot be walked out of the entry directory', () => {
    // index.json is community-authored; only the basename is ever trusted, so
    // this resolves to <dir>/screenshots/alpha-site/index.json — which does
    // not exist — rather than escaping upward.
    const { urls } = resolveScreenshots(FIXTURES, 'alpha-site', ['../../../index.json'])
    expect(urls).toEqual([])
  })
})

describe('resolveCorpus', () => {
  // A corpus staged outside the fixtures directory, so cases that need a
  // symlink or a 4 MiB file don't have to live in the repo.
  const staged = (build: (root: string) => void): string => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'atlas-corpus-'))
    const root = path.join(dir, 'staged-site', '.sightmap')
    fs.mkdirSync(root, { recursive: true })
    build(root)
    return dir
  }

  it('collects the vendored tree as .sightmap-prefixed members, in byte order', () => {
    const { files, problems } = resolveCorpus(FIXTURES, 'alpha-site')
    expect(problems).toEqual([])
    expect(files.map((f) => f.name)).toEqual([
      '.sightmap/app.yaml',
      '.sightmap/config.yaml',
      '.sightmap/shared/nav.yaml',
    ])
  })

  it('returns nothing at all for an entry with no vendored corpus', () => {
    // The site still builds and the entry still renders; only the install
    // command has nothing behind it. build-atlas.ts warns about that.
    expect(resolveCorpus(FIXTURES, 'beta-site')).toEqual({ files: [], problems: [] })
  })

  it('reports a symlink rather than publishing whatever it points at', () => {
    // tar would follow it and publish the target's bytes under a name the
    // atlas never reviewed.
    const dir = staged((root) => {
      fs.writeFileSync(path.join(root, 'config.yaml'), 'version: 1\n')
      fs.symlinkSync('/etc/passwd', path.join(root, 'secrets.yaml'))
    })
    const { problems } = resolveCorpus(dir, 'staged-site')
    expect(problems).toEqual(['.sightmap/secrets.yaml is a symlink'])
  })

  it('reports a file over the limit the CLI extractor enforces', () => {
    // Emitting it would publish an install that fails on the user's machine
    // while the build reports success.
    const dir = staged((root) => {
      fs.writeFileSync(path.join(root, 'huge.yaml'), Buffer.alloc(MAX_CORPUS_FILE_BYTES + 1))
    })
    const { files, problems } = resolveCorpus(dir, 'staged-site')
    expect(files).toEqual([])
    expect(problems[0]).toMatch(/huge\.yaml is \d+ bytes, over the 4194304-byte file limit/)
  })
})

describe('loadAtlas', () => {
  it('skips a schema-invalid entry instead of failing the build', async () => {
    // The whole reason the atlas is vendored rather than fetched is that a bad
    // community merge must not be able to break a sightmap.org deploy. A hard
    // throw here would hand any contributor exactly that.
    const atlas = await loadAtlas(FIXTURES)
    expect(atlas.entries.map((e) => e.slug)).not.toContain('broken-site')
    expect(atlas.skipped).toContain('broken-site')
    expect(warn).toHaveBeenCalled()
  })

  it('keeps the first of two entries sharing a slug', async () => {
    // Both would write dist/atlas/alpha-site/index.html; the second would
    // silently win. Dropping it loudly is the honest outcome.
    const atlas = await loadAtlas(FIXTURES)
    const alpha = atlas.entries.filter((e) => e.slug === 'alpha-site')
    expect(alpha).toHaveLength(1)
    expect(alpha[0].name).toBe('Alpha')
    expect(atlas.skipped.filter((s) => s === 'alpha-site')).toHaveLength(1)
  })

  it('sorts by updated date, newest first', async () => {
    const atlas = await loadAtlas(FIXTURES)
    expect(atlas.entries.map((e) => e.slug)).toEqual(['alpha-site', 'beta-site'])
  })

  it('resolves only the screenshots that were actually vendored', async () => {
    const atlas = await loadAtlas(FIXTURES)
    const alpha = atlas.entries.find((e) => e.slug === 'alpha-site')!
    // index.json lists two; one has no file on disk.
    expect(alpha.screenshots).toHaveLength(2)
    expect(alpha.screenshotUrls).toEqual(['/atlas/screenshots/alpha-site/01-one.webp'])
  })

  it('renders the entry README body and keeps the raw markdown for the twin', async () => {
    const atlas = await loadAtlas(FIXTURES)
    const alpha = atlas.entries.find((e) => e.slug === 'alpha-site')!
    expect(alpha.bodyHtml).toContain('<h2>Alpha</h2>')
    expect(alpha.bodyHtml).toContain('Body prose.')
    // The twin is verbatim, front matter included.
    expect(atlas.markdown.get('alpha-site')).toContain('---\nname: Alpha')
  })

  it('renders an entry with no README rather than dropping it', async () => {
    const atlas = await loadAtlas(FIXTURES)
    const beta = atlas.entries.find((e) => e.slug === 'beta-site')!
    expect(beta.bodyHtml).toBe('')
    expect(atlas.markdown.has('beta-site')).toBe(false)
  })

  it('exposes index.json verbatim for the /atlas/index.json route', async () => {
    const atlas = await loadAtlas(FIXTURES)
    // Byte-for-byte, including the entries this loader itself rejected: the
    // machine twin is what the generator produced, not what the site renders.
    expect(atlas.indexJson).toContain('"slug": "broken-site"')
    expect(atlas.schemaVersion).toBe(1)
  })
})
