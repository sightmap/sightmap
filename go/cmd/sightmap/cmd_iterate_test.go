package main

import "testing"

func TestURLSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://www.vividseats.com/concerts", "concerts"},
		{"https://www.vividseats.com/", "home"},
		{"https://www.vividseats.com", "home"},
		{"https://www.vividseats.com/foo/bar/baz", "foo-bar-baz"},
		{"https://www.vividseats.com/foo/bar/baz?q=1", "foo-bar-baz"},
		{"https://www.vividseats.com/foo/bar/baz#section", "foo-bar-baz"},
		{"https://www.vividseats.com/UPPER-CASE", "upper-case"},
		{
			// path longer than 40 chars gets truncated
			"https://example.com/a/very/long/path/that/exceeds/the/limit",
			"a-very-long-path-that-exceeds-the-limit",
		},
	}
	for _, tt := range tests {
		got := urlSlug(tt.input)
		if got != tt.want {
			t.Errorf("urlSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
