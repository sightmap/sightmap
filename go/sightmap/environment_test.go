package sightmap_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func loadFiles(t *testing.T, files map[string]string) *sightmap.Corpus {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		writeFile(t, filepath.Join(dir, name), content)
	}
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

const envCorpus = `
version: 1
environments:
  - name: local
    origins:
      api: http://localhost:8080
      app: http://localhost:*
  - name: prod
    origins:
      app: https://app.acme.com
  - name: android-prod
    platform: android
    app_id: com.acme.app
    build_type: release
    backend: prod
    origins:
      telemetry: https://android.telemetry.example.com
origins:
  api: https://api.acme.com
  telemetry: https://telemetry.example.com
views:
  - name: Orders
    route: /orders
    environments: [local, prod]
    origins: [app]
    requests:
      - name: ListOrders
        route: /api/orders
        method: GET
        environments: [prod, android-prod]
        origins: [api]
requests:
  - name: Pixel
    route: /tr
    origins: [telemetry]
`

func TestLoad_Environments(t *testing.T) {
	c := loadFiles(t, map[string]string{"app.yaml": envCorpus})

	if len(c.Environments) != 3 {
		t.Fatalf("want 3 environments, got %+v", c.Environments)
	}
	local := c.EnvironmentByName("local")
	if local == nil || local.Platform != sightmap.PlatformWeb {
		t.Fatalf("an omitted platform must load as web, got %+v", local)
	}
	if local.SourceFile != "app.yaml" || local.Origins["app"] != "http://localhost:*" {
		t.Errorf("local did not load faithfully: %+v", local)
	}
	android := c.EnvironmentByName("android-prod")
	if android == nil || !android.IsNative() || android.AppID != "com.acme.app" || android.BuildType != "release" || android.Backend != "prod" {
		t.Fatalf("android-prod did not load faithfully: %+v", android)
	}
	if want := map[string]string{"api": "https://api.acme.com", "telemetry": "https://telemetry.example.com"}; !reflect.DeepEqual(c.SharedOrigins, want) {
		t.Errorf("SharedOrigins = %v, want %v", c.SharedOrigins, want)
	}

	v := c.ViewByName("Orders")
	if !reflect.DeepEqual(v.Environments, []string{"local", "prod"}) || !reflect.DeepEqual(v.Origins, []string{"app"}) {
		t.Errorf("view refs did not load: %+v", v)
	}
	if r := v.Requests[0]; !reflect.DeepEqual(r.Environments, []string{"prod", "android-prod"}) || !reflect.DeepEqual(r.Origins, []string{"api"}) {
		t.Errorf("view-scoped request refs did not load: %+v", r)
	}
	if r := c.Requests[0]; !reflect.DeepEqual(r.Origins, []string{"telemetry"}) || r.Environments != nil {
		t.Errorf("global request refs did not load: %+v", r)
	}
	if errs := sightmap.Validate(c); len(errs) != 0 {
		t.Errorf("valid corpus produced diagnostics: %v", errs)
	}
}

func TestResolveOrigin(t *testing.T) {
	c := loadFiles(t, map[string]string{"app.yaml": envCorpus})
	cases := []struct {
		env, origin, want string
		ok                bool
	}{
		{"local", "api", "http://localhost:8080", true},                              // own entry overrides shared
		{"prod", "api", "https://api.acme.com", true},                                // falls through to shared
		{"prod", "app", "https://app.acme.com", true},                                // own entry
		{"android-prod", "app", "https://app.acme.com", true},                        // borrowed from backend
		{"android-prod", "telemetry", "https://android.telemetry.example.com", true}, // own beats shared
		{"android-prod", "api", "https://api.acme.com", true},                        // shared, via neither
		{"prod", "admin", "", false},
		{"nope", "api", "", false},
	}
	for _, tc := range cases {
		got, ok := c.ResolveOrigin(tc.env, tc.origin)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ResolveOrigin(%q, %q) = %q, %v; want %q, %v", tc.env, tc.origin, got, ok, tc.want, tc.ok)
		}
	}
}

// A backend's own origins override the shared map, and a native environment's
// own origins override its backend's.
func TestResolveOrigin_BackendBeatsShared(t *testing.T) {
	c := &sightmap.Corpus{
		Environments: []sightmap.EnvironmentDef{
			{Name: "prod", Platform: "web", Origins: map[string]string{"api": "https://api.acme.com"}},
			{Name: "ios", Platform: "ios", AppID: "com.acme", Backend: "prod"},
		},
		SharedOrigins: map[string]string{"api": "https://shared.acme.com"},
	}
	if got, _ := c.ResolveOrigin("ios", "api"); got != "https://api.acme.com" {
		t.Errorf("want the backend's api, got %q", got)
	}
}

func TestRequestEnvironments(t *testing.T) {
	view := &sightmap.ViewDef{Environments: []string{"local", "prod"}}
	cases := []struct {
		name        string
		view        *sightmap.ViewDef
		own         []string
		want        []string
		constrained bool
	}{
		{"global, unconstrained", nil, nil, nil, false},
		{"global, own list", nil, []string{"prod", "prod"}, []string{"prod"}, true},
		{"empty own list is unconstrained", nil, []string{}, nil, false},
		{"inherits view", view, nil, []string{"local", "prod"}, true},
		{"intersects", view, []string{"prod", "android"}, []string{"prod"}, true},
		{"disjoint", view, []string{"android"}, []string{}, true},
		{"empty view list adds nothing", &sightmap.ViewDef{Environments: []string{}}, []string{"prod"}, []string{"prod"}, true},
	}
	for _, tc := range cases {
		got, constrained := sightmap.RequestEnvironments(tc.view, sightmap.RequestDef{Environments: tc.own})
		if !reflect.DeepEqual(got, tc.want) || constrained != tc.constrained {
			t.Errorf("%s: got %v, %v; want %v, %v", tc.name, got, constrained, tc.want, tc.constrained)
		}
	}
}

// First by source path wins; the loser is dropped and warned about.
func TestLoad_EnvironmentRegistryFirstWins(t *testing.T) {
	c := loadFiles(t, map[string]string{
		"a.yaml": "version: 1\nenvironments:\n  - name: prod\n    origins: {app: 'https://a.acme.com'}\norigins:\n  cdn: https://cdn-a.acme.com\n",
		"b.yaml": "version: 1\nenvironments:\n  - name: prod\n    origins: {app: 'https://b.acme.com'}\norigins:\n  cdn: https://cdn-b.acme.com\n",
	})
	if len(c.Environments) != 1 || c.Environments[0].Origins["app"] != "https://a.acme.com" {
		t.Fatalf("want a.yaml's prod only, got %+v", c.Environments)
	}
	if c.SharedOrigins["cdn"] != "https://cdn-a.acme.com" {
		t.Errorf("want a.yaml's cdn, got %q", c.SharedOrigins["cdn"])
	}
	errs := sightmap.Validate(c)
	for _, code := range []string{"environment-name-collision", "origin-name-collision"} {
		var found *sightmap.ValidationError
		for i := range errs {
			if errs[i].Code == code {
				found = &errs[i]
			}
		}
		if found == nil {
			t.Fatalf("want %s, got %v", code, findingCodes(errs))
		}
		if found.IsError() || found.File != "b.yaml" || !strings.Contains(found.Message, "a.yaml") {
			t.Errorf("%s should be a warning on the loser naming the winner, got %+v", code, *found)
		}
	}
}

// An explicit [] must survive loading as non-nil, or environments-empty could
// never be told apart from an omitted list.
func TestLoad_EmptyReferenceListsSurvive(t *testing.T) {
	c := loadFiles(t, map[string]string{"app.yaml": `
version: 1
views:
  - name: Home
    route: /
    environments: []
    origins: []
  - name: About
    route: /about
`})
	home, about := c.ViewByName("Home"), c.ViewByName("About")
	if home.Environments == nil || home.Origins == nil || len(home.Environments) != 0 {
		t.Errorf("explicit [] must load as an empty non-nil slice, got %#v / %#v", home.Environments, home.Origins)
	}
	if about.Environments != nil || about.Origins != nil {
		t.Errorf("an omitted list must load as nil, got %#v / %#v", about.Environments, about.Origins)
	}
}

// A definition on a view, or a bare name at a file root, cannot decode into the
// typed fields, so the loader refuses the file.
func TestLoad_RejectsMisplacedDefinitionsAndNames(t *testing.T) {
	for name, src := range map[string]string{
		"definition on a view": "version: 1\nviews:\n  - name: Home\n    route: /\n    environments:\n      - name: prod\n        origins: {app: 'https://a.com'}\n",
		"bare names at root":   "version: 1\norigins: [api, app]\n",
		"bare env names":       "version: 1\nenvironments: [prod]\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "app.yaml"), src)
			if _, err := sightmap.DirLoader(dir).Load(); err == nil {
				t.Fatal("want a load error")
			}
		})
	}
}

// Environments and origins are not route-matching inputs: a URL outside an
// entity's environments and origins still matches its route.
func TestEnvironmentsDoNotAffectMatching(t *testing.T) {
	c := loadFiles(t, map[string]string{"app.yaml": envCorpus})
	if v := c.ViewForURL("https://elsewhere.example.org/orders"); v == nil || v.Name != "Orders" {
		t.Errorf("ViewForURL must ignore origins, got %+v", v)
	}
	if rs := c.RequestsForURL("https://elsewhere.example.org/api/orders", "GET"); len(rs) != 1 || rs[0].Name != "ListOrders" {
		t.Errorf("RequestsForURL must ignore environments and origins, got %+v", rs)
	}
}
