import { Link, Navigate } from 'react-router'
import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import Seo from '@/components/Seo'
import CopyButton from '@/components/CopyButton'
import AtlasMark from '@/components/atlas/AtlasMark'
import {
  badgeMarkdown,
  checksPassed,
  formatScanned,
  kindClass,
  kindLabel,
  shareUrls,
  toolsByPage,
} from '@/lib/directory'
import { useScrollToTopOnPush } from '@/lib/useScrollToTopOnPush'
import { directoryListings } from '@/generated/atlas-manifest'
import type { DirectoryListingView } from '@/types/directory'
// Shared with scripts/prerender.tsx so a client-side navigation from /atlas
// and a fresh load of the same listing produce an identical <title>.
import { atlasTitle } from '../../scripts/lib/site'

/** One row of the monospace metadata rail. Omitted when it has no value. */
function MetaRow({ label, children }: { label: string; children: React.ReactNode }) {
  if (children === null || children === undefined || children === '') return null
  return (
    <div className="atlas-meta__row">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  )
}

/** One `LABEL / value` cell in the facts strip. Facts only — no grade. */
function Fact({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="atlas-facts__item">
      <span className="atlas-facts__label">{label}</span>
      <span className="atlas-facts__value">{value}</span>
    </div>
  )
}

export default function AtlasListingPage({ listing }: { listing: DirectoryListingView | undefined }) {
  // A click from halfway down the gallery would otherwise land halfway down the
  // listing. Called before the `!listing` guard, since hooks cannot sit after a
  // conditional return.
  useScrollToTopOnPush()

  // Same shape as AtlasEntry's guard: the prerender only writes files for
  // slugs that exist, so anything else arriving here client-side (a stale
  // link, a deleted listing after a takedown) goes back to the gallery.
  if (!listing) return <Navigate to="/atlas" replace />

  const groups = toolsByPage(listing)
  const checks = checksPassed(listing)
  const share = shareUrls(listing)
  const badge = badgeMarkdown(listing.slug, listing.name)
  const rescanHref = `/atlas?url=${encodeURIComponent(listing.url)}&rescan=1#submit`

  return (
    <>
      <Seo title={atlasTitle(listing.name)} description={listing.description} />
      <Navigation />
      <main className="atlas-listing" data-component="AtlasListing">
        <div className="container container--wide">
          <Link to="/atlas" className="blog-post__back">
            &larr; Back to the atlas
          </Link>

          <header className="atlas-listing__header" data-component="ListingHeader">
            <a
              className="atlas-entry__eyebrow"
              href={listing.url}
              target="_blank"
              // Listings are submitted, so the outbound link is nofollow ugc —
              // a listing is not an endorsement and must not pass ranking.
              rel="noreferrer nofollow ugc"
            >
              <AtlasMark domain={listing.host} />
              <span>{listing.host}</span>
              <span className="atlas-entry__eyebrow-out" aria-hidden="true">
                &#8599;
              </span>
            </a>
            <h1>{listing.name}</h1>

            <div className="atlas-listing__pills">
              <span className={`atlas-type atlas-type--${listing.type}`}>{listing.type}</span>
              <span className="atlas-type atlas-type--surface">{listing.surface} surface</span>
              {listing.built_with_sightkick && <span className="atlas-badge">Built with Sightkick</span>}
              {listing.labels.map((label) => (
                <span key={label} className="atlas-badge atlas-badge--label">
                  {label}
                </span>
              ))}
            </div>

            <p className="atlas-entry__desc">{listing.description}</p>

            <div className="atlas-entry__cats">
              {/* Links back into the gallery's own filter, so a category is a
                  way to find neighbours rather than a decorative tag. */}
              <Link to={`/atlas?category=${encodeURIComponent(listing.category)}`} className="atlas-chip">
                {listing.category}
              </Link>
            </div>
          </header>

          <div className="atlas-listing__layout">
            <div className="atlas-listing__main">
              <div className="atlas-facts" data-component="ListingFacts">
                <Fact label="Tools detected" value={listing.counts.tools} />
                <Fact label="Pages checked" value={listing.counts.pages} />
                <Fact label="Checks passed" value={`${checks.passed}/${checks.total}`} />
                <Fact
                  label="Last scanned"
                  value={<time dateTime={listing.scannedAt}>{formatScanned(listing.scannedAt)}</time>}
                />
                <Fact label="Listed since" value={<time dateTime={listing.added}>{formatScanned(listing.added)}</time>} />
              </div>

              {groups.length > 0 && (
                <section className="atlas-tools" aria-labelledby="atlas-tools-h" data-component="ListingTools">
                  <h2 id="atlas-tools-h" className="atlas-listing__h2">
                    Tools by page
                  </h2>
                  <p className="atlas-listing__note">
                    Grouped by the page each tool was first seen on — a WebMCP surface is
                    route-scoped, so what an agent can call depends on where it is standing. Kinds
                    are classified by Atlas and corrected on review.
                  </p>

                  {groups.map((group) => (
                    <div key={group.page} className="atlas-tools__group">
                      <div className="atlas-tools__page">
                        <code>{group.page}</code>
                        <span className="atlas-tools__page-count">
                          {group.tools.length} {group.tools.length === 1 ? 'tool' : 'tools'}
                        </span>
                      </div>

                      <ul className="atlas-tools__list">
                        {group.tools.map((tool) => (
                          <li key={tool.name} className="atlas-tool">
                            <div className="atlas-tool__head">
                              <code className="atlas-tool__name">{tool.name}</code>
                              <span className={kindClass(tool.risk)}>{kindLabel(tool.risk)}</span>
                            </div>
                            {/* Every string below is untrusted: it came off a
                                third-party page and is rendered as a React text
                                node. Never dangerouslySetInnerHTML here. */}
                            {tool.description && <p className="atlas-tool__desc">{tool.description}</p>}
                            {tool.riskReason && (
                              <p className="atlas-tool__why">Classified by Atlas: {tool.riskReason}</p>
                            )}
                            {tool.warnings.length > 0 && (
                              <ul className="atlas-tool__warnings">
                                {tool.warnings.map((warning) => (
                                  <li key={warning}>{warning}</li>
                                ))}
                              </ul>
                            )}
                            <details className="atlas-tool__schema">
                              <summary>Input schema</summary>
                              <pre>{JSON.stringify(tool.inputSchema, null, 2)}</pre>
                            </details>
                          </li>
                        ))}
                      </ul>
                    </div>
                  ))}
                </section>
              )}

              {listing.report.checks.length > 0 && (
                <section className="atlas-checks" aria-labelledby="atlas-checks-h" data-component="ListingChecks">
                  <h2 id="atlas-checks-h" className="atlas-listing__h2">
                    What the scan could check
                  </h2>
                  <p className="atlas-listing__note">
                    Each line is something the scan could observe. A failed check is a note, not a
                    score: {checks.passed} of {checks.total} passed on the last scan.
                  </p>
                  <ul className="atlas-checks__list">
                    {listing.report.checks.map((check) => (
                      <li key={check.id} className={`atlas-checks__item${check.ok ? '' : ' atlas-checks__item--off'}`}>
                        <span className="atlas-checks__glyph" aria-hidden="true">
                          {check.ok ? '✓' : '!'}
                        </span>
                        <span className="atlas-checks__label">{check.label}</span>
                        <span className="atlas-checks__detail">{check.detail}</span>
                      </li>
                    ))}
                  </ul>
                </section>
              )}

              {listing.suggested_journeys.length > 0 && (
                <section className="atlas-listing__block" aria-labelledby="atlas-journeys-h" data-component="ListingJourneys">
                  <h2 id="atlas-journeys-h" className="atlas-listing__h2">
                    Suggested journeys
                  </h2>
                  <p className="atlas-listing__note">
                    Read-only runs these tools would support. None has been run yet.
                  </p>
                  <ul className="atlas-listing__journeys">
                    {listing.suggested_journeys.map((journey) => (
                      <li key={journey.intent}>
                        <span className="atlas-listing__journey-intent">{journey.intent}</span>
                        <span className="atlas-listing__journey-tools">
                          {journey.tools.map((tool) => (
                            <code key={tool}>{tool}</code>
                          ))}
                        </span>
                      </li>
                    ))}
                  </ul>
                </section>
              )}

              {listing.journey && (
                <section className="atlas-listing__block" aria-labelledby="atlas-journey-h" data-component="ListingStoredJourney">
                  <h2 id="atlas-journey-h" className="atlas-listing__h2">
                    Stored journey
                  </h2>
                  <div className={`atlas-listing__journey atlas-listing__journey--${listing.journey.outcome}`}>
                    <p className="atlas-listing__journey-intent">{listing.journey.intent}</p>
                    <dl className="atlas-listing__journey-facts">
                      <div>
                        <dt>Outcome</dt>
                        <dd>{listing.journey.outcome}</dd>
                      </div>
                      <div>
                        <dt>Ran</dt>
                        <dd>
                          <time dateTime={listing.journey.ran_at}>{formatScanned(listing.journey.ran_at)}</time>
                        </dd>
                      </div>
                      <div>
                        <dt>Duration</dt>
                        <dd>{(listing.journey.duration_ms / 1000).toFixed(1)}s</dd>
                      </div>
                      <div>
                        <dt>Tools called</dt>
                        <dd>
                          {listing.journey.tools_called.map((tool) => (
                            <code key={tool}>{tool}</code>
                          ))}
                        </dd>
                      </div>
                    </dl>
                    {listing.journey.notes && <p className="atlas-listing__journey-notes">{listing.journey.notes}</p>}
                    <p className="atlas-listing__note">
                      One supervised, read-only run a maintainer performed on the date shown. It is
                      not a continuous test.
                    </p>
                  </div>
                </section>
              )}

              {listing.drift && (listing.drift.added.length > 0 || listing.drift.removed.length > 0) && (
                <section className="atlas-listing__block" aria-labelledby="atlas-drift-h" data-component="ListingDrift">
                  <h2 id="atlas-drift-h" className="atlas-listing__h2">
                    Since {formatScanned(listing.drift.since)}
                  </h2>
                  <p className="atlas-listing__note">What the latest rescan changed in the tool set.</p>
                  <div className="atlas-listing__drift">
                    {listing.drift.added.length > 0 && (
                      <div className="atlas-listing__drift-side atlas-listing__drift-side--added">
                        <span className="atlas-listing__drift-label">Added</span>
                        <ul>
                          {listing.drift.added.map((name) => (
                            <li key={name}>
                              <code>{name}</code>
                            </li>
                          ))}
                        </ul>
                      </div>
                    )}
                    {listing.drift.removed.length > 0 && (
                      <div className="atlas-listing__drift-side atlas-listing__drift-side--removed">
                        <span className="atlas-listing__drift-label">Removed</span>
                        <ul>
                          {listing.drift.removed.map((name) => (
                            <li key={name}>
                              <code>{name}</code>
                            </li>
                          ))}
                        </ul>
                      </div>
                    )}
                  </div>
                </section>
              )}

              {(listing.strengths.length > 0 || listing.improvements.length > 0) && (
                <section className="atlas-listing__block" aria-labelledby="atlas-review-h" data-component="ListingReview">
                  <h2 id="atlas-review-h" className="atlas-listing__h2">
                    From the review
                  </h2>
                  <div className="atlas-listing__cols">
                    {listing.strengths.length > 0 && (
                      <div>
                        <span className="atlas-listing__col-label">Strong</span>
                        <ul className="atlas-listing__bullets">
                          {listing.strengths.map((line) => (
                            <li key={line}>{line}</li>
                          ))}
                        </ul>
                      </div>
                    )}
                    {listing.improvements.length > 0 && (
                      <div>
                        <span className="atlas-listing__col-label">Could improve</span>
                        <ul className="atlas-listing__bullets">
                          {listing.improvements.map((line) => (
                            <li key={line}>{line}</li>
                          ))}
                        </ul>
                      </div>
                    )}
                  </div>
                </section>
              )}

              <section className="atlas-starter" aria-labelledby="atlas-starter-h" data-component="ListingStarter">
                <h2 id="atlas-starter-h" className="atlas-listing__h2">
                  {listing.starter.title}
                </h2>
                <p className="atlas-listing__note">{listing.starter.intro}</p>

                <div className="atlas-starter__code">
                  <div className="atlas-starter__bar">
                    <span className="atlas-starter__lang">{listing.starter.lang}</span>
                    <CopyButton
                      value={listing.starter.code}
                      label="Copy"
                      className="atlas-starter__copy"
                      title="Copy the starter to the clipboard"
                    />
                  </div>
                  <pre>
                    <code>{listing.starter.code}</code>
                  </pre>
                </div>

                <details className="atlas-starter__prompt">
                  <summary>Prompt for a coding agent</summary>
                  <div className="atlas-starter__code">
                    <div className="atlas-starter__bar">
                      <span className="atlas-starter__lang">prompt</span>
                      <CopyButton
                        value={listing.starter.prompt}
                        label="Copy"
                        className="atlas-starter__copy"
                        title="Copy the agent prompt to the clipboard"
                      />
                    </div>
                    <pre>
                      <code>{listing.starter.prompt}</code>
                    </pre>
                  </div>
                </details>
              </section>

              <section className="atlas-share" aria-labelledby="atlas-share-h" data-component="ListingShare">
                <h2 id="atlas-share-h" className="atlas-listing__h2">
                  Share this listing
                </h2>
                <div className="atlas-share__links">
                  <a className="atlas-share__link" href={share.x} target="_blank" rel="noreferrer">
                    Share on X
                  </a>
                  <a className="atlas-share__link" href={share.linkedin} target="_blank" rel="noreferrer">
                    Share on LinkedIn
                  </a>
                  <CopyButton value={share.url} label="Copy link" className="atlas-share__link" title="Copy the listing URL" />
                </div>

                <div className="atlas-share__badge">
                  <img
                    src={`/atlas/${listing.slug}/badge.svg`}
                    alt={`${listing.name} on Sightmap Atlas`}
                    width={220}
                    height={20}
                    loading="lazy"
                    decoding="async"
                  />
                  <div className="atlas-share__snippet">
                    <pre>
                      <code>{badge}</code>
                    </pre>
                    <CopyButton value={badge} label="Copy" className="atlas-starter__copy" title="Copy the badge markdown" />
                  </div>
                </div>
              </section>

              <p className="atlas-listing__footnote">
                Classification is Atlas&rsquo;s own and is corrected by a maintainer on review. A
                listing records the tools a scan detected on the date shown, and says nothing about
                the site beyond that.
              </p>
            </div>

            <aside className="atlas-meta" aria-label="Listing metadata" data-component="ListingMeta">
              <dl>
                <MetaRow label="Type">{listing.type}</MetaRow>
                <MetaRow label="Surface">{listing.surface}</MetaRow>
                <MetaRow label="Status">{listing.status}</MetaRow>
                <MetaRow label="Scanned">
                  <time dateTime={listing.scannedAt}>{formatScanned(listing.scannedAt)}</time>
                </MetaRow>
                <MetaRow label="Scanner">
                  {listing.report.scanner.name} {listing.report.scanner.version}
                </MetaRow>
                <MetaRow label="Submitted by">{listing.submitted_by}</MetaRow>
                <MetaRow label="Listed">
                  <time dateTime={listing.added}>{formatScanned(listing.added)}</time>
                </MetaRow>
                <MetaRow label="Updated">
                  <time dateTime={listing.updated}>{formatScanned(listing.updated)}</time>
                </MetaRow>
              </dl>

              <div className="atlas-meta__machine">
                <div className="atlas-meta__machine-label">Machine-readable</div>
                <a href={`/atlas/sites/${listing.slug}.json`}>/atlas/sites/{listing.slug}.json</a>
                <a href={`/atlas/sites/${listing.slug}/tools.json`}>/atlas/sites/{listing.slug}/tools.json</a>
                <a href={`/atlas/scans/${listing.slug}.json`}>/atlas/scans/{listing.slug}.json</a>
                <a href={`/atlas/${listing.slug}.md`}>/atlas/{listing.slug}.md</a>
              </div>

              <div className="atlas-meta__machine atlas-listing__actions">
                {/* A plain <a>, not a <Link>: a full navigation is what makes
                    the browser honour the #submit fragment and land on the form
                    already prefilled. */}
                <a href={rescanHref}>Request a rescan</a>
                <a href={`mailto:atlas@sightmap.org?subject=${encodeURIComponent(`Atlas listing: ${listing.slug}`)}`}>
                  Report a problem
                </a>
              </div>
            </aside>
          </div>
        </div>
      </main>
      <Footer />
    </>
  )
}

/** Convenience for callers that only have the slug (the route dispatcher). */
export function listingBySlug(slug: string): DirectoryListingView | undefined {
  return directoryListings.find((l) => l.slug === slug)
}
