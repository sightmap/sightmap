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
    const components = [
      { name: "Row", selector: ".row" },
      {
        name: "Badge",
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
