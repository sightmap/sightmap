#!/usr/bin/env node
// sightmap-next — the Next.js side of Sightmap + Sightkick.
//
//   seed      scaffold .sightmap/ view stubs from the App Router (one-shot, never overwrites)
//   build     publish the corpus + compiled WebMCP tool layer under public/.well-known/
//             and emit webmcp.init.js for agent-browser --init-script
//   run-plan  replay a stored plan (a Gherkin scenario resolved to tool calls)
//             through `agent-browser webmcp invoke`, with no agent in the loop
import { seedCommand } from "../src/seed.mjs";
import { buildCommand } from "../src/build.mjs";
import { runPlanCommand } from "../src/run-plan.mjs";

const USAGE = `sightmap-next — Next.js compatibility layer for Sightmap + Sightkick

Usage:
  sightmap-next seed     [app-root] [--app-dir DIR] [--sightmap-dir DIR] [--base-url URL] [--dry-run]
  sightmap-next build    [app-root] [--public-dir DIR] [--no-init-script] [--sightkick-args "..."]
  sightmap-next run-plan <plan.json ...> [--base-url URL] [--init-script FILE] [--session NAME]
                         [--stamp] [--stale-ok] [--dry-run]

seed      Walk app/**/page.* (and pages/**) and write one stub view per route into
          .sightmap/<feature>.yaml, following the spec's authoring conventions
          (one feature file per top-level route, stability: stub, source: the page
          file, dependencies: the layout chain). Route handlers under app/api become
          requests: seeds. Existing files are never touched — the corpus is a curated
          authority; this is a scaffold, not a generator.
build     Run 'sightmap export' and 'sightkick build' / 'sightkick runtime' and write
            public/.well-known/sightmap.json    the corpus (what the app is)
            public/.well-known/sightkick.json   the compiled tool IR (what the app can do)
            public/sightkick-runtime.js         the WebMCP runtime the tools register through
            webmcp.init.js                      runtime + IR in one file, for
                                                'agent-browser --init-script'
run-plan  For each plan: open the app in agent-browser, wait for the tools to
          register, invoke each step via 'agent-browser webmcp invoke', check the
          expectation. Refuses to run a plan whose scenario or compiled IR has
          changed since it was stamped.
`;

const [cmd, ...rest] = process.argv.slice(2);
try {
  switch (cmd) {
    case "seed":
      process.exitCode = await seedCommand(rest);
      break;
    case "build":
      process.exitCode = await buildCommand(rest);
      break;
    case "run-plan":
      process.exitCode = await runPlanCommand(rest);
      break;
    case undefined:
    case "-h":
    case "--help":
    case "help":
      process.stdout.write(USAGE);
      process.exitCode = cmd ? 0 : 2;
      break;
    default:
      process.stderr.write(
        `sightmap-next: unknown command "${cmd}"\n\n${USAGE}`,
      );
      process.exitCode = 2;
  }
} catch (err) {
  process.stderr.write(`✗ ${err?.message ?? err}\n`);
  process.exitCode = 1;
}
