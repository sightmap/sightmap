package explore

import (
	"testing"
	"time"
)

func TestQuiet(t *testing.T) {
	base := sample{URL: "u", ReadyState: "complete", Mutations: 5, TextLen: 100}
	cases := []struct {
		name    string
		prev    sample
		cur     sample
		elapsed time.Duration
		want    bool
	}{
		{"identical", base, base, 100 * time.Millisecond, true},
		{"still loading", base, sample{URL: "u", ReadyState: "loading", Mutations: 5, TextLen: 100}, 100 * time.Millisecond, false},
		{"url changed", base, sample{URL: "v", ReadyState: "complete", Mutations: 5, TextLen: 100}, 100 * time.Millisecond, false},
		{"text changed", base, sample{URL: "u", ReadyState: "complete", Mutations: 5, TextLen: 101}, 100 * time.Millisecond, false},
		{"mutations early", base, sample{URL: "u", ReadyState: "complete", Mutations: 6, TextLen: 100}, 100 * time.Millisecond, false},
		{"mutations ignored after coarse", base, sample{URL: "u", ReadyState: "complete", Mutations: 6, TextLen: 100}, 1500 * time.Millisecond, true},
	}
	for _, c := range cases {
		if got := quiet(c.prev, c.cur, c.elapsed, time.Second); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
