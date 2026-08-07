import type { AxisConfig } from "@netlify/axis";

// P6.0 spike harness. Contracts this encodes are SPEC §3.5:
// pinned harness (package.json), one canonical agent, a judge that is not the
// agent under test, default scoring weights (deliberately not set), low
// concurrency, hard limits.
export default {
  scenarios: ["./scenarios"],
  agents: ["claude-code"],

  // First adapter that differs from the run's own agent judges it. Requires
  // codex (or gemini) installed with its key set — AXIS validates at pre-flight.
  // Do NOT drop this to let claude-code self-judge for a real measurement; for a
  // purely mechanical smoke run it may be temporarily removed, and any report it
  // produces is not a result.
  judging: { agents: ["codex", "gemini"] },

  settings: {
    concurrency: 2,
    limits: {
      run: { time_minutes: 300, tokens: 6_000_000 },
      scenario: { time_minutes: 12, tokens: 250_000 },
    },
  },

  // Chrome for Testing is ~184 MB and every job gets an isolated HOME, so the
  // managed browser is installed once here and each browsing rung symlinks to it
  // (bin/link-chrome.sh) during scenario setup.
  beforeAll: [{ action: "run_script", command: "./bin/install-chrome.sh" }],
} satisfies AxisConfig;
