import type { ScenarioInput } from "@netlify/axis";
import { sightmapLadder } from "../../helpers/variants";

// Archetype: extract (SPEC §3.1) — the SPEC §3.4 reference scenario. Prices
// drift, so the rubric asks for the shape (a currency amount read off the page)
// rather than a value (SPEC §3.2 rule 5).
export default {
  name: "Airbnb: read the nightly price on a listing page",
  prompt:
    "On airbnb.com, open any listing in Paris and report the nightly price shown " +
    "on the listing page, plus the name of the page section it appears in. Write " +
    "your findings to a file named answer.md, including the URL of the listing " +
    "you used. Do not sign in, book, or submit any forms.",
  judge: [
    {
      check:
        "answer.md identifies a specific listing detail page (its /rooms/<id> URL) as the source of the answer",
      weight: 0.3,
    },
    {
      check:
        "Reports a nightly price as a currency amount actually read off that page (visible in the transcript), not inferred from search results",
      weight: 0.4,
    },
    {
      check: "Names the page region the price appears in",
      weight: 0.2,
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
