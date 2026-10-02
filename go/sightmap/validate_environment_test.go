package sightmap_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func TestValidate_Environments(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want []string // every diagnostic code, sorted
	}{
		{
			name: "valid web and native",
			yaml: `
environments:
  - name: prod
    origins: {api: 'https://api.acme.com', app: 'https://**.acme.com:*'}
  - name: ios
    platform: ios
    app_id: com.acme.App_1
    backend: prod
`,
		},
		{
			name: "neither field declared",
			yaml: `
views:
  - name: Home
    route: /
requests:
  - name: R
    route: /r
`,
		},
		{
			name: "web without origins",
			yaml: "environments:\n  - name: prod\n",
			want: []string{"environment-invalid"},
		},
		{
			name: "web with native fields",
			yaml: "environments:\n  - name: prod\n    app_id: com.acme\n    build_type: beta\n    origins: {app: 'https://a.com'}\n",
			want: []string{"environment-invalid"},
		},
		{
			name: "explicit web with backend",
			yaml: "environments:\n  - name: s\n    origins: {app: 'https://s.com'}\n  - name: prod\n    platform: web\n    backend: s\n    origins: {app: 'https://a.com'}\n",
			want: []string{"environment-invalid"},
		},
		{
			name: "native without app_id",
			yaml: "environments:\n  - name: ios\n    platform: ios\n",
			want: []string{"environment-invalid"},
		},
		{
			name: "native with malformed app_id",
			yaml: "environments:\n  - name: ios\n    platform: ios\n    app_id: 'com..acme'\n",
			want: []string{"environment-invalid"},
		},
		{
			name: "unknown platform",
			yaml: "environments:\n  - name: win\n    platform: windows\n    origins: {app: 'https://a.com'}\n",
			want: []string{"environment-invalid"},
		},
		{
			name: "bad environment name",
			yaml: "environments:\n  - name: Prod\n    origins: {app: 'https://a.com'}\n",
			want: []string{"environment-invalid"},
		},
		{
			name: "missing environment name",
			yaml: "environments:\n  - origins: {app: 'https://a.com'}\n",
			want: []string{"environment-invalid"},
		},
		{
			name: "backend names nothing",
			yaml: "environments:\n  - name: ios\n    platform: ios\n    app_id: com.acme\n    backend: prod\n",
			want: []string{"environment-backend-invalid"},
		},
		{
			name: "backend names a native environment",
			yaml: `
environments:
  - name: android
    platform: android
    app_id: com.acme.android
  - name: ios
    platform: ios
    app_id: com.acme.ios
    backend: android
`,
			want: []string{"environment-backend-invalid"},
		},
		{
			name: "origin URLs the grammar rejects",
			yaml: `
origins:
  path: https://a.com/x
  scheme: ftp://a.com
  inner-wildcard: https://a.*.com
  port-range: https://a.com:65536
  port-zero: https://a.com:0
  Upper: https://a.com
`,
			want: []string{"origin-invalid", "origin-invalid", "origin-invalid", "origin-invalid", "origin-invalid", "origin-invalid"},
		},
		{
			name: "origin patterns the grammar accepts",
			yaml: `
origins:
  a: http://localhost:*
  b: https://deploy-preview-*--acme.netlify.app
  c: https://*.preview.acme.com
  d: https://**.acme.com
  e: http://127.0.0.1:65535
`,
		},
		{
			name: "invalid environment origin",
			yaml: "environments:\n  - name: prod\n    origins: {app: 'https://a.com/path'}\n",
			want: []string{"origin-invalid"},
		},
		{
			name: "unresolved references everywhere",
			yaml: `
environments:
  - name: prod
    origins: {app: 'https://a.com'}
views:
  - name: Home
    route: /
    environments: [prod, qa, qa]
    origins: [app, cdn]
    requests:
      - name: Load
        route: /load
        environments: [dev]
        origins: [api]
requests:
  - name: Pixel
    route: /tr
    environments: [canary]
    origins: [facebook]
`,
			want: []string{
				"environment-ref-unresolved", "environment-ref-unresolved", "environment-ref-unresolved",
				"origin-ref-unresolved", "origin-ref-unresolved", "origin-ref-unresolved",
			},
		},
		{
			name: "origin defined only on a native environment resolves",
			yaml: `
environments:
  - name: ios
    platform: ios
    app_id: com.acme
    origins: {push: 'https://push.acme.com'}
requests:
  - name: Register
    route: /register
    origins: [push]
`,
		},
		{
			name: "explicit empty lists",
			yaml: `
views:
  - name: Home
    route: /
    environments: []
    origins: []
    requests:
      - name: Load
        route: /load
        environments: []
requests:
  - name: Pixel
    route: /tr
    origins: []
`,
			want: []string{"environments-empty", "environments-empty", "origins-empty", "origins-empty"},
		},
		{
			name: "origin gap across web environments",
			yaml: `
environments:
  - name: prod
    origins: {app: 'https://a.com', admin: 'https://admin.a.com'}
  - name: staging
    origins: {app: 'https://s.a.com'}
  - name: local
    origins: {app: 'http://localhost:*'}
  - name: ios
    platform: ios
    app_id: com.acme
    origins: {only-native: 'https://n.a.com'}
`,
			want: []string{"origin-environment-gap"},
		},
		{
			name: "shared map closes the gap",
			yaml: `
environments:
  - name: prod
    origins: {app: 'https://a.com', api: 'https://api.a.com'}
  - name: staging
    origins: {app: 'https://s.a.com'}
origins:
  api: https://api.shared.com
`,
		},
		{
			name: "host shared ignoring scheme, case, and default port",
			yaml: `
environments:
  - name: preview
    origins: {app: 'https://preview.a.com', api: 'https://API.a.com:443'}
  - name: staging
    origins: {app: 'https://s.a.com', api: 'http://api.a.com'}
`,
			want: []string{"origin-host-shared"},
		},
		{
			name: "different ports are different hosts",
			yaml: `
environments:
  - name: one
    origins: {app: 'http://localhost:3000'}
  - name: two
    origins: {app: 'http://localhost:3001'}
`,
		},
		{
			name: "duplicate native target",
			yaml: `
environments:
  - name: android-a
    platform: android
    app_id: com.acme
    build_type: release
  - name: android-b
    platform: android
    app_id: com.acme
    build_type: release
  - name: android-beta
    platform: android
    app_id: com.acme
    build_type: beta
  - name: ios-a
    platform: ios
    app_id: com.acme
  - name: ios-b
    platform: ios
    app_id: com.acme
`,
			want: []string{"environment-duplicate", "environment-duplicate"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := loadFiles(t, map[string]string{"app.yaml": "version: 1\n" + tc.yaml})
			got := findingCodes(sightmap.Validate(c))
			sort.Strings(got)
			want := tc.want
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("codes = %v, want %v\n%v", got, want, sightmap.Validate(c))
			}
		})
	}
}

// Severities are part of the contract: the MUST-reject codes fail validation
// and the SHOULD/MAY codes do not.
func TestValidate_EnvironmentSeverities(t *testing.T) {
	c := loadFiles(t, map[string]string{"app.yaml": `
version: 1
environments:
  - name: prod
    origins: {app: 'https://a.com:70000', admin: 'https://admin.a.com'}
  - name: staging
    origins: {app: 'https://a.com:70000'}
  - name: ios
    platform: ios
    app_id: com.acme
    backend: nope
  - name: ios2
    platform: ios
    app_id: com.acme
  - name: web
    app_id: com.acme
    origins: {app: 'https://w.com'}
views:
  - name: Home
    route: /
    environments: [qa]
    origins: []
`})
	errorCodes := map[string]bool{
		"environment-invalid": true, "origin-invalid": true, "environment-backend-invalid": true,
		"environment-ref-unresolved": true, "origin-ref-unresolved": true,
	}
	seen := map[string]bool{}
	for _, f := range sightmap.Validate(c) {
		seen[f.Code] = true
		if f.IsError() != errorCodes[f.Code] {
			t.Errorf("%s: IsError() = %v, want %v", f.Code, f.IsError(), errorCodes[f.Code])
		}
	}
	for _, code := range []string{"environment-invalid", "origin-invalid", "environment-backend-invalid", "environment-ref-unresolved", "origin-environment-gap", "environment-duplicate", "origins-empty"} {
		if !seen[code] {
			t.Errorf("expected %s in the run", code)
		}
	}
}
