const fs = require("fs");
const path = require("path");

(0, eval)(fs.readFileSync(path.join(__dirname, "probe.js"), "utf8"));

describe("computeCompProps node cap", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  const idNumbers = (out) =>
    out.results.map((r) => parseInt(r.id.slice("2_".length), 10));

  // Every <p> holds a text node, which takes an id of its own. The cap counts
  // those ids too, so a cut document does not run past maxNodes by the text
  // children of the element it was cut at.
  test("a cut document stamps no id past the cap", () => {
    document.body.innerHTML = Array.from(
      { length: 50 },
      (_, i) => `<p>row ${i}</p>`,
    ).join("");
    const out = computeCompProps(true, false, "2_", 10);
    expect(out.success).toBe(true);
    expect(out.truncated).toBe(true);
    expect(Math.max(...idNumbers(out))).toBeLessThanOrEqual(10);
  });

  test("no cap leaves the document whole", () => {
    document.body.innerHTML = "<p>a</p><p>b</p>";
    const out = computeCompProps(true, false, "", 0);
    expect(out.truncated).toBe(false);
  });
});
