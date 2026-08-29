package webmcp

import (
	"regexp"
	"testing"
)

func TestPathMatchesRouteOneSegmentStar(t *testing.T) {
	if !pathMatchesRoute("/users/42", "/users/*") {
		t.Fatal("expected match")
	}
	if pathMatchesRoute("/users/42/edit", "/users/*") {
		t.Fatal("star must not cross a slash")
	}
	if pathMatchesRoute("/users", "/users/*") {
		t.Fatal("star needs a segment")
	}
}

func TestPathMatchesRouteGlobstar(t *testing.T) {
	cases := []struct {
		path, route string
		want        bool
	}{
		{"/admin", "/admin/**", true},
		{"/admin/users", "/admin/**", true},
		{"/admin/users/42/edit", "/admin/**", true},
		{"/a/b", "/a/**/b", true},
		{"/a/x/b", "/a/**/b", true},
	}
	for _, tc := range cases {
		if got := pathMatchesRoute(tc.path, tc.route); got != tc.want {
			t.Errorf("pathMatchesRoute(%q, %q) = %v, want %v", tc.path, tc.route, got, tc.want)
		}
	}
}

func TestPathMatchesRouteGluedGlobstarDegradesToInSegmentStar(t *testing.T) {
	if !pathMatchesRoute("/foobar", "/foo**") {
		t.Fatal("glued ** should match inside the segment")
	}
	if pathMatchesRoute("/foo/bar", "/foo**") {
		t.Fatal("glued ** must not cross a slash")
	}
}

func TestPathMatchesRouteParamIsOneSegment(t *testing.T) {
	if !pathMatchesRoute("/us/en/cat/desk-chairs", "/us/en/cat/:categorySlug") {
		t.Fatal("expected match")
	}
	if pathMatchesRoute("/us/en/cat/a/b", "/us/en/cat/:categorySlug") {
		t.Fatal(":param is one segment")
	}
}

func TestPathMatchesRouteNormalizesSlashQueryFragment(t *testing.T) {
	if !pathMatchesRoute("/search/", "/search") {
		t.Fatal("trailing slash")
	}
	if !pathMatchesRoute("/search?q=x", "/search") {
		t.Fatal("query")
	}
}

func TestPathMatchesRouteRoot(t *testing.T) {
	if !pathMatchesRoute("/", "/") {
		t.Fatal("root")
	}
	if pathMatchesRoute("/x", "/") {
		t.Fatal("root is exact")
	}
}

func TestPathMatchesRouteCatchAll(t *testing.T) {
	if !pathMatchesRoute("/anything/at/all", "/**") {
		t.Fatal("catch-all path")
	}
	if !pathMatchesRoute("/", "/**") {
		t.Fatal("catch-all root")
	}
}

func TestRouteGlobRegexSourceIsEmbeddable(t *testing.T) {
	re, err := regexp.Compile(routeGlobRegexSource("/us/en/p/:productSlug"))
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("/us/en/p/goersnygg-40504193") {
		t.Fatalf("source %q did not match", routeGlobRegexSource("/us/en/p/:productSlug"))
	}
}
