package browser

import "testing"

// A date-like control sanitizes a value it cannot parse to "", but a color
// control sanitizes it to "#000000" and a range control to its midpoint, so
// fill cannot tell a rejection from the value read back being empty alone.
func TestNativeValueRejected(t *testing.T) {
	for _, c := range []struct {
		name, typ, want, got string
		rejected             bool
	}{
		{"date taken", "date", "2027-04-16", "2027-04-16", false},
		{"date sanitized to empty", "date", "next tuesday", "", true},
		{"time sanitized to empty", "time", "25:99", "", true},
		{"empty value clears a date", "date", "", "", false},
		{"color taken", "color", "#ff0000", "#ff0000", false},
		{"color taken in other case", "color", "#FF0000", "#ff0000", false},
		{"color sanitized to black", "color", "red", "#000000", true},
		{"color short form sanitized to black", "color", "#f00", "#000000", true},
		{"range taken", "range", "7", "7", false},
		{"range clamped into bounds", "range", "70", "10", false},
		{"range sanitized to midpoint", "range", "high", "5", true},
		{"range given nothing", "range", "", "5", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := nativeValueRejected(c.typ, c.want, c.got); got != c.rejected {
				t.Errorf("nativeValueRejected(%q, %q, %q) = %v, want %v", c.typ, c.want, c.got, got, c.rejected)
			}
		})
	}
}
