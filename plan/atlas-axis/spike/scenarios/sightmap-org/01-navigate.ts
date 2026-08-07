import type { ScenarioInput } from "@netlify/axis";
import { sightmapLadder } from "../../helpers/variants";

// Archetype: navigate (SPEC §3.1). The corpus names BlogIndex (/blog) and
// BlogPost (/blog/:slug) with BlogPostTitle/BlogPostDate components — the map
// should turn "find the blog" into a route lookup.
export default {
  name: "sightmap.org: open the most recent blog post",
  prompt:
    "On sightmap.org, find the blog and open its most recent post. Write a file " +
    "named answer.md containing: the post's title, its publication date, and the " +
    "URL of the post's own page.",
  judge: [
    {
      check:
        "answer.md exists and names a real post from sightmap.org's blog, with the URL of that post's own page (a path under /blog/), not the blog index",
      weight: 0.4,
    },
    {
      check:
        "The publication date in answer.md matches what the post page shows in the transcript",
      weight: 0.3,
    },
    {
      check:
        "The transcript shows the title and date were read from the site, not guessed or recalled",
      weight: 0.3,
    },
  ],
  artifacts: ["answer.md", "*.png"],
  limits: { time_minutes: 8, tokens: 150_000 },
  variants: sightmapLadder("sightmap-org"),
} satisfies ScenarioInput;
