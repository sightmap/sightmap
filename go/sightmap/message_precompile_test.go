package sightmap

import "testing"

// The loader must precompile each message pattern once, so MessagesForRecord
// reuses it rather than recompiling per record. This is an internal test because
// the cached regexp is unexported; it guards against the loader hook silently
// rotting (matching would still work via the on-the-fly fallback, so a
// behavioral test wouldn't catch a missing precompile).
func TestToMessageDefsPrecompiles(t *testing.T) {
	defs := toMessageDefs([]rawMessage{
		{Name: "Valid", Message: "cart .* mismatch"},
		{Name: "LevelOnly", Level: "error"},
		{Name: "Invalid", Message: "("}, // unbalanced group
	})

	if defs[0].re == nil {
		t.Error("valid pattern should be precompiled at load")
	}
	if defs[1].re != nil {
		t.Error("a def with no pattern should have no compiled regexp")
	}
	if defs[2].re != nil {
		t.Error("an invalid pattern should leave re nil (reported by validation)")
	}
}
