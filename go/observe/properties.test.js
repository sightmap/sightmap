const fs = require("fs");
const path = require("path");

(0, eval)(
  fs.readFileSync(
    path.join(__dirname, "..", "browser", "deepquery.js"),
    "utf8",
  ),
);
(0, eval)(fs.readFileSync(path.join(__dirname, "properties.js"), "utf8"));

describe("__smExtractProperties", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("extracts text by data-sightmap-id, anchoring to the exact matched element", () => {
    document.body.innerHTML = `
      <div class="card" data-sightmap-id="1">
        <span class="label">outer</span>
        <div class="card" data-sightmap-id="2"><span class="label">inner</span></div>
      </div>
    `;
    const specs = [
      {
        id: "2",
        selector: ".card",
        props: [{ name: "label", extract: "text", transform: "" }],
      },
    ];
    expect(__smExtractProperties(specs)).toEqual({ 2: { label: "inner" } });
  });

  test("falls back to the selector when id is empty", () => {
    document.body.innerHTML =
      '<div class="card"><span class="label">hi</span></div>';
    const specs = [
      {
        id: "",
        selector: ".card",
        props: [{ name: "label", extract: ".label" }],
      },
    ];
    expect(__smExtractProperties(specs)).toEqual({ "": { label: "hi" } });
  });

  test("exists: reports true only when the sub-selector matches, including inside shadow DOM", () => {
    document.body.innerHTML = '<div class="card" data-sightmap-id="1"></div>';
    const host = document.querySelector(".card");
    host.attachShadow({ mode: "open" }).innerHTML =
      '<span class="icon"></span>';

    const specs = [
      {
        id: "1",
        selector: ".card",
        props: [
          { name: "hasIcon", extract: "exists:.icon" },
          { name: "hasBadge", extract: "exists:.badge" },
        ],
      },
    ];
    expect(__smExtractProperties(specs)).toEqual({ 1: { hasIcon: "true" } });
  });

  test("attr= reads the named attribute", () => {
    document.body.innerHTML =
      '<a class="link" href="/x" data-sightmap-id="1"></a>';
    const specs = [
      {
        id: "1",
        selector: ".link",
        props: [{ name: "href", extract: "attr=href" }],
      },
    ];
    expect(__smExtractProperties(specs)).toEqual({ 1: { href: "/x" } });
  });

  test("applies a transform to the extracted value", () => {
    document.body.innerHTML =
      '<div class="price" data-sightmap-id="1">Total: $12.50 today</div>';
    const specs = [
      {
        id: "1",
        selector: ".price",
        props: [{ name: "price", extract: "text", transform: "first_dollar" }],
      },
    ];
    expect(__smExtractProperties(specs)).toEqual({ 1: { price: "$12.50" } });
  });

  test("omits a property when extraction yields nothing", () => {
    document.body.innerHTML = '<div class="card" data-sightmap-id="1"></div>';
    const specs = [
      {
        id: "1",
        selector: ".card",
        props: [{ name: "label", extract: ".missing" }],
      },
    ];
    expect(__smExtractProperties(specs)).toEqual({});
  });

  test("skips a spec whose element is not found", () => {
    document.body.innerHTML = "<div></div>";
    const specs = [
      {
        id: "nope",
        selector: ".missing",
        props: [{ name: "label", extract: "text" }],
      },
    ];
    expect(__smExtractProperties(specs)).toEqual({});
  });
});
