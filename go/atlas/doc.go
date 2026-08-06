// Package atlas implements the client half of the community atlas wire
// contract: the published index at github.com/sightmap/atlas from which a
// visitor installs a ready-made `.sightmap/` corpus with one command.
//
// The package owns the index schema, the raw-URL layout, the fail-closed
// validation rules, the fetch policy, and the [Install] operation. The CLI's
// `sightmap add` adapter is one consumer; the sightmap/atlas repository's
// publisher CI is the other, and it is an intended one. Both sides must agree
// on what a valid entry is, so the rules live here in one importable place
// rather than being re-implemented per side — a published entry that passes
// atlas CI but that every shipped CLI refuses to install is the failure this
// package exists to prevent.
//
// # Wire contract
//
// The atlas publishes a single JSON index:
//
//	{
//	  "schema_version": 1,
//	  "entries": [
//	    {
//	      "slug":   "square-pos",                                  // required, identifies the entry
//	      "name":   "Square POS",                                  // optional, display only
//	      "commit": "0123456789abcdef0123456789abcdef01234567",    // optional, 40-char lowercase sha
//	      "files":  [".sightmap/config.yaml", ".sightmap/views/checkout.yaml"]
//	    }
//	  ]
//	}
//
// Unknown object fields are ignored on purpose so the atlas can grow metadata
// (descriptions, stars, screenshots) without breaking already-shipped CLIs.
// [SchemaVersion] is the version this package understands; a higher
// schema_version is refused before the rest of the document is decoded, so a
// restructured future index produces "upgrade sightmap" rather than a raw JSON
// error.
//
// Every string in the index is untrusted input. Slugs, commits, and file paths
// are validated fail-closed ([ValidateSlug], [ValidateCommit],
// [ValidateCorpusPath], [Entry.Validate]) before they are spliced into a URL or
// a filesystem path, and any index-supplied string that reaches a terminal is
// rendered through [SafeText].
//
// # URL layout
//
// One rule, no per-host special cases:
//
//	index:  <root>/<ref>/index.json
//	files:  <root>/<ref>/entries/<slug>/<path>
//
// <root> is the raw-content root of the atlas repository, <ref> is a git ref,
// and <path> is the entry's corpus-relative path (always under .sightmap/).
// The ref of a file fetch is the entry's commit when it publishes one and the
// ref parsed from the index URL otherwise — never a hardcoded branch, so
// pointing --index at a non-main ref fetches that ref's content.
//
// The ref is parsed off the index URL: it is the trailing path segment before
// the index file, or — when the path contains a "refs" segment — everything
// from that segment on, so GitHub's own refs/heads/<branch> raw URLs describe
// the same layout. That single rule covers raw.githubusercontent.com, a GitHub
// Enterprise raw host, and a plain byte-for-byte copy of what either serves;
// a mirror needs no bespoke shape, only the ref directory level it was copied
// from. An index URL with no ref segment is rejected with an explicit error
// rather than guessed at.
//
// # Transport policy
//
// [Client] fetches over HTTPS only, with a plain-HTTP exception for loopback
// hosts so tests and local mirrors work. The policy runs on the URL a caller
// hands [Client.Fetch] and again on every redirect hop, so a mirror or a
// man-in-the-middle cannot downgrade a fetch to plaintext with a 302.
// Responses are read through a size cap ([MaxIndexBytes], [MaxFileBytes],
// [MaxEntryBytes]) and an entry may not list more than [MaxEntryFiles] files,
// because the per-fetch [FetchTimeout] bounds duration, not bytes.
//
// # Install
//
// [Install] checks local preconditions before touching the network, fetches
// every file before writing any, stages the result in a temporary directory,
// renames it onto the target, and loads the corpus that landed to prove the
// atlas entry actually works. Whatever the target held is moved aside, not
// deleted, until that load succeeds; a corpus that does not load undoes the
// rename and puts the previous contents back. An install therefore either
// lands whole or leaves the target exactly as it was.
//
// # Replacing a target
//
// [Options.Replace] is destructive: the target's previous contents do not
// survive. So it is confined to directories that are visibly a corpus and
// nothing else. Two rules, both fail-closed, both reported as
// [ErrUnsafeReplace]:
//
//   - Location. The working directory, any directory containing it, the user's
//     home directory, any directory containing that, and a filesystem root are
//     refused whatever they hold. A mistyped target is the whole hazard here,
//     and no inspection of the contents makes deleting one of those the
//     caller's intent.
//   - Contents. Every top-level entry of the target must be corpus content: a
//     .yaml or .yml file, or one of the corpus's own subdirectories (views,
//     snapshots, review). A .git directory, a .env, a go.mod, a src — anything
//     else — means the target is a project directory that happens to hold
//     YAML. macOS's .DS_Store is the single exception: it holds nothing of the
//     user's, and refusing every corpus someone has opened in Finder would
//     read as a bug.
//
// The contents rule is an allowlist rather than a list of alarming names on
// purpose: a blocklist only refuses the destruction someone thought of in
// advance, while the allowlist confines Replace to directories shaped like the
// ones Install itself writes. It reads one level deep, which is what it takes
// to establish that the target *is* a corpus directory; proving every byte
// beneath it is corpus content would mean walking a snapshot set on every
// install.
package atlas
