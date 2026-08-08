import { Link, Navigate, useParams } from 'react-router'
import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import Seo from '@/components/Seo'
import AtlasMark from '@/components/atlas/AtlasMark'
import AtlasInstall from '@/components/atlas/AtlasInstall'
import { formatDate } from '@/components/BlogCard'
import { figLabel, primaryDomain } from '@/lib/atlas'
import { atlasEntries } from '@/generated/atlas-manifest'
// Shared with scripts/prerender.tsx so a client-side navigation from /atlas
// and a fresh load of the same entry produce an identical <title>.
import { atlasTitle } from '../../scripts/lib/site'

/**
 * One row of the monospace metadata sidebar. Rendered only when there is a
 * value: `commit` is optional in the schema, and a label with an empty slot
 * under it reads as a bug rather than as "not applicable".
 */
function MetaRow({ label, children }: { label: string; children: React.ReactNode }) {
  if (children === null || children === undefined || children === '') return null
  return (
    <div className="atlas-meta__row">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  )
}

export default function AtlasEntryPage() {
  const { slug = '' } = useParams()
  const entry = atlasEntries.find((e) => e.slug === slug)

  // Same shape as BlogPost's guard: an unknown slug has no page of its own
  // (the prerender only writes files for entries that exist), so anything else
  // arriving here client-side goes back to the gallery.
  if (!entry) return <Navigate to="/atlas" replace />

  const domain = primaryDomain(entry)

  return (
    <>
      <Seo title={atlasTitle(entry.name)} description={entry.description} />
      <Navigation />
      <main className="atlas-entry" data-component="AtlasEntry">
        <div className="container container--wide">
          <Link to="/atlas" className="blog-post__back">
            &larr; Back to the atlas
          </Link>

          <header className="atlas-entry__header">
            <a
              className="atlas-entry__eyebrow"
              href={entry.site_url}
              target="_blank"
              rel="noreferrer nofollow ugc"
            >
              <AtlasMark domain={domain} />
              <span>{domain}</span>
              <span className="atlas-entry__eyebrow-out" aria-hidden="true">
                &#8599;
              </span>
            </a>
            <h1>{entry.name}</h1>
            <AtlasInstall slug={entry.slug} />
            <p className="atlas-entry__desc">{entry.description}</p>
            {entry.categories.length > 0 && (
              <div className="atlas-entry__cats">
                {entry.categories.map((cat) => (
                  // Links back into the gallery's own filter, so a category is
                  // a way to find neighbours rather than a decorative tag.
                  <Link key={cat} to={`/atlas?category=${encodeURIComponent(cat)}`} className="atlas-chip">
                    {cat}
                  </Link>
                ))}
              </div>
            )}
          </header>

          <div className="atlas-entry__layout">
            <div className="atlas-entry__main">
              {entry.screenshotUrls.length > 0 && (
                <section className="atlas-figs" aria-label="Screenshots">
                  {entry.screenshotUrls.map((url, i) => (
                    <figure key={url} className="atlas-fig">
                      <img
                        src={url}
                        alt={`${entry.name} screenshot ${i + 1}`}
                        loading={i === 0 ? 'eager' : 'lazy'}
                        decoding="async"
                      />
                      <figcaption>
                        <span className="atlas-fig__label">{figLabel(i)}</span>
                        {/* The filename is the only caption the schema gives us,
                            and the vendored names are descriptive (01-home,
                            02-blog-index), so it beats inventing prose. */}
                        <span className="atlas-fig__name">
                          {url.split('/').pop()?.replace(/\.\w+$/, '').replace(/^\d+-/, '').replace(/-/g, ' ')}
                        </span>
                      </figcaption>
                    </figure>
                  ))}
                </section>
              )}

              {entry.per_view.length > 0 && (
                <section className="atlas-views" aria-labelledby="atlas-views-h">
                  <h2 id="atlas-views-h" className="atlas-entry__h2">
                    Views
                  </h2>
                  {/* Wrapped so a long route can scroll the table instead of
                      the page — see .atlas-table-wrap in index.css. */}
                  <div className="atlas-table-wrap">
                    <table className="atlas-table">
                      <thead>
                        <tr>
                          <th scope="col">View</th>
                          <th scope="col">Route</th>
                          <th scope="col" className="atlas-table__num">
                            Components
                          </th>
                          <th scope="col" className="atlas-table__num">
                            Requests
                          </th>
                        </tr>
                      </thead>
                      <tbody>
                        {entry.per_view.map((view) => (
                          <tr key={`${view.name}:${view.route}`}>
                            <td>{view.name}</td>
                            <td>
                              <code>{view.route}</code>
                            </td>
                            <td className="atlas-table__num">{view.components}</td>
                            <td className="atlas-table__num">{view.requests}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </section>
              )}

              {entry.bodyHtml && (
                // Not `renderPostBody`: that path exists to swap blog widget
                // markers, and this HTML comes from scripts/lib/atlas.ts, whose
                // renderer escapes raw HTML and allowlists URL schemes exactly
                // so community markdown can be dropped in as-is. See the note
                // at the top of that file.
                <section
                  className="prose atlas-body"
                  dangerouslySetInnerHTML={{ __html: entry.bodyHtml }}
                />
              )}
            </div>

            <aside className="atlas-meta" aria-label="Entry metadata">
              <dl>
                <MetaRow label="Capture method">{entry.method}</MetaRow>
                <MetaRow label="Auth">{entry.auth}</MetaRow>
                <MetaRow label="Last verified">
                  <time dateTime={entry.last_verified}>{formatDate(entry.last_verified)}</time>
                </MetaRow>
                <MetaRow label="CLI version">{entry.cli_version}</MetaRow>
                <MetaRow label="Spec version">v{entry.spec_version}</MetaRow>
                <MetaRow label="Author">{entry.author}</MetaRow>
                {/* Optional in the schema. Shown short, like a git log. */}
                <MetaRow label="Commit">
                  {entry.commit ? <code>{entry.commit.slice(0, 12)}</code> : ''}
                </MetaRow>
              </dl>

              <div className="atlas-meta__machine">
                <div className="atlas-meta__machine-label">Machine-readable</div>
                <a href={`/atlas/${entry.slug}.md`}>
                  /atlas/{entry.slug}.md
                </a>
                <a href="/atlas/index.json">/atlas/index.json</a>
              </div>
            </aside>
          </div>
        </div>
      </main>
      <Footer />
    </>
  )
}
