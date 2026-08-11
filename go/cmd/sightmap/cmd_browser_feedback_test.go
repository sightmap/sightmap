package main

import (
	"errors"
	"github.com/sightmap/sightmap/go/sightmap"
	"testing"
)

func TestActionLabel(t *testing.T) {
	withBounds := &sightmap.ComponentNode{
		Id:     "42",
		Bounds: &sightmap.Bounds{X: 100, Y: 40, Width: 80, Height: 20},
	}
	if got, want := actionLabel("CartNavButton", withBounds), "CartNavButton @ (140,50)"; got != want {
		t.Errorf("actionLabel(bounds) = %q, want %q", got, want)
	}

	noBounds := &sightmap.ComponentNode{Id: "7"}
	if got, want := actionLabel("7", noBounds), "7"; got != want {
		t.Errorf("actionLabel(no bounds) = %q, want %q", got, want)
	}
	if got, want := actionLabel("Foo", nil), "Foo"; got != want {
		t.Errorf("actionLabel(nil) = %q, want %q", got, want)
	}
}

func TestFriendlyBodyErr(t *testing.T) {
	evicted := errors.New("daemon returned 500 Internal Server Error: Network.getResponseBody error -32000: No resource with given identifier found")
	got := friendlyBodyErr(evicted)
	if want := "evicted from the network cache"; !contains(got, want) {
		t.Errorf("friendlyBodyErr(evicted) = %q, want it to mention %q", got, want)
	}

	other := errors.New("connection refused")
	if got := friendlyBodyErr(other); !contains(got, "connection refused") {
		t.Errorf("friendlyBodyErr(other) = %q, want it to include the raw message", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
