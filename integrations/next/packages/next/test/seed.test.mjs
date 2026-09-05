import { test } from "node:test";
import assert from "node:assert/strict";
import {
  mkdtempSync,
  mkdirSync,
  existsSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  scanNextProject,
  segmentToRoute,
  routeToName,
  routeToFeature,
  renderFeatureYaml,
  seed,
} from "../src/seed.mjs";

const FIXTURE = new URL("./fixtures/app-router", import.meta.url).pathname;

test("segments map to spec route syntax", () => {
  assert.equal(segmentToRoute("[id]"), ":id");
  assert.equal(segmentToRoute("[...slug]"), "**");
  assert.equal(segmentToRoute("[[...slug]]"), "**");
  assert.equal(segmentToRoute("(marketing)"), "");
  assert.equal(segmentToRoute("@modal"), null);
  assert.equal(segmentToRoute("(.)photo"), null);
  assert.equal(segmentToRoute("_lib"), null);
  assert.equal(segmentToRoute("tasks"), "tasks");
});

test("names and feature files are deterministic", () => {
  assert.equal(routeToName("/"), "Home");
  assert.equal(routeToName("/tasks/:id"), "TasksById");
  assert.equal(routeToName("/docs/**"), "DocsCatchAll");
  assert.equal(routeToFeature("/"), "home");
  assert.equal(routeToFeature("/tasks/:id"), "tasks");
  assert.equal(routeToFeature("/about"), "about");
});

test("scan walks the App Router and skips slots, private, and intercepting routes", () => {
  const { views, requests } = scanNextProject(FIXTURE);
  assert.deepEqual(
    views.map((v) => [v.name, v.route, v.source]),
    [
      ["Home", "/", "app/page.tsx"],
      ["About", "/about", "app/(marketing)/about/page.tsx"],
      ["DocsCatchAll", "/docs/**", "app/docs/[...slug]/page.mdx"],
      ["TasksById", "/tasks/:id", "app/tasks/[id]/page.tsx"],
    ],
  );
  const detail = views.find((v) => v.name === "TasksById");
  assert.deepEqual(detail.dependencies, [
    "app/layout.tsx",
    "app/tasks/layout.tsx",
  ]);
  assert.deepEqual(
    requests.map((r) => [r.route, r.methods.sort()]),
    [
      ["/api/tasks", ["GET", "POST"]],
      ["/api/tasks/:id", ["DELETE"]],
    ],
  );
});

test("rendered YAML is valid corpus shape", () => {
  const { views, requests } = scanNextProject(FIXTURE);
  const yaml = renderFeatureYaml(
    "tasks",
    views.filter((v) => v.route.startsWith("/tasks")),
    requests,
    { baseUrl: "http://localhost:3000" },
  );
  assert.match(yaml, /^version: 1\n/);
  assert.match(yaml, /route: .\/tasks\/:id.\n/);
  assert.match(yaml, /stability: stub/);
  assert.doesNotMatch(
    yaml,
    /url: http:\/\/localhost:3000\/tasks\/:id/,
    "parameterized routes get no url",
  );
  assert.match(
    yaml,
    /requests:\n  - name: TasksApi\n    route: \/api\/tasks\n    source: app\/api\/tasks\/route.ts\n    description: Route handler exporting GET, POST/,
  );
});

test("seed writes once and never overwrites", () => {
  const root = mkdtempSync(join(tmpdir(), "sightmap-next-"));
  writeFileSync(join(root, "package.json"), "{}");
  const app = join(root, "app");
  mkdirSync(join(app, "about"), { recursive: true });
  writeFileSync(join(app, "page.tsx"), "");
  writeFileSync(join(app, "about", "page.tsx"), "");
  const first = seed(root, { log: () => {} });
  assert.deepEqual(first.written.sort(), [
    ".sightmap/about.yaml",
    ".sightmap/config.yaml",
    ".sightmap/home.yaml",
  ]);
  assert.ok(existsSync(join(root, ".sightmap", "home.yaml")));
  writeFileSync(
    join(root, ".sightmap", "home.yaml"),
    "version: 1\n# curated\n",
  );
  const second = seed(root, { log: () => {} });
  assert.deepEqual(second.written, []);
  assert.deepEqual(second.skipped.sort(), [
    ".sightmap/about.yaml",
    ".sightmap/home.yaml",
  ]);
  assert.equal(
    readFileSync(join(root, ".sightmap", "home.yaml"), "utf8"),
    "version: 1\n# curated\n",
  );
});
