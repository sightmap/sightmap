// The contribution guide for the atlas repo, which is where an entry actually
// gets authored — the corpus, screenshots and README all land in a PR against
// sightmap/atlas, not this repo. Deep-linked to CONTRIBUTING.md rather than the
// repo root so the first thing a would-be contributor sees is the checklist.
const CONTRIBUTING_URL = 'https://github.com/sightmap/atlas/blob/main/CONTRIBUTING.md'

/**
 * The trailing card in the gallery grid: an invitation to add an entry rather
 * than an entry itself. It reuses `.atlas-card`'s shell so the grid's rhythm
 * holds, and diverges only where it should read as an action instead of a
 * listing — dashed edges, an accent plus in place of a screenshot, and no
 * stats line, since it has nothing to count.
 */
export default function AtlasSubmitCard() {
  return (
    <a
      className="atlas-card atlas-card--submit"
      href={CONTRIBUTING_URL}
      target="_blank"
      rel="noreferrer"
      data-component="AtlasSubmitCard"
    >
      <div className="atlas-card__shot">
        <span className="atlas-card__plus" aria-hidden="true">
          +
        </span>
      </div>

      <div className="atlas-card__body">
        <div className="atlas-card__head">
          {/* Not <AtlasMark>: that derives its letter and colour from a domain,
              and this card stands for no site in particular. */}
          <span className="atlas-mark atlas-card__submit-mark" aria-hidden="true">
            +
          </span>
          <span className="atlas-card__name">Submit a site</span>
          <span className="atlas-card__method">PR</span>
        </div>

        <p className="atlas-card__desc">
          Map a site by browsing it, then add the corpus, screenshots and README to
          sightmap/atlas and open a pull request. The guide covers the entry layout, the
          local validator, and what a site has to meet to be listed.
        </p>

        <div className="atlas-card__foot">
          <span>CONTRIBUTING.md</span>
          <span aria-hidden="true">&#8599;</span>
        </div>
      </div>
    </a>
  )
}
