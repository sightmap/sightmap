// Package atlas is the client half of the community atlas: the published
// catalog at sightmap.org/atlas that an agent searches to find out whether the
// site it is about to automate has already been mapped, and installs from when
// it has.
//
// Both the index and the archives come from sightmap.org, not from the atlas
// git repo. The gallery rebuild is what enforces removed.yaml, so a CLI that
// read the repo directly would keep serving entries the atlas has taken down,
// and would install an archive nothing in the repo produces. [IndexOptions.URL]
// (`--index`) and [Options.ArchiveURL] (`--source`) override both, which is how
// a mirror or a private corpus store works.
//
// The package owns the index schema, the search ranking, the index cache, the
// fetch policy, and the [Install] operation. The CLI's `sightmap atlas`
// adapter is one consumer; the sightmap/atlas repository's publisher CI is the
// other, and it is an intended one. Both sides must agree on what a valid
// entry is, so the rules live here in one importable place — a published entry
// that passes atlas CI but that every shipped CLI refuses is the failure this
// package exists to prevent.
//
// # Search and install are decoupled
//
// [LoadIndex] reads the catalog. [Install] fetches one archive from a URL
// template and never reads the index. The split is what keeps an install
// working through an index outage or a schema change, and lets the index grow
// fields without a CLI release.
//
//	idx, err := atlas.LoadIndex(ctx, atlas.IndexOptions{})
//	hits := idx.Index.Search(atlas.Query{Text: "squareup.com"})
//	res, err := atlas.Install(ctx, hits[0].Entry.Slug, atlas.Options{Target: ".sightmap"})
//
// # Index
//
// The atlas publishes a single JSON index:
//
//	{
//	  "schema_version": 1,
//	  "entries": [
//	    {
//	      "slug":          "square-pos",
//	      "name":          "Square POS",
//	      "description":   "Point-of-sale checkout, catalog, and order history.",
//	      "domains":       ["squareup.com", "app.squareup.com"],
//	      "categories":    ["payments", "commerce"],
//	      "stats":         {"views": 12, "components": 48, "requests": 23},
//	      "last_verified": "2026-07-14"
//	    }
//	  ]
//	}
//
// Unknown object fields are ignored on purpose so the atlas can grow metadata
// (stars, screenshots, authors) without breaking already-shipped CLIs.
// [SchemaVersion] is the version this package understands; a higher
// schema_version is refused before the rest of the document is decoded, so a
// restructured future index produces "upgrade sightmap" rather than a raw JSON
// error.
//
// [LoadIndex] caches the fetched bytes in ~/.sightmap/atlas/index.json for
// [IndexTTL], alongside the browser cache at ~/.sightmap/browsers. The cache
// is an optimization: a missing, corrupt, stale, or differently-sourced file
// is a miss rather than an error, and a cache that cannot be written costs a
// round trip rather than a search.
//
// # Search
//
// [Index.Search] matches the query against slug, name, domains, categories,
// and description, ranked so an exact domain hit comes first — the caller
// usually has a URL, not a slug. Matching is case-insensitive substring
// containment. Slugs and domains also match when the query *contains* the
// field, so `square-pos-terminal` finds `square-pos`; names, categories, and
// descriptions do not, because a short category read that way turns `position
// tracking` into a hit for every entry filed under `pos`. Domains are
// normalized first, so a pasted URL, a bare hostname, and a www. hostname
// resolve to the same entry. An empty query matches everything, which is what
// `sightmap atlas list` runs.
//
// Every string in the index is untrusted input. Results carry an install
// command, so an entry whose slug [ValidateSlug] rejects never appears in one;
// every other index-supplied string that reaches a terminal goes through
// [SafeText] and [Entry.Detail]. Searching prints far more atlas-authored text
// than installing ever did. [Entry.Validate] is the stricter, publisher-side
// check that catches the same bytes at the source.
//
// # Transport policy
//
// [Client] fetches over HTTPS only, with a plain-HTTP exception for loopback
// hosts so tests and local mirrors work. The policy runs on the URL a caller
// hands [Client.Fetch] and again on every redirect hop, so a mirror or a
// man-in-the-middle cannot downgrade a fetch to plaintext with a 302. Bodies
// are read through a size cap ([MaxIndexBytes], [MaxArchiveBytes]), because
// the per-fetch [FetchTimeout] bounds duration, not bytes.
//
// # Install
//
// [Install] refuses a non-empty target before it touches the network, fetches
// one .tar.gz, extracts it into a temporary directory beside the target, loads
// the staged corpus, and renames it into place. The rename is the atomicity:
// until it runs the target does not exist, and it is one syscall. A corpus
// that does not load is reported as a defect in the atlas entry, and nothing
// is installed.
//
// An archive is untrusted too. What it may do is bounded on every axis:
// [MaxArchiveBytes] on the wire and [MaxCorpusBytes] decompressed (a gzip bomb
// is small on the wire and enormous on disk), [MaxCorpusFileBytes] per file,
// [MaxArchiveEntries] members, regular files and directories only, and every
// member path under .sightmap/ with no absolute path, no traversal, and no
// control characters ([ValidateCorpusPath]), re-checked for containment after
// it is joined onto the staging directory.
//
// Reproducibility is not this package's job. A corpus is YAML the user checks
// into their own repository; the commit it was installed from stops mattering
// the moment they edit it.
package atlas
