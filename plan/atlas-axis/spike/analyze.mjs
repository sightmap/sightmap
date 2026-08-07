#!/usr/bin/env node
// Aggregates an AXIS report into the spike's two readouts (PLAN P6.0):
//   1. per-rung medians of the AXIS composite and dimensions, plus the two lifts
//   2. harness-independent operational metrics from the same runs
//      (completion, duration, tokens) — if these disagree with the AXIS lift,
//      that disagreement is the finding.
//
// report.json's schema is not a published contract, so this reads defensively:
// it hunts for per-job entries carrying a scenario key, scores, and usage, and
// prints whatever it can ground. Adjust field paths against the first real
// report if they have drifted.
//
// Usage: node analyze.mjs [path-to-report.json]   (default: latest report)

import fs from "node:fs";
import path from "node:path";

const RUNGS = ["no-context", "with-skills", "with-sightmap"];

function latestReport() {
  const dir = ".axis/reports";
  const ids = fs.existsSync(dir) ? fs.readdirSync(dir).sort() : [];
  if (!ids.length) throw new Error("no reports under .axis/reports — run `npx axis run` first");
  return path.join(dir, ids[ids.length - 1], "report.json");
}

const file = process.argv[2] ?? latestReport();
const manifest = JSON.parse(fs.readFileSync(file, "utf8"));

// Collect anything that looks like a per-job result: an object with a scenario
// key containing "@<rung>-r<N>". Walk the whole manifest so we don't depend on
// the exact nesting.
const jobs = [];
(function walk(node) {
  if (Array.isArray(node)) return node.forEach(walk);
  if (node && typeof node === "object") {
    const key = node.scenario ?? node.scenarioKey ?? node.key;
    if (typeof key === "string" && /@(no-context|with-skills|with-sightmap)-r\d+$/.test(key)) {
      jobs.push({ key, raw: node });
    }
    Object.values(node).forEach(walk);
  }
})(manifest);

if (!jobs.length) {
  console.error(`no per-job entries recognized in ${file} — inspect it by hand and fix the key regex here`);
  process.exit(1);
}

const num = (v) => (typeof v === "number" && Number.isFinite(v) ? v : undefined);
// Pull a numeric field by trying several plausible names/paths.
function pick(obj, names) {
  for (const n of names) {
    const v = n.split(".").reduce((o, k) => (o ? o[k] : undefined), obj);
    const x = num(v);
    if (x !== undefined) return x;
  }
  return undefined;
}

for (const j of jobs) {
  const m = j.key.match(/^(.*)@([a-z-]+)-r(\d+)$/);
  j.base = m[1];
  j.rung = m[2];
  j.failed = j.raw.failed === true;
  j.composite = pick(j.raw, ["result", "axisResult", "score", "scores.result", "scores.composite"]);
  j.goal = pick(j.raw, ["scores.goal_achievement", "scores.goalAchievement", "goal_achievement", "dimensions.goal_achievement"]);
  j.environment = pick(j.raw, ["scores.environment", "environment", "dimensions.environment"]);
  j.service = pick(j.raw, ["scores.service", "service", "dimensions.service"]);
  j.agentDim = pick(j.raw, ["scores.agent", "agent_score", "dimensions.agent"]);
  j.tokens = pick(j.raw, ["tokens.total", "usage.total_tokens", "usage.totalTokens", "totalTokens", "tokens"]);
  j.durationMs = pick(j.raw, ["duration_ms", "durationMs", "duration"]);
}

const median = (xs) => {
  const s = xs.filter((x) => x !== undefined).sort((a, b) => a - b);
  if (!s.length) return undefined;
  const mid = s.length >> 1;
  return s.length % 2 ? s[mid] : (s[mid - 1] + s[mid]) / 2;
};
const iqr = (xs) => {
  const s = xs.filter((x) => x !== undefined).sort((a, b) => a - b);
  if (s.length < 2) return 0;
  const q = (p) => s[Math.min(s.length - 1, Math.floor(p * (s.length - 1)))];
  return Math.round((q(0.75) - q(0.25)) * 10) / 10;
};
const fmt = (v) => (v === undefined ? "—" : Math.round(v * 10) / 10);

const bases = [...new Set(jobs.map((j) => j.base))].sort();
for (const base of bases) {
  console.log(`\n=== ${base} ===`);
  console.log("rung            n  fail  composite  goal  env  svc  agent  spread  med.tokens  med.min");
  const rungStats = {};
  for (const rung of RUNGS) {
    const rows = jobs.filter((j) => j.base === base && j.rung === rung);
    const ok = rows.filter((j) => !j.failed);
    const st = {
      composite: median(ok.map((j) => j.composite)),
      goal: median(ok.map((j) => j.goal)),
      env: median(ok.map((j) => j.environment)),
      svc: median(ok.map((j) => j.service)),
      agent: median(ok.map((j) => j.agentDim)),
      spread: iqr(ok.map((j) => j.composite)),
      tokens: median(ok.map((j) => j.tokens)),
      mins: median(ok.map((j) => j.durationMs).map((d) => (d === undefined ? undefined : d / 60000))),
    };
    rungStats[rung] = st;
    console.log(
      `${rung.padEnd(14)} ${String(rows.length).padStart(2)}  ${String(rows.length - ok.length).padStart(4)}  ` +
        `${String(fmt(st.composite)).padStart(9)}  ${String(fmt(st.goal)).padStart(4)}  ${String(fmt(st.env)).padStart(3)}  ` +
        `${String(fmt(st.svc)).padStart(3)}  ${String(fmt(st.agent)).padStart(5)}  ${String(st.spread).padStart(6)}  ` +
        `${String(fmt(st.tokens)).padStart(10)}  ${String(fmt(st.mins)).padStart(7)}`,
    );
  }
  const s = rungStats["with-skills"], m = rungStats["with-sightmap"], n0 = rungStats["no-context"];
  if (s?.composite !== undefined && m?.composite !== undefined) {
    const lift = fmt(m.composite - s.composite);
    const clear = Math.abs(m.composite - s.composite) > Math.max(s.spread, m.spread);
    console.log(`lift (map − skills): ${lift}  [goal ${fmt(m.goal - s.goal)}, agent ${fmt(m.agent - s.agent)}]  confidence: ${clear ? "clear" : "weak"}`);
  }
  if (n0?.composite !== undefined && m?.composite !== undefined) {
    console.log(`lift_total (map − no-context): ${fmt(m.composite - n0.composite)}`);
  }
}
console.log(
  "\nRemember SPEC §2.6: an AXIS lift that disagrees with the raw token/duration/completion\nmovement is a finding about the instrument, not about the map.",
);
