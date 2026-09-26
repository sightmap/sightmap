// The contribution guide for the atlas repo, which is where a *community map*
// actually gets authored — the corpus, screenshots and README all land in a PR
// against sightmap/atlas, not this repo. Deep-linked to CONTRIBUTING.md rather
// than the repo root so the first thing a would-be contributor sees is the
// checklist. It is the secondary path now: the primary one is the scan form
// below the grid, which needs no PR and no corpus.
const CONTRIBUTING_URL = 'https://github.com/sightmap/atlas/blob/main/CONTRIBUTING.md'

/**
 * The trailing card in the gallery grid: an invitation to submit a site rather
 * than a listing itself. It reuses `.atlas-card`'s shell so the grid's rhythm
 * holds, and diverges only where it should read as an action instead of a
 * listing — dashed edges, an accent plus in place of a screenshot, and no
 * stats line, since it has nothing to count.
 *
 * The card is a wrapper rather than one big `<a>` because it carries two
 * destinations: the in-page jump to the scan form (the primary one — a visitor
 * who wants their own site scanned should land on the form, not on someone
 * else's contribution checklist) and the community-map guide underneath it.
 * Nesting the second inside the first would be invalid HTML.
 */
export default function AtlasSubmitCard() {
  return (
    <div className="atlas-card atlas-card--submit" data-component="AtlasSubmitCard">
      <a className="atlas-card__submit-link" href="#submit">
        <div className="atlas-card__shot">
          <span className="atlas-card__plus" aria-hidden="true">
            +
          </span>
        </div>

        <div className="atlas-card__body">
          <div className="atlas-card__head">
            {/* Not <AtlasMark>: that derives its letter and colour from a
                domain, and this card stands for no site in particular. */}
            <span className="atlas-mark atlas-card__submit-mark" aria-hidden="true">
              +
            </span>
            <span className="atlas-card__name">Submit a site</span>
          </div>

          <p className="atlas-card__desc">
            Send a URL for a free scan. It enumerates the WebMCP tools your pages register without
            calling any of them.
          </p>

          <div className="atlas-card__foot">
            <span>Scan my site</span>
            <span aria-hidden="true">&darr;</span>
          </div>
        </div>
      </a>

      <p className="atlas-card__submit-aside">
        Mapping someone else&rsquo;s site instead? Community maps arrive by pull request —{' '}
        <a href={CONTRIBUTING_URL} target="_blank" rel="noreferrer">
          CONTRIBUTING.md
        </a>
        .
      </p>
    </div>
  )
}
