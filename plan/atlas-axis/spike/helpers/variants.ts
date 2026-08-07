import type { ScenarioInput } from "@netlify/axis";

type Variant = NonNullable<ScenarioInput["variants"]>[number];

// The canonical skills live at the monorepo root, three levels above this spike
// (plan/atlas-axis/spike → repo root). AXIS resolves local skill paths relative
// to the config file. If AXIS's skills field turns out not to reach the agent
// (a named P6.0 question), the fallback is a setup step:
//   sightmap skills install --target "$HOME/.claude/skills"
const SIGHTMAP_SKILLS = [
  "../../../skills/sightmap-browser",
  "../../../skills/sightmap-authoring",
];

// Make the shared Chrome visible inside this job's isolated HOME.
const chrome = {
  action: "run_script",
  command: '"$AXIS_CONFIG_DIR/bin/link-chrome.sh"',
} as const;

// Stage the entry's corpus into the workspace. The spike copies from vendored
// fixtures rather than running `sightmap atlas add` because the atlas index is
// not published yet; the production ladder (SPEC §3.4) installs from the atlas.
function corpus(slug: string) {
  return {
    action: "copy",
    match: `./fixtures/${slug}/.sightmap/**/*`,
    destination: "./.sightmap",
  } as const;
}

// The three-rung ladder (SPEC §2.2), with repetitions expanded as variants so a
// single `axis run` carries the whole matrix in one report. Reps per rung follow
// SPEC §4.1: no-context is context for lift_total, not the instrument, and is
// the least controlled rung — extra repetitions there buy noise.
//
// Variant `setup` REPLACES the parent's rather than merging, so each rung
// restates exactly what it needs. `no-context` gets nothing on purpose: whatever
// the agent does in a bare workspace (including bootstrapping its own tooling)
// is the status quo being measured.
export function sightmapLadder(
  slug: string,
  reps: { noContext: number; withSkills: number; withSightmap: number } = {
    noContext: 2,
    withSkills: 3,
    withSightmap: 3,
  },
): Variant[] {
  const out: Variant[] = [];
  for (let i = 1; i <= reps.noContext; i++) {
    out.push({ name: `no-context-r${i}`, setup: [] });
  }
  for (let i = 1; i <= reps.withSkills; i++) {
    out.push({ name: `with-skills-r${i}`, skills: SIGHTMAP_SKILLS, setup: [chrome] });
  }
  for (let i = 1; i <= reps.withSightmap; i++) {
    out.push({
      name: `with-sightmap-r${i}`,
      skills: SIGHTMAP_SKILLS,
      setup: [chrome, corpus(slug)],
    });
  }
  return out;
}
