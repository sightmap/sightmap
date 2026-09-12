// Turns the vendored src/data/atlas/ tree into the two things the site needs:
//
//   1. src/generated/atlas-manifest.ts — a typed manifest the app imports, the
//      same shape of generated module scripts/build-blog.ts produces for posts.
//   2. public/atlas/ — the servable side: screenshots, the verbatim index.json,
//      one verbatim `<slug>.md` per entry, and one `<slug>.tar.gz` holding that
//      entry's corpus. Vite copies public/ into dist/ untouched and `vite dev`
//      serves it, so this is what makes the images, the machine twins (P4.3),
//      and the install work in dev and in a build with no route handling of
//      their own.
//
// The tarball is what `sightmap atlas add <slug>` fetches. It is served from
// here rather than from raw.githubusercontent so a takedown binds both
// surfaces: an entry removed from the vendored tree stops being installable in
// the same rebuild that stops it being listed.
//
// public/atlas/ is generated and gitignored — it is wiped and rewritten on
// every run so a slug removed upstream (a takedown) actually disappears from
// dist/ instead of lingering as an orphaned file.
//
// The same two outputs cover the WebMCP directory (src/data/directory/): the
// manifest gains `directoryListings` / `directoryCategories`, and public/atlas/
// gains the agent-facing documents its README's "What the build generates"
// table names — directory.json, stats.json, sites/ (including the building
// blueprint each listing's scan derives), scans/, hosts/, one .md twin and one
// badge per listing. Same no-network, wipe-and-rewrite contract.
//
// The screenshots have to be copied rather than imported from src/: they are
// referenced from React components that scripts/prerender.tsx renders under
// tsx, where Vite's asset pipeline (`import.meta.glob`, `?url`) does not
// exist. public/ is the site's existing answer for that — content images for
// blog posts live there for the same reason.
import fs from 'node:fs'
import path from 'node:path'
import { atlasCategories, loadAtlas, resolveCorpus, resolveScreenshots } from './lib/atlas'
import { blueprintSummary, deriveBlueprint } from './lib/blueprint'
import { assignLots, fillerFor, planCity, propsFor } from './lib/city'
import { directoryCategories, loadDirectory, scanDateOf, scanFilesFor } from './lib/directory'
import { listingMarkdown } from './lib/directory-markdown'
import {
  directoryIndexDocument,
  directoryStatsDocument,
  hostDocuments,
  reportWithoutTranscript,
  siteDocument,
  toolsDocument,
} from './lib/directory-outputs'
import { listingBadgeSvg } from './lib/badge'
import { sightkickStarter } from './lib/sightkick-starter'
import { tarGz } from './lib/tar'
import type { Blueprint } from '../src/types/blueprint'
import type { DirectoryListingView } from '../src/types/directory'

const DATA_DIR = path.resolve('src/data/atlas')
const DATA_DIR_DIRECTORY = path.resolve('src/data/directory')
const GENERATED_DIR = path.resolve('src/generated')
const PUBLIC_DIR = path.resolve('public/atlas')

/** Writes one generated JSON document under public/atlas/. */
function writeJson(rel: string, value: unknown) {
  const full = path.join(PUBLIC_DIR, rel)
  fs.mkdirSync(path.dirname(full), { recursive: true })
  fs.writeFileSync(full, `${JSON.stringify(value, null, 2)}\n`)
}

/** Copies a vendored file into public/atlas/ byte for byte. */
function copyInto(from: string, rel: string) {
  const full = path.join(PUBLIC_DIR, rel)
  fs.mkdirSync(path.dirname(full), { recursive: true })
  fs.copyFileSync(from, full)
}

async function main() {
  const atlas = await loadAtlas(DATA_DIR)

  fs.rmSync(PUBLIC_DIR, { recursive: true, force: true })
  fs.mkdirSync(PUBLIC_DIR, { recursive: true })
  fs.mkdirSync(GENERATED_DIR, { recursive: true })

  // P4.3: /atlas/index.json, served byte-for-byte as vendored. Written from
  // the original text rather than re-serialized from the parsed object so the
  // route really is the file the atlas generator produced.
  fs.writeFileSync(path.join(PUBLIC_DIR, 'index.json'), atlas.indexJson)

  for (const entry of atlas.entries) {
    // P4.3: /atlas/<slug>.md — front matter plus body, verbatim.
    const md = atlas.markdown.get(entry.slug)
    if (md !== undefined) fs.writeFileSync(path.join(PUBLIC_DIR, `${entry.slug}.md`), md)

    const { files } = resolveScreenshots(DATA_DIR, entry.slug, entry.screenshots)
    if (files.length > 0) {
      const outDir = path.join(PUBLIC_DIR, 'screenshots', entry.slug)
      fs.mkdirSync(outDir, { recursive: true })
      for (const file of files) fs.copyFileSync(file, path.join(outDir, path.basename(file)))
    }

    // /atlas/<slug>.tar.gz — the corpus `sightmap atlas add` pulls. Same
    // per-entry, non-fatal contract as the schema check: a corpus that would
    // not install is left unpublished with a warning, and the rest of the site
    // ships. The page keeps rendering; the command on it 404s, which is the
    // honest outcome for an entry with nothing behind it.
    const corpus = resolveCorpus(DATA_DIR, entry.slug)
    let members = 0
    if (corpus.problems.length > 0) {
      console.warn(
        `  ! atlas entry ${entry.slug} publishes no corpus archive:\n` +
          corpus.problems.map((p) => `    - ${p}`).join('\n')
      )
    } else if (corpus.files.length === 0) {
      console.warn(`  ! atlas entry ${entry.slug} has no vendored .sightmap/ — sightmap atlas add will 404`)
    } else {
      // Packing reads community-vendored files off disk, so it stays inside
      // the per-entry contract: whatever goes wrong costs this entry its
      // archive, never the build.
      try {
        const archive = tarGz(corpus.files.map((f) => ({ name: f.name, body: fs.readFileSync(f.path) })))
        fs.writeFileSync(path.join(PUBLIC_DIR, `${entry.slug}.tar.gz`), archive)
        members = corpus.files.length
      } catch (err) {
        console.warn(
          `  ! atlas entry ${entry.slug} publishes no corpus archive: ${err instanceof Error ? err.message : err}`
        )
      }
    }

    console.log(
      `  built ${entry.slug} (${entry.stats.views} view(s), ${files.length} screenshot(s), ${members} corpus file(s)${
        md === undefined ? ', no README' : ''
      })`
    )
  }

  // ---- WebMCP directory ----
  //
  // Read after the community atlas and with its slugs reserved: both kinds of
  // listing render at /atlas/<slug> and publish files into this same
  // directory, so a slug already taken by an entry has to lose here rather
  // than silently overwrite that entry's .md twin (see loadDirectory).
  const directory = loadDirectory(DATA_DIR_DIRECTORY, atlas.entries.map((e) => e.slug))
  const generatedAt = new Date().toISOString()
  const blueprints = new Map<string, Blueprint>()

  for (const listing of directory.listings) {
    // Every scan on file, verbatim. Copied rather than re-serialized from the
    // parsed report for the same reason index.json is: the published artifact
    // has to be the file the scanner wrote, or it is not evidence.
    const scanFiles = scanFilesFor(DATA_DIR_DIRECTORY, listing.slug)
    for (const rel of scanFiles) {
      copyInto(path.join(DATA_DIR_DIRECTORY, rel), `scans/${listing.slug}/${scanDateOf(rel)}.json`)
    }
    // /atlas/scans/<slug>.json is the one the listing was reviewed against —
    // the report every other document on this page summarises, which is what
    // makes it the right one to serve unversioned.
    copyInto(path.join(DATA_DIR_DIRECTORY, listing.scan), `scans/${listing.slug}.json`)

    writeJson(`sites/${listing.slug}.json`, siteDocument(listing))
    writeJson(`sites/${listing.slug}/tools.json`, toolsDocument(listing))

    // The building, derived from the scan. Published next to the tools it draws
    // so anything that renders a listing — this site's city, someone else's —
    // reads one file rather than re-deriving the geometry.
    const blueprint = deriveBlueprint(listing)
    blueprints.set(listing.slug, blueprint)
    writeJson(`sites/${listing.slug}/blueprint.json`, blueprint)

    // The markdown twin of the listing page, and the badge a listed site can
    // embed. Both are generated from the listing, never copied: unlike a
    // community README there is no authored file behind them.
    fs.writeFileSync(path.join(PUBLIC_DIR, `${listing.slug}.md`), listingMarkdown(listing))
    const badgeDir = path.join(PUBLIC_DIR, listing.slug)
    fs.mkdirSync(badgeDir, { recursive: true })
    fs.writeFileSync(path.join(badgeDir, 'badge.svg'), listingBadgeSvg(listing))

    // Host lookup, served at /api/atlas/lookup/<host> by a netlify.toml
    // rewrite. Both the bare and the www. spelling, so an agent holding a
    // hostname off the address bar never has to normalise it first.
    const hosts = hostDocuments(listing)
    for (const { host, document } of hosts) writeJson(`hosts/${host}.json`, document)

    console.log(
      `  listed ${listing.slug} (${listing.counts.tools} tool(s), ${listing.counts.pages} page(s), ` +
        `${scanFiles.length} scan(s), ${hosts.length} host lookup(s))`
    )
    // The derived shape, in one line: a rescan that changes the building shows
    // up here rather than only in a 300-line JSON diff.
    console.log(`    building ${blueprintSummary(blueprint)}`)
  }

  // Written whichever way the directory came out, including empty: an agent
  // that fetches these should read "nothing listed yet", not a 404 it has to
  // interpret.
  writeJson('directory.json', directoryIndexDocument(directory.listings, generatedAt))
  writeJson(
    'stats.json',
    directoryStatsDocument(directory.listings, { generatedAt, communityMaps: atlas.entries.length })
  )

  // What the app imports. The scan report rides along so the listing page can
  // render checks and pages without a fetch — minus the `$ ` CLI transcript,
  // which nothing on the page renders and which is already published in full
  // in the scan JSON above.
  // The city: one plan, the same on every build, with the listings placed on
  // it and the unclaimed lots filled or fenced. Written after the listings so
  // a lot a listing's YAML records is what the page shows.
  const plan = planCity()
  const assignments = assignLots(
    plan,
    // The blueprint's floor count is what a building is drawn from, so the
    // city needs it to know how tall each listing stands on its lot.
    directory.listings.map((listing) => ({ ...listing, floors: blueprints.get(listing.slug)?.floors.length }))
  )
  const fills = fillerFor(plan, assignments)
  const cityDocument = { ...plan, props: propsFor(plan, assignments), assignments, fills }
  writeJson('city.json', cityDocument)
  const placed = new Map(assignments.map((a) => [a.slug, a]))
  console.log(`  city: ${assignments.length} placed on ${plan.lots.length} lots, ${fills.length} filled or fenced`)

  const directoryListings: DirectoryListingView[] = directory.listings.map((listing) => ({
    ...listing,
    report: reportWithoutTranscript(listing.report),
    starter: sightkickStarter(listing.report),
    blueprint: blueprints.get(listing.slug) as Blueprint,
    lot: placed.get(listing.slug)?.lot ?? -1,
    peak: placed.get(listing.slug)?.peak ?? false,
  }))

  fs.writeFileSync(
    path.join(GENERATED_DIR, 'atlas-manifest.ts'),
    `// Auto-generated by scripts/build-atlas.ts — do not edit\n` +
      `import type { AtlasEntry } from '@/types/atlas'\n` +
      `import type { CityDocument } from '@/components/city/document'\n` +
      `import type { DirectoryListingView } from '@/types/directory'\n\n` +
      `export const atlasGeneratedAt = ${JSON.stringify(atlas.generatedAt)}\n\n` +
      `export const atlasEntries: AtlasEntry[] = ${JSON.stringify(atlas.entries, null, 2)}\n\n` +
      `export const atlasCategories: string[] = ${JSON.stringify(atlasCategories(atlas.entries))}\n\n` +
      `export const directoryListings: DirectoryListingView[] = ${JSON.stringify(directoryListings, null, 2)}\n\n` +
      `export const directoryCategories: string[] = ${JSON.stringify(directoryCategories(directory.listings))}\n\n` +
      // The city rides the manifest rather than a fetch of public/atlas/
      // city.json: the page has to know the plan before it draws a frame, and
      // public/atlas is wiped and rewritten on every build. Written compact —
      // three hundred lots pretty-printed is a megabyte of generated source.
      `export const city: CityDocument = ${JSON.stringify(cityDocument)}\n`
  )

  const skipped = atlas.skipped.length > 0 ? `, ${atlas.skipped.length} skipped` : ''
  const skippedListings = directory.skipped.length > 0 ? `, ${directory.skipped.length} skipped` : ''
  console.log(
    `\n  atlas build complete: ${atlas.entries.length} entry(s)${skipped}, ` +
      `${directory.listings.length} listing(s)${skippedListings}`
  )
}

main().catch((err) => {
  console.error('Atlas build failed:\n', err instanceof Error ? err.message : err)
  process.exit(1)
})
