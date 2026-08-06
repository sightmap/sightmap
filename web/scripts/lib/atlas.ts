// The single reader for src/data/atlas — the vendored copy of the community
// atlas (see that directory's README.md for how it gets there). Every build
// script goes through this so one malformed entry is diagnosed in one place
// instead of surfacing as `undefined` in a meta tag three scripts downstream.
//
// Validation is deliberately *per entry* and non-fatal: the whole point of
// vendoring rather than fetching is that a bad community merge cannot break a
// sightmap.org deploy. An entry that fails the schema is dropped from the
// gallery with a loud warning in the build log; the rest of the site ships.
// Failing the build here would hand any contributor a way to take the site
// down, which is exactly the failure mode the vendoring was designed against.
import fs from 'node:fs'
import path from 'node:path'
import matter from 'gray-matter'
import { Marked } from 'marked'
import { z } from 'zod'
import type { AtlasEntry, AtlasEntryMeta } from '../../src/types/atlas'
import { esc } from './site'
import { MAX_MEMBER_NAME_BYTES } from './tar'

// Atlas READMEs are community-authored, which makes them a different trust
// class from content/blog/*.md (written by maintainers). scripts/lib/posts.ts
// deliberately lets raw HTML through so a post can embed a widget marker;
// doing that here would mean a contributor's README could ship a <script> to
// every visitor of sightmap.org. So the atlas gets its own `Marked` instance —
// not `marked.use()`, which mutates the shared singleton the blog renders
// through — with three rules:
//
//   1. Raw HTML, block or inline, renders as literal text instead of markup.
//   2. Link and image URLs are scheme-allowlisted; anything else degrades to
//      the plain text it was written as, rather than a `javascript:` anchor.
//   3. Off-site images become links. An <img> pointing at a third party is a
//      run-time network call from the visitor's browser to a domain a
//      contributor chose — the same ground-rule and privacy problem as
//      fetching favicons (see src/lib/atlas.ts).
//
// This is a hardening pass over markdown output, not a general-purpose HTML
// sanitizer: it works because the *only* HTML that reaches it is what this
// renderer itself emits.
const SAFE_SCHEME = /^(?:https?:|mailto:)/i
const HAS_SCHEME = /^[a-z][a-z0-9+.-]*:/i

/** Same-origin: a relative or root-relative URL, but not protocol-relative. */
export function isLocalUrl(href: string): boolean {
  return !HAS_SCHEME.test(href) && !href.startsWith('//')
}

export function isSafeUrl(href: string): boolean {
  return isLocalUrl(href) || SAFE_SCHEME.test(href)
}

const atlasMarked = new Marked()
atlasMarked.use({
  renderer: {
    html({ text }) {
      return esc(text)
    },
    // READMEs open with their own `# <site>` heading, but on the detail page
    // the entry's display name is already the <h1> and the body is a section
    // beneath it. Demoting every heading one level keeps the document outline
    // single-rooted instead of shipping two <h1>s per page.
    heading({ tokens, depth }) {
      const level = Math.min(depth + 1, 6)
      return `<h${level}>${this.parser.parseInline(tokens)}</h${level}>\n`
    },
    link({ href, title, tokens }) {
      const inner = this.parser.parseInline(tokens)
      if (!isSafeUrl(href)) return inner
      const t = title ? ` title="${esc(title)}"` : ''
      // Community content, so off-site links carry nofollow/ugc as well as the
      // usual noreferrer.
      const rel = isLocalUrl(href) ? '' : ' target="_blank" rel="noreferrer nofollow ugc"'
      return `<a href="${esc(href)}"${t}${rel}>${inner}</a>`
    },
    image({ href, title, text }) {
      const alt = esc(text ?? '')
      if (isLocalUrl(href)) {
        const t = title ? ` title="${esc(title)}"` : ''
        return `<img src="${esc(href)}" alt="${alt}"${t} loading="lazy" decoding="async">`
      }
      if (!isSafeUrl(href)) return alt
      return `<a href="${esc(href)}" target="_blank" rel="noreferrer nofollow ugc">${alt || esc(href)}</a>`
    },
  },
})

export async function renderAtlasBody(markdown: string): Promise<string> {
  const html = await atlasMarked.parse(markdown, { async: true })
  // A markdown table in a README (the coverage tables are the common case) is
  // the one element wide enough to force the page to scroll sideways on a
  // phone. The page renders this HTML through dangerouslySetInnerHTML, so
  // there is no chance to wrap it in JSX — do it here instead. Safe as a plain
  // string replace precisely because the renderer above escapes raw HTML: the
  // only `<table>` in this string is one marked itself emitted.
  return html
    .replace(/<table>/g, '<div class="atlas-table-wrap"><table class="atlas-table">')
    .replace(/<\/table>/g, '</table></div>')
}

const PerViewSchema = z.object({
  name: z.string().min(1),
  route: z.string().min(1),
  components: z.number().int().nonnegative(),
  requests: z.number().int().nonnegative(),
})

const StatsSchema = z.object({
  views: z.number().int().nonnegative(),
  components: z.number().int().nonnegative(),
  requests: z.number().int().nonnegative(),
  properties: z.number().int().nonnegative(),
  memory: z.number().int().nonnegative(),
})

export const AtlasEntrySchema = z.object({
  slug: z.string().regex(/^[a-z0-9]+(-[a-z0-9]+)*$/, 'must be lowercase kebab-case'),
  name: z.string().min(1),
  site_url: z.string().url(),
  domains: z.array(z.string().min(1)).default([]),
  description: z.string().min(1),
  categories: z.array(z.string().min(1)).default([]),
  author: z.string().min(1),
  // ISO date only. The pages format these with `new Date(d + 'T00:00:00')`.
  created: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, 'must be YYYY-MM-DD'),
  updated: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, 'must be YYYY-MM-DD'),
  last_verified: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, 'must be YYYY-MM-DD'),
  cli_version: z.string().min(1),
  spec_version: z.number().int().positive(),
  method: z.string().min(1),
  auth: z.string().min(1),
  stats: StatsSchema,
  per_view: z.array(PerViewSchema).default([]),
  // Optional by policy: an entry is allowed to ship with no screenshots.
  screenshots: z.array(z.string().min(1)).default([]),
  files: z.array(z.string().min(1)).default([]),
  // Optional: only present when the mapped site is a repo we can pin.
  commit: z.string().min(1).optional(),
})

export const AtlasIndexSchema = z.object({
  schema_version: z.number().int().positive(),
  generated_at: z.string().min(1),
  entries: z.array(z.unknown()).default([]),
})

export interface LoadedAtlas {
  /** Verbatim index.json text, for the `/atlas/index.json` machine twin. */
  indexJson: string
  generatedAt: string
  schemaVersion: number
  entries: AtlasEntry[]
  /** Verbatim `<slug>.md` text, keyed by slug, for the `.md` machine twin. */
  markdown: Map<string, string>
  /** Slugs dropped by validation, so a caller can report them. */
  skipped: string[]
}

function issuesOf(error: z.ZodError): string {
  return error.issues.map((i) => `    - ${i.path.join('.') || '(root)'}: ${i.message}`).join('\n')
}

/**
 * Resolves an entry's index.json screenshot paths to servable URLs.
 *
 * index.json lists them relative to the entry directory in the atlas repo
 * (`screenshots/01-home.webp`); the vendor step files them under a per-slug
 * directory here (`screenshots/<slug>/01-home.webp`). Only the basename is
 * trusted — a `..` or an absolute path in community-authored JSON must not
 * reach into the filesystem — and a path with no file behind it is dropped
 * rather than rendered as a broken image.
 */
export function resolveScreenshots(
  dataDir: string,
  slug: string,
  screenshots: string[]
): { urls: string[]; files: string[] } {
  const urls: string[] = []
  const files: string[] = []
  for (const rel of screenshots) {
    const base = path.basename(rel)
    if (!base || base === '.' || base === '..') continue
    const full = path.join(dataDir, 'screenshots', slug, base)
    if (!fs.existsSync(full)) continue
    urls.push(`/atlas/screenshots/${slug}/${encodeURIComponent(base)}`)
    files.push(full)
  }
  return { urls, files }
}

// Caps on what one entry may publish as an archive, mirrored from the CLI's
// extractor (go/atlas/install.go). The CLI refuses an archive that breaks any
// of them, so emitting one would publish an install that fails on the user's
// machine with the site reporting success. Checking here turns that into a
// build-log warning against the entry that caused it.
export const MAX_CORPUS_FILE_BYTES = 4 << 20 // 4 MiB, one file
export const MAX_CORPUS_BYTES = 32 << 20 // 32 MiB, decompressed total
export const MAX_CORPUS_MEMBERS = 512

/** The one directory an atlas archive may publish files under. */
const CORPUS_PREFIX = '.sightmap/'

export interface CorpusFile {
  /** Archive member name: `.sightmap/`-prefixed, slash-separated, relative. */
  name: string
  /** The vendored file this member is read from. */
  path: string
  size: number
}

export interface ResolvedCorpus {
  files: CorpusFile[]
  /** Reasons this corpus is not publishable, one line each. */
  problems: string[]
}

/** Rejects a name that could not survive a round trip through tar and back. */
function isSafeName(name: string): boolean {
  if (name === '' || name === '.' || name === '..') return false
  if (name.includes('/') || name.includes('\\')) return false
  // Control characters, including the ESC that would turn a filename the CLI
  // prints into a terminal escape sequence.
  return !/[\u0000-\u001f\u007f]/.test(name)
}

function walkCorpus(dir: string, prefix: string, into: ResolvedCorpus): void {
  const dirents = fs
    .readdirSync(dir, { withFileTypes: true })
    // Byte order, not locale order, so the archive is identical whatever the
    // build machine's locale is.
    .sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0))

  for (const dirent of dirents) {
    const name = `${prefix}${dirent.name}`
    const full = path.join(dir, dirent.name)
    if (!isSafeName(dirent.name)) {
      into.problems.push(`${JSON.stringify(name)} is not a safe file name`)
      continue
    }
    // A symlink reads as a regular file after tar follows it, which is exactly
    // the substitution the CLI refuses members for. Report it instead.
    if (dirent.isSymbolicLink()) {
      into.problems.push(`${name} is a symlink`)
      continue
    }
    if (dirent.isDirectory()) {
      walkCorpus(full, `${name}/`, into)
      continue
    }
    if (!dirent.isFile()) {
      into.problems.push(`${name} is not a regular file`)
      continue
    }
    if (Buffer.byteLength(name, 'utf-8') > MAX_MEMBER_NAME_BYTES) {
      into.problems.push(`${name} is longer than ${MAX_MEMBER_NAME_BYTES} bytes`)
      continue
    }
    const { size } = fs.statSync(full)
    if (size > MAX_CORPUS_FILE_BYTES) {
      into.problems.push(`${name} is ${size} bytes, over the ${MAX_CORPUS_FILE_BYTES}-byte file limit`)
      continue
    }
    into.files.push({ name, path: full, size })
  }
}

/**
 * Collects the `.sightmap/` corpus vendored for one entry, as archive members.
 *
 * This is the payload behind `sightmap atlas add <slug>`: index.json's `files[]`
 * names it, and without it the install command on the entry's page has nothing
 * to fetch. The vendored tree is the source of truth rather than `files[]` —
 * what ships is what someone can read in this repo.
 *
 * `problems` is non-empty when the corpus is there but not publishable. A
 * caller should skip the whole archive rather than publish what is left: a
 * corpus missing a file installs, loads, and quietly maps less of the site than
 * the entry claims.
 */
export function resolveCorpus(dataDir: string, slug: string): ResolvedCorpus {
  const root = path.join(dataDir, slug, '.sightmap')
  const resolved: ResolvedCorpus = { files: [], problems: [] }
  if (!fs.existsSync(root)) return resolved

  walkCorpus(root, CORPUS_PREFIX, resolved)

  if (resolved.files.length > MAX_CORPUS_MEMBERS) {
    resolved.problems.push(`holds ${resolved.files.length} files, over the ${MAX_CORPUS_MEMBERS}-file limit`)
  }
  const total = resolved.files.reduce((sum, f) => sum + f.size, 0)
  if (total > MAX_CORPUS_BYTES) {
    resolved.problems.push(`is ${total} bytes, over the ${MAX_CORPUS_BYTES}-byte limit`)
  }
  return resolved
}

export async function loadAtlas(dataDir: string): Promise<LoadedAtlas> {
  const indexPath = path.join(dataDir, 'index.json')
  const indexJson = fs.readFileSync(indexPath, 'utf-8')

  const parsedIndex = AtlasIndexSchema.safeParse(JSON.parse(indexJson))
  if (!parsedIndex.success) {
    // The envelope itself is ours, not a contributor's — if it is malformed the
    // vendor step is broken and there is nothing to degrade to.
    throw new Error(`Invalid atlas index.json:\n${issuesOf(parsedIndex.error)}`)
  }

  const entries: AtlasEntry[] = []
  const markdown = new Map<string, string>()
  const skipped: string[] = []
  const seen = new Set<string>()

  for (const [i, raw] of parsedIndex.data.entries.entries()) {
    const parsed = AtlasEntrySchema.safeParse(raw)
    if (!parsed.success) {
      const label =
        raw && typeof raw === 'object' && 'slug' in raw ? String((raw as { slug: unknown }).slug) : `#${i}`
      console.warn(`  ! skipping atlas entry ${label}:\n${issuesOf(parsed.error)}`)
      skipped.push(label)
      continue
    }
    const meta = parsed.data as AtlasEntryMeta

    // Two entries with one slug would fight over dist/atlas/<slug>/index.html
    // and the second would silently overwrite the first.
    if (seen.has(meta.slug)) {
      console.warn(`  ! skipping atlas entry ${meta.slug}: duplicate slug`)
      skipped.push(meta.slug)
      continue
    }
    seen.add(meta.slug)

    // The entry README is the source for both the detail page's prose and the
    // `/atlas/<slug>.md` machine twin. An entry without one still renders —
    // the spec sheet above the body is built entirely from index.json.
    const mdPath = path.join(dataDir, `${meta.slug}.md`)
    let bodyHtml = ''
    if (fs.existsSync(mdPath)) {
      const rawMd = fs.readFileSync(mdPath, 'utf-8')
      markdown.set(meta.slug, rawMd)
      bodyHtml = await renderAtlasBody(matter(rawMd).content)
    } else {
      console.warn(`  ! atlas entry ${meta.slug} has no ${meta.slug}.md — rendering without a body`)
    }

    const { urls } = resolveScreenshots(dataDir, meta.slug, meta.screenshots)
    entries.push({ ...meta, screenshotUrls: urls, bodyHtml })
  }

  // Most recently updated first, slug as the tiebreak so the order is stable
  // across builds (index.json's own order is generator-defined).
  entries.sort((a, b) => (a.updated === b.updated ? a.slug.localeCompare(b.slug) : b.updated.localeCompare(a.updated)))

  return {
    indexJson,
    generatedAt: parsedIndex.data.generated_at,
    schemaVersion: parsedIndex.data.schema_version,
    entries,
    markdown,
    skipped,
  }
}

/** Every distinct category across the given entries, alphabetical. */
export function atlasCategories(entries: AtlasEntry[]): string[] {
  return [...new Set(entries.flatMap((e) => e.categories))].sort()
}
