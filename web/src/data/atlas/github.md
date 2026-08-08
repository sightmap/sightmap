---
name: GitHub
slug: github
site_url: https://github.com/
domains: [github.com]
description: Public repository hosting — org, repo, file, issue, pull request, and release views, mapped signed-out across two frontends.
categories: [devtools, saas]
author: chiplay
created: 2026-08-08
updated: 2026-08-08
last_verified: 2026-08-08
cli_version: 0.19.0
spec_version: 1
method: browser
auth: none
---

# GitHub

The public, signed-out surface of github.com, mapped against the `sightmap`
organization and its repositories.

## Coverage

| View | Route | What it holds |
|---|---|---|
| `OrgProfile` | `/:org` | Pinned items and the public repository list |
| `RepoHome` | `/:owner/:repo` | File table, latest commit, rendered README |
| `FileBlob` | `/:owner/:repo/blob/:ref/**` | One file at a ref |
| `PullList` | `/:owner/:repo/pulls` | Open pull requests — legacy frontend |
| `PullDetail` | `/:owner/:repo/pull/:number` | Conversation, commits, checks, diff |
| `IssueList` | `/:owner/:repo/issues` | Issues — React frontend |
| `Releases` | `/:owner/:repo/releases` | Tagged releases, newest first |
| `NotFound` | `/**` | 404, served with a real 404 status |

## Hazards

**Two frontends are in production at once, and the seam runs between two pages
that look identical.** `/pulls` is served by the legacy frontend: rows are
`.js-issue-row`, each carrying an `id` of `issue_<number>`. `/issues` is served
by React: rows are `[data-testid="issue-pr-title-link"]` inside
`section[aria-label="All issues"]`, and `.js-issue-row` matches nothing.
Selectors written against either list return zero on the other, with no error.
Establish which frontend served a page before reading anything off it.

**Half the ids and classes in the document are generated and change without
notice.** Ids like `#_R_2qbd_` are React node ids and differ between renders of
the same page. Classes like `.Primer_Brand__Text-module__Text___XeGJJ` are CSS
module hashes and change on every deploy.

**The CSS module prefix is stable even though the hash is not.** The
`Primer_Brand__Text-module__Text___` part comes from the source module name and
survives rebuilds, so `[class*="Primer_Brand__Text-module__Text___"]` works
where the full class does not. Use `class*=`, not `class^=` — `^=` matches the
start of the whole `class` attribute, so it quietly depends on that class being
listed first, which is not something the site guarantees.

**Prefer the `aria-label`ed landmarks to everything else.** `nav[aria-label="Global"]`,
`nav[aria-label="Repository"]`, `nav[aria-label="Repository files"]`,
`nav[aria-label="Organization"]`, `nav[aria-label="Pull request navigation tabs"]`
and `nav[aria-label="Pagination"]` are written for assistive technology, are
stable across both frontends, and are the only naming on the page that
describes purpose rather than styling.

**`/:org` and `/:user` are the same URL shape.** Nothing in the path says which
you are on. `nav[aria-label="Organization"]` is present for an organization and
absent for a user.

**The 404 page has no `h1`.** Anything that treats "no heading yet" as "still
loading" will wait forever on it. `img[alt*="404"]` is the positive signal.

**`nav[aria-label="Repository files"]` is on the repo root and not on a blob.**
The sidebars look the same. Its absence is the cheapest test for which of the
two you are on.

## Method

Mapped signed-out with a real browser against `sightmap/sightmap`,
`sightmap/atlas` and the `sightmap` organization page. Every selector in the
corpus was counted live on the view that declares it.

Screenshots deliberately avoid pull request and issue conversation threads,
which carry contributor names, avatars, and their writing.
