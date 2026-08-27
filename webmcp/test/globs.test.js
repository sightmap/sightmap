const { routeGlobToRegex, pathMatchesRoute } = require("../src/globs");

describe("route globs (spec: Route matching)", () => {
  test("* matches exactly one segment", () => {
    expect(pathMatchesRoute("/users/42", "/users/*")).toBe(true);
    expect(pathMatchesRoute("/users/42/edit", "/users/*")).toBe(false);
    expect(pathMatchesRoute("/users", "/users/*")).toBe(false);
  });

  test("** as its own segment matches zero or more segments", () => {
    expect(pathMatchesRoute("/admin", "/admin/**")).toBe(true);
    expect(pathMatchesRoute("/admin/users", "/admin/**")).toBe(true);
    expect(pathMatchesRoute("/admin/users/42/edit", "/admin/**")).toBe(true);
    expect(pathMatchesRoute("/a/b", "/a/**/b")).toBe(true);
    expect(pathMatchesRoute("/a/x/b", "/a/**/b")).toBe(true);
  });

  test("** glued into a segment degrades to in-segment *", () => {
    expect(pathMatchesRoute("/foobar", "/foo**")).toBe(true);
    expect(pathMatchesRoute("/foo/bar", "/foo**")).toBe(false);
  });

  test(":param normalizes to one segment", () => {
    expect(
      pathMatchesRoute("/us/en/cat/desk-chairs", "/us/en/cat/:categorySlug"),
    ).toBe(true);
    expect(pathMatchesRoute("/us/en/cat/a/b", "/us/en/cat/:categorySlug")).toBe(
      false,
    );
  });

  test("trailing slashes and query/fragment are normalized away", () => {
    expect(pathMatchesRoute("/search/", "/search")).toBe(true);
    expect(pathMatchesRoute("/search?q=x", "/search")).toBe(true);
  });

  test("root route", () => {
    expect(pathMatchesRoute("/", "/")).toBe(true);
    expect(pathMatchesRoute("/x", "/")).toBe(false);
  });

  test("catch-all", () => {
    expect(pathMatchesRoute("/anything/at/all", "/**")).toBe(true);
    expect(pathMatchesRoute("/", "/**")).toBe(true);
  });

  test("regex source is embeddable", () => {
    const re = new RegExp(routeGlobToRegex("/us/en/p/:productSlug").source);
    expect(re.test("/us/en/p/goersnygg-40504193")).toBe(true);
  });
});
