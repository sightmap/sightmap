import type { ScenarioInput } from "@netlify/axis";
import { sightmapLadder } from "../../helpers/variants";

// Archetype: traverse (SPEC §3.1): home → search → listing detail. The corpus
// maps SearchResults (/s/:location/homes) and ListingDetail (/rooms/:listingId)
// with named sections (TitleSection, OverviewSection, …), which is exactly what
// the third check asks the agent to produce.
export default {
  name: "Airbnb: reach a listing detail page for a Paris stay",
  prompt:
    "Starting from airbnb.com, find stays in Paris and open the page for any one " +
    "listing. Write a file named answer.md containing: the listing's title, the " +
    "URL of its page, and the names of three distinct sections that appear on " +
    "that page. Do not sign in, book, or submit any forms.",
  judge: [
    {
      check:
        "answer.md identifies a specific listing detail page (its /rooms/<id> URL or equivalent) as the source of the answer, not a search-results page",
      weight: 0.4,
    },
    {
      check: "The listing title in answer.md matches what that page shows in the transcript",
      weight: 0.2,
    },
    {
      check:
        "answer.md names three distinct sections that genuinely appear on the listing page per the transcript (any three real ones count)",
      weight: 0.3,
    },
    {
      check: "Did NOT sign in, attempt a booking, or submit any form",
      weight: 0.1,
    },
  ],
  artifacts: ["answer.md", "*.png"],
  limits: { time_minutes: 12, tokens: 250_000 },
  variants: sightmapLadder("airbnb"),
} satisfies ScenarioInput;
