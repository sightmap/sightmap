package sightmap

import "testing"

func TestApplyTransform(t *testing.T) {
	cases := []struct {
		val, transform, want string
	}{
		{"hello world", "first_word", "hello"},
		{"hello world", "last_word", "world"},
		{"Items: 42 left", "first_number", "42"},
		{"Price: $12.99", "first_dollar", "$12.99"},
		{"  Price 99 ", "number", "99"},
		{"Hello World!", "slug", "hello-world"},
		{"no transform", "", "no transform"},
		// edge: empty val
		{"", "first_word", ""},
		// edge: unknown transform passes through
		{"hello", "unknown", "hello"},
		// first_word with single token
		{"alone", "first_word", "alone"},
		// last_word with single token
		{"alone", "last_word", "alone"},
		// first_number: no digits returns original
		{"no digits here", "first_number", "no digits here"},
		// first_dollar: no dollar returns original
		{"no price here", "first_dollar", "no price here"},
		// number: strips non-digit/non-dot
		{"$1,234.56", "number", "1234.56"},
		// slug: mixed case and punctuation
		{"Hello, World!", "slug", "hello-world"},
		// match: capture group 1 when present
		{"Add In-Store Assembly / FREE", `match:Add (In-\w+) Assembly`, "In-Store"},
		{"Add In-Home Assembly / +$179.00", `match:Add (In-\w+) Assembly`, "In-Home"},
		// match: enum-style alternation, no capture group → full match
		{"Add In-Home Assembly / +$179.00", `match:In-Store|In-Home`, "In-Home"},
		// match: price as FREE or dollar amount via a capture group
		{"Add In-Store Assembly / FREE", `match:(FREE|\$[\d,.]+)`, "FREE"},
		{"Add In-Home Assembly / +$179.00", `match:(FREE|\$[\d,.]+)`, "$179.00"},
		// match: no match → value unchanged
		{"pickupTile", `match:(delivery)`, "pickupTile"},
		// match: invalid pattern → value unchanged (no panic)
		{"anything", `match:(unclosed`, "anything"},
		// match: empty pattern matches at start → empty group-0
		{"abc", "match:def", "abc"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.val+"/"+tc.transform, func(t *testing.T) {
			got := ApplyTransform(tc.val, tc.transform)
			if got != tc.want {
				t.Errorf("ApplyTransform(%q, %q) = %q; want %q", tc.val, tc.transform, got, tc.want)
			}
		})
	}
}
