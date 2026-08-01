package main

import (
	"bytes"
	"slices"
	"testing"
)

func TestFinalChromeArgs(t *testing.T) {
	base := []string{"--remote-debugging-port=9222", "--headless=new"}

	// Non-root: no --no-sandbox; extra flags appended.
	got := finalChromeArgs(base, 1000, []string{"--disable-gpu"})
	if slices.Contains(got, "--no-sandbox") {
		t.Error("non-root must not add --no-sandbox")
	}
	if !slices.Contains(got, "--disable-gpu") {
		t.Error("caller --chrome-flag not appended")
	}

	// Root: --no-sandbox added automatically.
	if got := finalChromeArgs(base, 0, nil); !slices.Contains(got, "--no-sandbox") {
		t.Error("root (euid 0) must add --no-sandbox")
	}

	// Base must not be mutated.
	if len(base) != 2 {
		t.Errorf("base slice was mutated: %v", base)
	}
}

func TestBoundedBufferTruncatesAndReports(t *testing.T) {
	var b boundedBuffer
	b.Write(bytes.Repeat([]byte("x"), chromeStderrCap+512))
	if got := len(b.buf); got != chromeStderrCap {
		t.Errorf("retained %d bytes, want cap %d", got, chromeStderrCap)
	}

	var empty boundedBuffer
	if r := empty.tailReport(); r == "" {
		t.Error("empty tailReport should still say something")
	}

	var withErr boundedBuffer
	withErr.Write([]byte("Running as root without --no-sandbox is not supported\n"))
	if r := withErr.tailReport(); !bytes.Contains([]byte(r), []byte("--no-sandbox")) {
		t.Errorf("tailReport should surface the stderr line, got: %q", r)
	}
}
