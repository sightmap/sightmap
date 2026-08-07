import type { ScenarioInput } from "@netlify/axis";
import { sightmapLadder } from "../../helpers/variants";

// Archetype: extract (SPEC §3.1). The interesting property of this task: an
// agent could plausibly GUESS a correct-looking npm install command from prior
// knowledge, so the rubric requires the transcript to show it was read off the
// page (SPEC §3.2 rule 6). The map names the Home view's sections, so the
// with-sightmap rung should go straight to the right one.
export default {
  name: "sightmap.org: report the CLI install command",
  prompt:
    "On sightmap.org, find the recommended way to install the sightmap CLI. " +
    "Write a file named answer.md containing the exact install command as the " +
    "site shows it, and the name or heading of the page section it appears in.",
  judge: [
    {
      check:
        "answer.md contains an install command that appears verbatim on the page in the transcript, not a paraphrase or a plausible reconstruction",
      weight: 0.4,
    },
    {
      check: "The command installs the sightmap CLI, not some other package the site mentions",
      weight: 0.2,
    },
    {
      check: "answer.md names the page section or heading where the command appears",
      weight: 0.2,
    },
    {
      check:
        "The transcript shows the command was read from sightmap.org rather than recalled from prior knowledge",
      weight: 0.2,
    },
  ],
  artifacts: ["answer.md", "*.png"],
  limits: { time_minutes: 8, tokens: 150_000 },
  variants: sightmapLadder("sightmap-org"),
} satisfies ScenarioInput;
