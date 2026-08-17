const fs = require("fs");
const path = require("path");

// deepquery.js declares __smDeepQuery/__smDeepQueryAll as globals so it can be
// prepended to a CDP Runtime.evaluate expression. Load it the same way here,
// via indirect eval, so the test exercises the exact file that ships.
(0, eval)(fs.readFileSync(path.join(__dirname, "deepquery.js"), "utf8"));

describe("__smDeepQueryAll", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("matches light-DOM descendants like querySelectorAll", () => {
    document.body.innerHTML =
      '<div><span class="x"></span><span class="x"></span></div>';
    expect(__smDeepQueryAll(document, ".x")).toHaveLength(2);
  });

  test("matches elements inside a shadow root", () => {
    const host = document.createElement("div");
    document.body.appendChild(host);
    const root = host.attachShadow({ mode: "open" });
    root.innerHTML = '<span class="x">shadow</span>';

    const matches = __smDeepQueryAll(document, ".x");
    expect(matches).toHaveLength(1);
    expect(matches[0].textContent).toBe("shadow");
  });

  test("pierces nested shadow roots at any depth", () => {
    const outer = document.createElement("div");
    document.body.appendChild(outer);
    const inner = document.createElement("div");
    outer.attachShadow({ mode: "open" }).appendChild(inner);
    inner.attachShadow({ mode: "open" }).innerHTML =
      '<span class="x">deep</span>';

    expect(__smDeepQueryAll(document, ".x")).toHaveLength(1);
  });

  // probe.js's procNode flattens a node's light children fully (including
  // each child's own shadow content) before moving to the node's next light
  // sibling. __smDeepQueryAll must return matches in that same order so
  // "first match" agrees with the corpus — see deepquery.go and
  // spec/v1/schema.md's "Selector model & shadow DOM".
  test("match order mirrors probe.js flattening: a light child fully (incl. its own shadow) before the next light sibling", () => {
    // <div id=a><div id=b>#shadow[<span id=x class=sel>]</div><span id=c class=sel></div>
    // flattens to a, b, x, c — so a query for .sel starting at `a` must
    // return [x, c], not [c, x] (document-order-of-light-matches-first).
    const a = document.createElement("div");
    const b = document.createElement("div");
    a.appendChild(b);
    const x = document.createElement("span");
    x.id = "x";
    x.className = "sel";
    b.attachShadow({ mode: "open" }).appendChild(x);
    const c = document.createElement("span");
    c.id = "c";
    c.className = "sel";
    a.appendChild(c);

    expect(__smDeepQueryAll(a, ".sel").map((el) => el.id)).toEqual(["x", "c"]);
  });

  test("an invalid selector fails soft: returns no matches instead of throwing", () => {
    document.body.innerHTML = "<div></div>";
    expect(() => __smDeepQueryAll(document, "[")).not.toThrow();
    expect(__smDeepQueryAll(document, "[")).toEqual([]);
  });

  test("root itself carrying a shadow root is searched too", () => {
    const host = document.createElement("div");
    host.attachShadow({ mode: "open" }).innerHTML =
      '<span class="x">shadow</span>';
    expect(__smDeepQueryAll(host, ".x")).toHaveLength(1);
  });

  test("root does not match itself, only descendants (mirrors querySelectorAll)", () => {
    const el = document.createElement("div");
    el.className = "x";
    document.body.appendChild(el);
    expect(__smDeepQueryAll(el, ".x")).toHaveLength(0);
  });
});

describe("__smDeepQuery", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("returns the first match in flattened order, not light-DOM-first order", () => {
    const a = document.createElement("div");
    document.body.appendChild(a);
    const b = document.createElement("div");
    a.appendChild(b);
    const shadowMatch = document.createElement("span");
    shadowMatch.className = "sel";
    shadowMatch.textContent = "shadow-match";
    b.attachShadow({ mode: "open" }).appendChild(shadowMatch);
    const lightMatch = document.createElement("span");
    lightMatch.className = "sel";
    lightMatch.textContent = "light-match";
    a.appendChild(lightMatch);

    expect(__smDeepQuery(a, ".sel").textContent).toBe("shadow-match");
  });

  test("returns null when nothing matches", () => {
    document.body.innerHTML = "<div></div>";
    expect(__smDeepQuery(document, ".nope")).toBeNull();
  });
});
