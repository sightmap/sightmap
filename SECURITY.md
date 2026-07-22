# Security policy

## What counts as a security issue here

This repo contains:

1. A specification (YAML schema, documentation, JSON Schema)
2. A reference implementation (the Go library and `sightmap` CLI, plus its npm wrapper)
3. Two static websites (Astro docs site and a React marketing site, deployed to Netlify)

Things we treat as security issues:

- Vulnerabilities in a website (XSS, dependency CVEs in a shipped bundle, exposure of secrets)
- Vulnerabilities in the CLI or library (e.g. path traversal while loading a corpus, unsafe handling of a browser session)
- Supply-chain concerns with our published artifacts (the npm package, GitHub release binaries)
- Ambiguities in the spec that could cause implementations to be exploitable if followed literally (e.g. a route-matching rule weaponizable for path traversal)

Things that are **not** security issues and should be filed as regular bugs or discussions:

- Misbehavior of third-party tools that consume sightmaps
- General spec design questions

## Reporting a vulnerability

**Please do not open a public GitHub issue for security reports.**

Email: **subtext@fullstory.com**

Include:

- A clear description of the issue
- Steps to reproduce, or a proof of concept
- The affected version, commit SHA, or published package version
- Your assessment of impact
- Whether you would like to be credited, and if so how

We will acknowledge receipt within **3 business days** and aim to provide a substantive response within **10 business days**.

## Disclosure process

1. You report privately via email.
2. We confirm the issue and determine scope.
3. We develop and test a fix on a private branch.
4. We coordinate a disclosure date with you. For most issues we aim to disclose within 30 days of confirmation.
5. We release the fix, publish an advisory on GitHub, and credit you if you'd like.

If an issue is being actively exploited, we may shorten this timeline.

## Scope

In-scope:

- `github.com/sightmap/sightmap` (this repo — spec, Go implementation, both sites)
- The [`@sightmap/sightmap`](https://www.npmjs.com/package/@sightmap/sightmap) npm package and the GitHub release binaries
- `sightmap.org` and `docs.sightmap.org`

Other repos under the [`sightmap` GitHub organization](https://github.com/sightmap) inherit the org-wide policy at [`sightmap/.github/SECURITY.md`](https://github.com/sightmap/.github/blob/main/SECURITY.md), which uses the same reporting address.

Out-of-scope:

- Third-party integrations, SDKs, or tools that consume sightmaps but are not maintained by the Subtext team
- Any Subtext commercial product — those have their own security process; see [subtext.fullstory.com](https://subtext.fullstory.com)

## Safe harbor

We will not pursue legal action against security researchers who:

- Make a good-faith effort to avoid privacy violations, destruction of data, or interruption of services
- Only interact with accounts they own or with explicit permission from the account holder
- Give us reasonable time to investigate and fix the issue before public disclosure
- Do not exploit the issue beyond what is necessary to demonstrate it
