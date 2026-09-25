const fs = require("fs");
const path = require("path");

// resolver.js is an ES module (import/export) so it ships to the browser as
// one, but Jest here has no ESM transform configured. Its only module-level
// statements are the two-line buildFormatted re-export at the top and the
// `export` keyword on three function declarations — neither affects the
// property-extraction logic under test, so strip them and eval the exact
// file that ships, same as the other embedded-JS tests in this repo.
const source = fs
  .readFileSync(path.join(__dirname, "resolver.js"), "utf8")
  .replace('import { buildFormatted } from "./types.js";\n', "")
  .replace("export { buildFormatted };\n", "")
  .replace(/^export function/gm, "function");
(0, eval)(source);

describe("extractProperties", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("text reads the element's DOM text content", () => {
    document.body.innerHTML = '<div id="card">Hello <b>World</b></div>';
    const card = document.getElementById("card");
    const result = extractProperties(card, [{ name: "label", extract: "text" }], []);
    expect(result).toEqual({ label: "Hello World" });
  });

  test("attr=NAME reads the named attribute", () => {
    document.body.innerHTML = '<a id="link" href="/x">Link</a>';
    const link = document.getElementById("link");
    const result = extractProperties(link, [{ name: "href", extract: "attr=href" }], []);
    expect(result).toEqual({ href: "/x" });
  });

  test("attr=NAME on a missing attribute omits the property", () => {
    document.body.innerHTML = '<a id="link">Link</a>';
    const link = document.getElementById("link");
    const result = extractProperties(link, [{ name: "href", extract: "attr=href" }], []);
    expect(result).toEqual({});
  });

  test("exists:PATH resolves true when the descendant component matched", () => {
    document.body.innerHTML =
      '<div id="card"><span class="badge">New</span></div>';
    const card = document.getElementById("card");
    const components = [{ name: "Badge", selector: ".badge" }];
    const result = extractProperties(
      card,
      [{ name: "hasBadge", extract: "exists:Badge" }],
      components,
    );
    expect(result).toEqual({ hasBadge: "true" });
  });

  test("exists:PATH omits the property when the descendant component didn't match", () => {
    document.body.innerHTML = '<div id="card"></div>';
    const card = document.getElementById("card");
    const components = [{ name: "Badge", selector: ".badge" }];
    const result = extractProperties(
      card,
      [{ name: "hasBadge", extract: "exists:Badge" }],
      components,
    );
    expect(result).toEqual({});
  });

  test("PATH.prop reads a descendant component's own extracted property", () => {
    document.body.innerHTML =
      '<div id="card"><span class="price">$12.00</span></div>';
    const card = document.getElementById("card");
    const components = [
      {
        name: "Price",
        selector: ".price",
        properties: [{ name: "amount", extract: "text" }],
      },
    ];
    const result = extractProperties(
      card,
      [{ name: "amount", extract: "Price.amount" }],
      components,
    );
    expect(result).toEqual({ amount: "$12.00" });
  });

  test("PATH.prop resolves through two levels of descendant components", () => {
    document.body.innerHTML =
      '<div id="card"><div class="row"><span class="badge">Sale</span></div></div>';
    const card = document.getElementById("card");
    // Badge is declared as Row's child, which is what "Row.Badge.label" asserts.
    const components = [
      { name: "Row", selector: ".row" },
      {
        name: "Badge",
        parentChain: ["Row"],
        selector: ".badge",
        properties: [{ name: "label", extract: "text" }],
      },
    ];
    const result = extractProperties(
      card,
      [{ name: "label", extract: "Row.Badge.label" }],
      components,
    );
    expect(result).toEqual({ label: "Sale" });
  });

  test("PATH.prop omits the property when the descendant path doesn't resolve", () => {
    document.body.innerHTML = '<div id="card"></div>';
    const card = document.getElementById("card");
    const components = [
      {
        name: "Price",
        selector: ".price",
        properties: [{ name: "amount", extract: "text" }],
      },
    ];
    const result = extractProperties(
      card,
      [{ name: "amount", extract: "Price.amount" }],
      components,
    );
    expect(result).toEqual({});
  });

  test("collapses whitespace and caps at 120 characters", () => {
    document.body.innerHTML = `<div id="card">${"word ".repeat(40)}</div>`;
    const card = document.getElementById("card");
    const result = extractProperties(card, [{ name: "label", extract: "text" }], []);
    expect(result.label).toHaveLength(120);
    expect(result.label).not.toMatch(/\s{2,}/);
  });

  test("an invalid selector on a component def is skipped, not thrown", () => {
    document.body.innerHTML = '<div id="card"></div>';
    const card = document.getElementById("card");
    const components = [{ name: "Bad", selector: "[[[" }];
    const result = extractProperties(
      card,
      [{ name: "x", extract: "exists:Bad" }],
      components,
    );
    expect(result).toEqual({});
  });

  test("an unrecognized extract form omits the property", () => {
    document.body.innerHTML = '<div id="card">text</div>';
    const card = document.getElementById("card");
    const result = extractProperties(card, [{ name: "x", extract: "bogus" }], []);
    expect(result).toEqual({});
  });
});

// Component names are unique only WITHIN A PARENT (spec, "Selector semantics":
// children are matched inside their parent's subtree, "how Sightmap avoids
// naming collisions between, say, two different card components that both
// contain a button.primary").
//
// The extension originally keyed three separate lookups on the bare name. Each
// test below fails against one of those. Fixtures mirror a production sign-in
// corpus where `Label` occurs 13 times and `Text` 18 times.

describe("componentAddress", () => {
  test("a root component's address is its name", () => {
    expect(componentAddress({ name: "Widget", parentChain: [] })).toBe("Widget");
    expect(componentAddress({ name: "Widget" })).toBe("Widget");
  });

  test("the same name under different parents gives different addresses", () => {
    const a = { name: "Label", parentChain: ["Form", "Field"] };
    const b = { name: "Label", parentChain: ["Form", "SubmitButton"] };
    expect(componentAddress(a)).not.toBe(componentAddress(b));
    expect(parentAddress(a)).not.toBe(parentAddress(b));
  });

  test("a name cannot forge another component's address", () => {
    // `name` has no charset restriction in the schema, so the separator has to
    // be something a name cannot contain.
    const forged = { name: "Field > Label", parentChain: ["Form"] };
    const real = { name: "Label", parentChain: ["Form", "Field"] };
    expect(componentAddress(forged)).not.toBe(componentAddress(real));
  });
});

describe("dedupeByAddress", () => {
  test("keeps every namesake that sits under a different parent", () => {
    // The live bug: keyed by name, 104 components collapsed to 54 and a click
    // resolved two levels deep instead of six.
    const components = [
      { name: "Form", parentChain: [] },
      { name: "Label", parentChain: ["Form", "Field"] },
      { name: "Label", parentChain: ["Form", "Submit"] },
      { name: "Label", parentChain: [] },
    ];
    expect(dedupeByAddress(components)).toHaveLength(4);
  });

  test("a later list overrides an earlier one at the SAME address", () => {
    const globals = [{ name: "Nav", parentChain: [], selector: ".global" }];
    const view = [{ name: "Nav", parentChain: [], selector: ".view" }];
    const merged = dedupeByAddress(globals, view);
    expect(merged).toHaveLength(1);
    expect(merged[0].selector).toBe(".view");
  });

  test("a view component does NOT override a global that merely shares a name", () => {
    const globals = [{ name: "Label", parentChain: [], selector: ".global" }];
    const view = [
      { name: "Label", parentChain: ["Card"], selector: ".in-card" },
    ];
    expect(dedupeByAddress(globals, view)).toHaveLength(2);
  });
});

describe("resolveElement", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("resolves the full chain when components share a leaf name", () => {
    document.body.innerHTML = `
      <form id="form">
        <div class="field"><label class="label"><input id="identifier"></label></div>
        <button class="submit"><span class="btn-label">Sign in</span></button>
      </form>`;
    const components = [
      { name: "Form", parentChain: [], selector: "#form" },
      { name: "Field", parentChain: ["Form"], selector: ".field" },
      { name: "Label", parentChain: ["Form", "Field"], selector: ".label" },
      {
        name: "Input",
        parentChain: ["Form", "Field", "Label"],
        selector: "#identifier",
      },
      { name: "Submit", parentChain: ["Form"], selector: ".submit" },
      { name: "Label", parentChain: ["Form", "Submit"], selector: ".btn-label" },
    ];
    const path = resolveElement(
      document.getElementById("identifier"),
      components,
    );
    expect(path.map((m) => m.name)).toEqual(["Form", "Field", "Label", "Input"]);
    // Addresses keep the two Labels apart for downstream consumers.
    expect(path[2].address).toBe(componentAddress(components[2]));
  });

  test("a namesake's match does not vouch for another component's parent", () => {
    // Child is declared under A > Ghost, which is absent from the page. B > Ghost
    // IS present and shares the leaf name "Ghost". Scoped by name, Child accepts
    // B > Ghost's match as its parent and is wrongly included; scoped by address,
    // its parent never matched and it is correctly dropped.
    document.body.innerHTML =
      '<div class="b"><div class="ghost-b"><span class="target">x</span></div></div>';
    const components = [
      { name: "A", parentChain: [], selector: ".a" },
      { name: "B", parentChain: [], selector: ".b" },
      { name: "Ghost", parentChain: ["A"], selector: ".ghost-a" },
      { name: "Ghost", parentChain: ["B"], selector: ".ghost-b" },
      { name: "Child", parentChain: ["A", "Ghost"], selector: ".target" },
    ];
    const path = resolveElement(document.querySelector(".target"), components);
    expect(path.map((m) => m.name)).toEqual(["B", "Ghost"]);
  });

  test("order-independent: a shuffled component list resolves identically", () => {
    document.body.innerHTML =
      '<div class="a"><div class="b"><span class="c">x</span></div></div>';
    const components = [
      { name: "A", parentChain: [], selector: ".a" },
      { name: "B", parentChain: ["A"], selector: ".b" },
      { name: "C", parentChain: ["A", "B"], selector: ".c" },
    ];
    const el = document.querySelector(".c");
    expect(resolveElement(el, [...components].reverse()).map((m) => m.address))
      .toEqual(resolveElement(el, components).map((m) => m.address));
  });

  test("full ancestor-chain selectors (the served wire shape) resolve too", () => {
    document.body.innerHTML =
      '<div class="card"><div class="body"><a class="link">x</a></div></div>';
    const wire = [
      { name: "Card", parentChain: [], selector: ".card" },
      { name: "Body", parentChain: ["Card"], selector: ".card .body" },
      {
        name: "Link",
        parentChain: ["Card", "Body"],
        selector: ".card .body .link",
      },
    ];
    const path = resolveElement(document.querySelector(".link"), wire);
    expect(path.map((m) => m.name)).toEqual(["Card", "Body", "Link"]);
  });

  test("an invalid selector is skipped, not thrown", () => {
    document.body.innerHTML = '<div id="form"><span class="x">y</span></div>';
    const path = resolveElement(document.querySelector(".x"), [
      { name: "Bad", parentChain: [], selector: "[[[" },
      { name: "Form", parentChain: [], selector: "#form" },
    ]);
    expect(path.map((m) => m.name)).toEqual(["Form"]);
  });
});

describe("resolveTier", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("T1 is decided by the innermost match's address, not its name", () => {
    document.body.innerHTML =
      '<div class="card"><span class="label">x</span></div>';
    // A namesake declared FIRST whose selector does not match the element: by
    // name it is found first and T1 is missed.
    const components = [
      { name: "Label", parentChain: ["Elsewhere"], selector: ".nope" },
      { name: "Card", parentChain: [], selector: ".card" },
      { name: "Label", parentChain: ["Card"], selector: ".label" },
    ];
    const el = document.querySelector(".label");
    expect(resolveTier(el, resolveElement(el, components), components)).toBe(1);
  });

  test("T3 when nothing matched", () => {
    document.body.innerHTML = "<div></div>";
    expect(resolveTier(document.querySelector("div"), [], [])).toBe(3);
  });
});

describe("extractProperties owner scoping", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  const twoLabels = () => {
    document.body.innerHTML = `
      <div class="field"><span class="label">Phone number</span></div>
      <button class="submit"><span class="btn-label">Sign in</span></button>`;
    return [
      { name: "Field", parentChain: [], selector: ".field" },
      {
        name: "Label",
        parentChain: ["Field"],
        selector: ".label",
        properties: [{ name: "label", extract: "text" }],
      },
      {
        name: "Submit",
        parentChain: [],
        selector: ".submit",
        properties: [{ name: "label", extract: "Label.label" }],
      },
      {
        name: "Label",
        parentChain: ["Submit"],
        selector: ".btn-label",
        properties: [{ name: "label", extract: "text" }],
      },
    ];
  };

  test("PATH.prop resolves the OWNER's child, not the first namesake", () => {
    // Observed live: Submit's `Label.label` resolved `Label` to the text field's
    // label, found nothing inside the button, and dropped the property.
    const components = twoLabels();
    const result = extractProperties(
      document.querySelector(".submit"),
      [{ name: "label", extract: "Label.label" }],
      components,
      "Submit",
    );
    expect(result).toEqual({ label: "Sign in" });
  });

  test("resolveElement threads the owner address into extraction", () => {
    const components = twoLabels();
    const path = resolveElement(document.querySelector(".btn-label"), components);
    expect(path.find((m) => m.name === "Submit").properties).toEqual({
      label: "Sign in",
    });
  });

  test("a descendant reference to a non-child does not resolve", () => {
    document.body.innerHTML =
      '<div class="card"><span class="badge">New</span></div>';
    // Badge is declared under Other, so it is not Card's child — even though
    // .badge does sit inside .card.
    const components = [
      { name: "Card", parentChain: [], selector: ".card" },
      { name: "Badge", parentChain: ["Other"], selector: ".badge" },
    ];
    const result = extractProperties(
      document.querySelector(".card"),
      [{ name: "hasBadge", extract: "exists:Badge" }],
      components,
      "Card",
    );
    expect(result).toEqual({});
  });
});

describe("content.js mirrors resolver.js", () => {
  const START = "// ─── SHARED-WITH-CONTENT:START ───";
  const END = "// ─── SHARED-WITH-CONTENT:END ───";

  const block = (file) => {
    const src = fs.readFileSync(path.join(__dirname, file), "utf8");
    const i = src.indexOf(START);
    const j = src.indexOf(END);
    expect(i).toBeGreaterThanOrEqual(0);
    expect(j).toBeGreaterThan(i);
    return src.slice(src.indexOf("\n", i) + 1, j);
  };

  // content.js is a classic MV3 content script and cannot import an ES module,
  // so the matching core is copied into it. Copying by hand is how the two
  // drifted the first time: one name-keyed bug, fixed twice, months apart.
  test("the shared block is identical apart from `export`", () => {
    const want = block("resolver.js").replace(/^export function/gm, "function");
    const have = block("content.js").replace(
      /^\/\/ \(generated by scripts\/sync-extension-resolver\.mjs.*\n/m,
      "",
    );
    expect(have).toBe(want);
  });
});
