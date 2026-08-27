// Corpus loader — reads a .sightmap/ directory the way the spec describes
// (every *.yaml / *.yml discovered recursively, shallow-append merge, $ref
// expansion against the root-level component registry) and builds the indexes
// the compiler resolves manifest references against:
//
//   - components: breadcrumb path → { chainLevels, props } where chainLevels
//     is the ancestor-prepended selector chain (each level a list of
//     alternative selectors, first match wins — spec "Selector semantics")
//   - requests:   name → { route, method, properties, ... }
//   - views:      name → { route, url, ... }
//
// This is a read-only consumer: it never validates the corpus beyond what it
// needs (that is `sightmap validate`'s job), and unknown fields are ignored
// per the spec's "MAY ignore fields it doesn't use".

const fs = require("fs");
const path = require("path");
const yaml = require("js-yaml");

function listYamlFiles(dir) {
  const out = [];
  const walk = (d, top) => {
    for (const entry of fs.readdirSync(d, { withFileTypes: true })) {
      if (entry.name.startsWith(".")) continue;
      // Skip the reference CLI's non-corpus subdirectories, as its own
      // loader does: review/ holds punch-list YAML, snapshots/ holds
      // snapshot blobs.
      if (
        entry.isDirectory() &&
        (entry.name === "review" || entry.name === "snapshots")
      ) {
        continue;
      }
      const p = path.join(d, entry.name);
      if (entry.isDirectory()) walk(p, false);
      else if (/\.ya?ml$/.test(entry.name)) out.push(p);
    }
  };
  walk(dir, true);
  return out.sort();
}

// Expand `$ref` entries against the registry of root-level components.
// First-seen wins on duplicate names (files already sorted by path).
function buildRegistry(files) {
  const registry = new Map();
  for (const { doc } of files) {
    for (const comp of doc.components || []) {
      if (comp && comp.name && !comp.$ref && !registry.has(comp.name)) {
        registry.set(comp.name, comp);
      }
    }
  }
  return registry;
}

function expandRefs(list, registry, seen) {
  if (!Array.isArray(list)) return [];
  const out = [];
  for (const entry of list) {
    if (entry && entry.$ref) {
      const target = registry.get(entry.$ref);
      if (!target) {
        throw new Error(
          `ref-unresolved: no root-level component named "${entry.$ref}"`,
        );
      }
      if (seen.has(entry.$ref)) {
        throw new Error(`ref-circular: "${entry.$ref}" expands through itself`);
      }
      const nextSeen = new Set(seen).add(entry.$ref);
      out.push(expandComponent(target, registry, nextSeen));
    } else if (entry && entry.name) {
      out.push(expandComponent(entry, registry, seen));
    }
  }
  return out;
}

function expandComponent(comp, registry, seen) {
  const copy = { ...comp };
  copy.children = expandRefs(comp.children, registry, seen);
  return copy;
}

function selectorAlternatives(selector) {
  if (Array.isArray(selector)) return selector.map(String);
  return [String(selector)];
}

// Index one component subtree. `prefixPath` is the breadcrumb of the parent
// ("" at root); `prefixLevels` its selector chain; `scope` is null for a
// global component or the owning view's name for a view-scoped one (the same
// name is often defined per-view with different selectors — IKEA's
// ProductCard renders differently on search and category pages).
function indexComponent(comp, prefixPath, prefixLevels, index, scope) {
  const crumb = prefixPath ? `${prefixPath} ${comp.name}` : comp.name;
  const levels = [...prefixLevels, selectorAlternatives(comp.selector)];
  const props = {};
  for (const p of comp.properties || []) {
    if (p && p.name && p.extract) {
      props[p.name] = { extract: String(p.extract) };
      if (p.transform) props[p.name].transform = String(p.transform);
    }
  }
  const entry = {
    path: crumb,
    name: comp.name,
    chainLevels: levels,
    props,
    scope: scope || null,
    description: comp.description || "",
    memory: comp.memory || [],
  };
  if (!index.has(crumb)) index.set(crumb, []);
  index.get(crumb).push(entry);
  for (const child of comp.children || []) {
    indexComponent(child, crumb, levels, index, scope);
  }
}

function sameChain(a, b) {
  return JSON.stringify(a.chainLevels) === JSON.stringify(b.chainLevels);
}

function loadCorpus(dir) {
  if (!fs.existsSync(dir) || !fs.statSync(dir).isDirectory()) {
    throw new Error(`sightmap corpus not found: ${dir} is not a directory`);
  }
  const filePaths = listYamlFiles(dir);
  const files = [];
  for (const p of filePaths) {
    const doc = yaml.load(fs.readFileSync(p, "utf8"));
    if (!doc || typeof doc !== "object") continue;
    // A YAML file without "version: 1" is a tooling file (survey.yaml and
    // friends), not a corpus file — skip it, as the reference loader does.
    if (doc.version !== 1) continue;
    files.push({ path: p, doc });
  }
  if (files.length === 0) {
    throw new Error(
      `sightmap corpus at ${dir} contains no corpus YAML files (version: 1)`,
    );
  }

  const registry = buildRegistry(files);

  // components: breadcrumb path → [entry, ...]. Duplicate identical entries
  // (e.g. a $ref expansion of a global inside a view) are collapsed; genuinely
  // different definitions under the same breadcrumb stay as a list, and the
  // compiler reports the ambiguity if a manifest references them.
  const components = new Map();
  const views = new Map();
  const requests = new Map();
  const duplicateRequests = new Set();

  for (const { path: p, doc } of files) {
    for (const comp of expandRefs(doc.components, registry, new Set())) {
      indexComponent(comp, "", [], components, null);
    }
    for (const req of doc.requests || []) {
      if (req && req.name) {
        if (requests.has(req.name)) duplicateRequests.add(req.name);
        else requests.set(req.name, req);
      }
    }
    for (const view of doc.views || []) {
      if (!view || !view.name) continue;
      if (!views.has(view.name)) {
        views.set(view.name, {
          name: view.name,
          route: view.route,
          url: view.url || doc.url || null,
          description: view.description || "",
          memory: view.memory || [],
        });
      }
      for (const comp of expandRefs(view.components, registry, new Set())) {
        indexComponent(comp, "", [], components, view.name);
      }
      for (const req of view.requests || []) {
        if (req && req.name) {
          if (requests.has(req.name)) duplicateRequests.add(req.name);
          else requests.set(req.name, req);
        }
      }
    }
  }

  // Collapse duplicate identical component entries (e.g. a $ref expansion of
  // a global inside a view). Identical chains resolve identically regardless
  // of scope, so a merged entry keeps the widest scope.
  for (const [crumb, entries] of components) {
    const unique = [];
    for (const e of entries) {
      const dup = unique.find((u) => sameChain(u, e));
      if (dup) {
        if (dup.scope !== e.scope) dup.scope = null;
      } else {
        unique.push(e);
      }
    }
    components.set(crumb, unique);
  }

  // name → [breadcrumb, ...] index for bare-name resolution.
  const byName = new Map();
  for (const crumb of components.keys()) {
    const name = crumb.split(" ").pop();
    if (!byName.has(name)) byName.set(name, []);
    byName.get(name).push(crumb);
  }

  return {
    dir,
    files: files.map((f) => f.path),
    components,
    byName,
    views,
    requests,
    duplicateRequests,
  };
}

module.exports = { loadCorpus };
