// seed — scaffold .sightmap/ view stubs from a Next.js router tree.
//
// This is deliberately a one-shot scaffold and not a build step. The Sightmap
// spec's authoring conventions make `.sightmap/` a curated authority: nothing
// regenerates it from source, and discovery happens against the running app.
// What a router tree *can* contribute deterministically is exactly what the
// `sightmap-authoring` skill tells an agent to seed by hand — the route table
// (`route:`), the page file (`source:`), and the layout chain
// (`dependencies:`). Everything else (selectors, properties, memory) needs the
// live loop. So: seed once, mark every view `stability: stub`, never overwrite.
import {
  readdirSync,
  statSync,
  existsSync,
  mkdirSync,
  writeFileSync,
  readFileSync,
} from "node:fs";
import { join, relative, sep, basename } from "node:path";
import { parseArgs } from "./args.mjs";

const PAGE_RE = /^page\.(js|jsx|ts|tsx|md|mdx)$/;
const LAYOUT_RE = /^layout\.(js|jsx|ts|tsx)$/;
const ROUTE_RE = /^route\.(js|jsx|ts|tsx)$/;
const HTTP_METHODS = [
  "GET",
  "HEAD",
  "POST",
  "PUT",
  "DELETE",
  "PATCH",
  "OPTIONS",
];

/** Convert one App Router directory segment to its URL form, or null to skip the subtree. */
export function segmentToRoute(seg) {
  if (seg.startsWith("_")) return null; // private folder: excluded from routing
  if (seg.startsWith("@")) return null; // parallel route slot: rendered inside its parent
  if (/^\(\.{1,3}\)/.test(seg)) return null; // intercepting route: a view of another route
  if (/^\(.*\)$/.test(seg)) return ""; // route group: no URL segment
  if (/^\[\[\.\.\..+\]\]$/.test(seg)) return "**"; // optional catch-all
  if (/^\[\.\.\..+\]$/.test(seg)) return "**"; // catch-all
  const m = /^\[(.+)\]$/.exec(seg);
  if (m) return `:${m[1]}`; // dynamic segment — the spec's Express-style param form
  return seg;
}

function pascal(s) {
  return s
    .replace(/[^a-zA-Z0-9]+/g, " ")
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .map((w) => w[0].toUpperCase() + w.slice(1))
    .join("");
}

/** A readable, deterministic view name from a route: `/` → Home, `/tasks/:id` → TasksById. */
export function routeToName(route) {
  if (route === "/") return "Home";
  const parts = route.split("/").filter(Boolean);
  const out = [];
  for (const p of parts) {
    if (p === "**") out.push("CatchAll");
    else if (p.startsWith(":")) out.push("By" + pascal(p.slice(1)));
    else out.push(pascal(p));
  }
  return out.join("") || "Home";
}

/** The feature file a route belongs to, per the spec's "one feature file per top-level route". */
export function routeToFeature(route) {
  const first = route.split("/").filter(Boolean)[0];
  if (!first || first.startsWith(":") || first === "**") return "home";
  return first.replace(/[^a-z0-9]+/gi, "-").toLowerCase();
}

function walk(dir, segs, layouts, acc, root) {
  let entries;
  try {
    entries = readdirSync(dir).sort();
  } catch {
    return;
  }
  const here = [...layouts];
  const layout = entries.find((e) => LAYOUT_RE.test(e));
  if (layout) here.push(relative(root, join(dir, layout)).split(sep).join("/"));

  for (const e of entries) {
    const full = join(dir, e);
    if (PAGE_RE.test(e)) {
      acc.views.push({
        route: "/" + segs.filter(Boolean).join("/"),
        source: relative(root, full).split(sep).join("/"),
        dependencies: here,
      });
    } else if (ROUTE_RE.test(e)) {
      acc.requests.push({
        route: "/" + segs.filter(Boolean).join("/"),
        source: relative(root, full).split(sep).join("/"),
        methods: detectMethods(full),
      });
    }
  }
  for (const e of entries) {
    const full = join(dir, e);
    let st;
    try {
      st = statSync(full);
    } catch {
      continue;
    }
    if (!st.isDirectory()) continue;
    const seg = segmentToRoute(e);
    if (seg === null) continue;
    walk(full, seg === "" ? segs : [...segs, seg], here, acc, root);
  }
}

function detectMethods(file) {
  let src = "";
  try {
    src = readFileSync(file, "utf8");
  } catch {
    return [];
  }
  const found = new Set();
  for (const m of HTTP_METHODS) {
    if (new RegExp(`export\\s+(async\\s+)?function\\s+${m}\\b`).test(src))
      found.add(m);
    if (new RegExp(`export\\s+(const|let)\\s+${m}\\b`).test(src)) found.add(m);
    if (new RegExp(`export\\s*\\{[^}]*\\b${m}\\b[^}]*\\}`).test(src))
      found.add(m);
  }
  return [...found];
}

/** Also honor the Pages Router, minus _app/_document/_error and api/ (the latter become requests). */
function walkPages(dir, segs, acc, root) {
  let entries;
  try {
    entries = readdirSync(dir).sort();
  } catch {
    return;
  }
  for (const e of entries) {
    const full = join(dir, e);
    const st = statSync(full);
    if (st.isDirectory()) {
      if (segs.length === 0 && e === "api") {
        walkPagesApi(full, ["api"], acc, root);
        continue;
      }
      walkPages(full, [...segs, segmentToRoute(e) ?? e], acc, root);
      continue;
    }
    const m = /^(.+)\.(js|jsx|ts|tsx|md|mdx)$/.exec(e);
    if (!m) continue;
    const name = m[1];
    if (
      segs.length === 0 &&
      ["_app", "_document", "_error", "404", "500"].includes(name)
    )
      continue;
    const seg = name === "index" ? "" : (segmentToRoute(name) ?? name);
    acc.views.push({
      route: "/" + [...segs, seg].filter(Boolean).join("/"),
      source: relative(root, full).split(sep).join("/"),
      dependencies: [],
    });
  }
}

function walkPagesApi(dir, segs, acc, root) {
  for (const e of readdirSync(dir).sort()) {
    const full = join(dir, e);
    if (statSync(full).isDirectory()) {
      walkPagesApi(full, [...segs, segmentToRoute(e) ?? e], acc, root);
      continue;
    }
    const m = /^(.+)\.(js|jsx|ts|tsx)$/.exec(e);
    if (!m) continue;
    const seg = m[1] === "index" ? "" : (segmentToRoute(m[1]) ?? m[1]);
    acc.requests.push({
      route: "/" + [...segs, seg].filter(Boolean).join("/"),
      source: relative(root, full).split(sep).join("/"),
      methods: [],
    });
  }
}

/**
 * Scan a Next.js project and return `{ views, requests }` — each view carries
 * `route`, `source`, `dependencies`; each request carries `route`, `source`,
 * `methods`. Pure: reads the tree, writes nothing.
 */
export function scanNextProject(root, { appDir } = {}) {
  const acc = { views: [], requests: [] };
  const candidates = appDir ? [appDir] : ["app", "src/app"];
  for (const c of candidates) {
    const dir = join(root, c);
    if (existsSync(dir)) {
      walk(dir, [], [], acc, root);
      break;
    }
  }
  for (const c of ["pages", "src/pages"]) {
    const dir = join(root, c);
    if (existsSync(dir)) {
      walkPages(dir, [], acc, root);
      break;
    }
  }
  // Deterministic order, unique names.
  acc.views.sort((a, b) => a.route.localeCompare(b.route));
  const seen = new Map();
  for (const v of acc.views) {
    let name = routeToName(v.route);
    if (seen.has(name)) {
      seen.set(name, seen.get(name) + 1);
      name = `${name}${seen.get(name)}`;
    } else seen.set(name, 1);
    v.name = name;
  }
  acc.requests.sort((a, b) => a.route.localeCompare(b.route));
  for (const r of acc.requests)
    r.name = routeToName(r.route.replace(/^\/api\b/, "") || "/") + "Api";
  return acc;
}

function y(s) {
  // Quote only when YAML would otherwise mis-parse (leading punctuation, colons, etc.).
  if (/^[A-Za-z0-9_./-]+$/.test(s) && !/^(true|false|null|yes|no|~)$/i.test(s))
    return s;
  return "'" + s.replace(/'/g, "''") + "'";
}

/** Render one feature file's YAML (views + any requests under the same top-level segment). */
export function renderFeatureYaml(feature, views, requests, { baseUrl } = {}) {
  const lines = ["version: 1", ""];
  if (views.length) {
    lines.push("views:");
    for (const v of views) {
      lines.push(`  - name: ${v.name}`);
      lines.push(`    route: ${y(v.route)}`);
      const concrete = !/[:*]/.test(v.route);
      if (baseUrl && concrete)
        lines.push(`    url: ${y(baseUrl.replace(/\/$/, "") + v.route)}`);
      lines.push(`    stability: stub`);
      lines.push(`    source: ${y(v.source)}`);
      if (v.dependencies?.length) {
        lines.push(`    dependencies:`);
        for (const d of v.dependencies) lines.push(`      - ${y(d)}`);
      }
      lines.push(
        `    description: ${y("Seeded from the Next.js router by sightmap-next. Curate against the running app, then drop stability: stub.")}`,
      );
      lines.push(`    components: []`);
    }
  }
  if (requests.length) {
    if (views.length) lines.push("");
    lines.push("requests:");
    for (const r of requests) {
      lines.push(`  - name: ${r.name}`);
      lines.push(`    route: ${y(r.route)}`);
      if (r.methods?.length === 1) lines.push(`    method: ${r.methods[0]}`);
      lines.push(`    source: ${y(r.source)}`);
      if (r.methods?.length > 1)
        lines.push(
          `    description: Route handler exporting ${r.methods.join(", ")}. Split into one request per method if the payloads differ.`,
        );
    }
  }
  lines.push("");
  return lines.join("\n");
}

/** Group scan results into feature files and write the ones that don't exist yet. */
export function seed(
  root,
  {
    appDir,
    sightmapDir = ".sightmap",
    baseUrl = "http://localhost:3000",
    dryRun = false,
    log = console.log,
  } = {},
) {
  const { views, requests } = scanNextProject(root, { appDir });
  if (!views.length && !requests.length) {
    log(`nothing to seed: no app/ or pages/ router found under ${root}`);
    return { written: [], skipped: [], views, requests };
  }
  const byFeature = new Map();
  for (const v of views) {
    const f = routeToFeature(v.route);
    if (!byFeature.has(f)) byFeature.set(f, { views: [], requests: [] });
    byFeature.get(f).views.push(v);
  }
  for (const r of requests) {
    const f = routeToFeature(r.route.replace(/^\/api\b/, "") || "/");
    if (!byFeature.has(f)) byFeature.set(f, { views: [], requests: [] });
    byFeature.get(f).requests.push(r);
  }
  const outDir = join(root, sightmapDir);
  const written = [];
  const skipped = [];
  if (!dryRun) mkdirSync(outDir, { recursive: true });
  const cfg = join(outDir, "config.yaml");
  if (!existsSync(cfg) && !dryRun) {
    writeFileSync(cfg, "version: 1\n");
    written.push(relative(root, cfg));
  }
  for (const [feature, group] of [...byFeature.entries()].sort()) {
    const file = join(outDir, `${feature}.yaml`);
    const rel = relative(root, file);
    const yaml = renderFeatureYaml(feature, group.views, group.requests, {
      baseUrl,
    });
    if (existsSync(file)) {
      skipped.push(rel);
      continue;
    }
    if (dryRun) {
      log(`--- ${rel}\n${yaml}`);
    } else {
      writeFileSync(file, yaml);
    }
    written.push(rel);
  }
  return { written, skipped, views, requests };
}

export async function seedCommand(argv) {
  const args = parseArgs(argv, { booleans: ["dry-run"] });
  const root = args._[0] ?? process.cwd();
  const res = seed(root, {
    appDir: args["app-dir"],
    sightmapDir: args["sightmap-dir"],
    baseUrl: args["base-url"],
    dryRun: !!args["dry-run"],
  });
  const n = res.views.length;
  console.log(
    `${args["dry-run"] ? "would seed" : "seeded"} ${n} view${n === 1 ? "" : "s"}, ${res.requests.length} request${res.requests.length === 1 ? "" : "s"} from the Next.js router`,
  );
  for (const f of res.written)
    console.log(`  ${args["dry-run"] ? "would write" : "wrote  "} ${f}`);
  for (const f of res.skipped)
    console.log(
      `  skipped ${f} (exists — the corpus is curated; edit it by hand or with the sightmap-authoring skill)`,
    );
  if (res.written.length && !args["dry-run"]) {
    console.log(
      `\nNext:\n  1. sightmap validate\n  2. next dev, then curate each stub against the live app (sightmap-authoring skill)\n  3. write .sightkick/tools.yaml (sightkick-authoring skill), then: sightmap-next build`,
    );
  }
  return 0;
}
