const fs = require("fs");
const path = require("path");

(0, eval)(
  fs.readFileSync(
    path.join(__dirname, "..", "..", "browser", "deepquery.js"),
    "utf8",
  ),
);
(0, eval)(fs.readFileSync(path.join(__dirname, "cmd_sel_probe.js"), "utf8"));

describe("__smQueryElements", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("describes each match: tag, id, classes, role, text, attrs", () => {
    document.body.innerHTML = `
      <button id="save" class="btn primary" role="button" data-testid="save-btn">
        Save now
      </button>
    `;
    const [el] = __smQueryElements("button", 10, false);
    expect(el).toMatchObject({
      tag: "button",
      id: "save",
      cls: ["btn", "primary"],
      role: "button",
      text: "Save now",
      attrs: { "data-testid": "save-btn" },
    });
  });

  test("truncates text to 80 chars unless full is true", () => {
    document.body.innerHTML = `<div id="d">${"x".repeat(200)}</div>`;
    expect(__smQueryElements("#d", 10, false)[0].text).toHaveLength(80);
    expect(__smQueryElements("#d", 10, true)[0].text).toHaveLength(200);
  });

  test("caps results at max", () => {
    document.body.innerHTML =
      '<i class="x"></i><i class="x"></i><i class="x"></i>';
    expect(__smQueryElements(".x", 2, false)).toHaveLength(2);
  });

  test("collects ancestors up to 5 levels, stopping at html", () => {
    document.body.innerHTML =
      '<div data-testid="root"><div><div><span id="leaf"></span></div></div></div>';
    const [el] = __smQueryElements("#leaf", 10, false);
    expect(el.parents.length).toBeGreaterThan(0);
    expect(el.parents.some((p) => p.dt === "root")).toBe(true);
    expect(el.parents.every((p) => p.tag !== "html")).toBe(true);
  });

  test("finds matches inside shadow DOM", () => {
    const host = document.createElement("div");
    document.body.appendChild(host);
    host.attachShadow({ mode: "open" }).innerHTML =
      '<button id="shadow-btn"></button>';
    expect(__smQueryElements("#shadow-btn", 10, false)).toHaveLength(1);
  });

  // deepquery.js's __smDeepQueryAll already fails soft on an invalid
  // selector (returns [], doesn't throw), so this never reaches the
  // {error} branch — it just sees zero matches, same as any other miss.
  test("a bad selector yields no matches instead of throwing", () => {
    expect(() => __smQueryElements("[", 10, false)).not.toThrow();
    expect(__smQueryElements("[", 10, false)).toEqual([]);
  });
});

describe("__smLiveSelectorCount", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("returns the true, uncapped match count", () => {
    document.body.innerHTML =
      '<i class="x"></i><i class="x"></i><i class="x"></i>';
    expect(__smLiveSelectorCount(".x")).toBe(3);
  });

  // Same fail-soft reasoning as __smQueryElements above: __smDeepQueryAll
  // never throws on a bad selector, so this sees a length-0 result, not -1.
  test("a bad selector yields 0, not -1 (deepquery already fails soft)", () => {
    expect(__smLiveSelectorCount("[")).toBe(0);
  });
});

describe("__smSelProbeAllCount", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("caps the count at max", () => {
    document.body.innerHTML =
      '<i class="x"></i><i class="x"></i><i class="x"></i>';
    expect(__smSelProbeAllCount(".x", 2)).toBe(2);
  });

  test("a bad selector yields 0, not -1 (deepquery already fails soft)", () => {
    expect(__smSelProbeAllCount("[", 10)).toBe(0);
  });
});
