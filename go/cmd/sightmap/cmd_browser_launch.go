package main

import (
	"strings"
	"sync"
)

// stringSliceFlag collects a repeatable string flag (e.g. --chrome-flag).
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, " ") }

func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// finalChromeArgs appends the root --no-sandbox default (euid 0) and any
// caller-supplied flags to base, without mutating base. Chrome refuses to start
// as root without --no-sandbox, which is the standard agent/CI/container env.
// euid is a parameter (rather than reading os.Geteuid directly) so the root
// behaviour is unit-testable on any host.
func finalChromeArgs(base []string, euid int, extra []string) []string {
	out := append([]string(nil), base...)
	if euid == 0 {
		out = append(out, "--no-sandbox")
	}
	out = append(out, extra...)
	return out
}

// chromeStderrCap bounds how many bytes of Chrome's stderr we retain for the
// failure report — enough for the tail that names the real cause.
const chromeStderrCap = 8 << 10

// boundedBuffer is an io.Writer that retains only the last chromeStderrCap
// bytes, so a chatty child can't grow it without bound. Safe for concurrent
// writes: os/exec copies a process's stderr on its own goroutine.
type boundedBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > chromeStderrCap {
		b.buf = b.buf[len(b.buf)-chromeStderrCap:]
	}
	return len(p), nil
}

// tailReport renders the retained stderr as an indented block for the
// "did not become ready" error, so the real cause (e.g. the root/--no-sandbox
// message) is visible in one shot instead of hidden behind a bare CDP timeout.
func (b *boundedBuffer) tailReport() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := strings.TrimSpace(string(b.buf))
	if s == "" {
		return "  (Chrome produced no stderr output)"
	}
	var sb strings.Builder
	sb.WriteString("  chrome stderr (tail):\n")
	for _, line := range strings.Split(s, "\n") {
		sb.WriteString("    ")
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}
