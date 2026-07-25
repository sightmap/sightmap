package authoring

import "testing"

func TestNormalizePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/products/12345", "/products/*"},
		{"/p/garden-hose-50ft", "/p/*"},                               // slug with digit → dynamic
		{"/category/outdoor", "/category/outdoor"},                    // pure-word slug → kept
		{"/orders/550e8400-e29b-41d4-a716-446655440000", "/orders/*"}, // UUID
		{"/", "/"},
		{"/about", "/about"},
		{"/u/42/settings", "/u/*/settings"},
	}
	for _, c := range cases {
		if got := NormalizePath(c.in); got != c.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
