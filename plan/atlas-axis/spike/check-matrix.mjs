// Load each scenario module the way AXIS does (jiti) and print the job matrix.
import { createJiti } from "jiti";
const jiti = createJiti(import.meta.url);
const files = [
  "./scenarios/sightmap-org/01-navigate.ts",
  "./scenarios/sightmap-org/02-extract.ts",
  "./scenarios/airbnb/01-traverse.ts",
  "./scenarios/airbnb/02-extract.ts",
];
let total = 0;
for (const f of files) {
  const mod = await jiti.import(f);
  const s = mod.default;
  const rungs = {};
  for (const v of s.variants) rungs[v.name.replace(/-r\d+$/, "")] = (rungs[v.name.replace(/-r\d+$/, "")] ?? 0) + 1;
  const w = s.judge.reduce((a, c) => a + (c.weight ?? 0), 0);
  total += s.variants.length;
  console.log(`${f.replace("./scenarios/", "")}: ${s.variants.length} jobs ${JSON.stringify(rungs)} judge=${s.judge.length} checks (Σw=${w}) limits=${s.limits.time_minutes}m/${s.limits.tokens}tok`);
  for (const v of s.variants) {
    if (v.prompt || v.judge) throw new Error(`${f} variant ${v.name} overrides prompt/judge — invalid per SPEC §3.2`);
  }
}
console.log(`TOTAL: ${total} jobs`);
