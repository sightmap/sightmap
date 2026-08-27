const { parseQuery } = require("../src/query");

describe("component query parser", () => {
  test("bare name", () => {
    expect(parseQuery("ProductCard")).toEqual({
      kind: "query",
      links: [{ name: "ProductCard", preds: [], index: null }],
    });
  });

  test("predicates with each operator", () => {
    const q = parseQuery('Card[title="Chairs"][price^=$1][text*="weber" i]');
    expect(q.links[0].preds).toEqual([
      { prop: "title", op: "=", value: "Chairs", ci: false },
      { prop: "price", op: "^=", value: "$1", ci: false },
      { prop: "text", op: "*=", value: "weber", ci: true },
    ]);
  });

  test("bare value with i flag", () => {
    expect(parseQuery("Card[title*=weber i]").links[0].preds[0]).toEqual({
      prop: "title",
      op: "*=",
      value: "weber",
      ci: true,
    });
  });

  test("descendant chain with predicate on ancestor", () => {
    const q = parseQuery('Row[label="Desk"] Buy');
    expect(q.links.map((l) => l.name)).toEqual(["Row", "Buy"]);
    expect(q.links[0].preds).toHaveLength(1);
  });

  test("occurrence index", () => {
    expect(parseQuery("Tile#1").links[0].index).toBe(1);
  });

  test("predicate values keep spaces and template refs", () => {
    expect(parseQuery('Row[label="a b {x}"]').links[0].preds[0].value).toBe(
      "a b {x}",
    );
  });

  test("css escape hatch", () => {
    expect(parseQuery("css:.plp-price-module a")).toEqual({
      kind: "css",
      selector: ".plp-price-module a",
    });
  });

  test("child combinator is rejected with guidance", () => {
    expect(() => parseQuery("A > B")).toThrow(/descendant/);
  });

  test("unterminated predicate", () => {
    expect(() => parseQuery("Card[title=")).toThrow(/unterminated/);
  });
});
