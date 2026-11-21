package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The settle fixture's start() attaches a shadow root holding another, the way
// components render, then changes text in the inner one every 100 ms, eight
// times. Nothing outside the shadow trees changes after the first 25 ms, so a
// document-only observer calls it quiet mid-count.
const settleFixture = `<!doctype html><html><body>
<div id="host"></div>
<script>
function start() {
  const root = document.getElementById("host").attachShadow({ mode: "open" });
  const inner = document.createElement("div");
  root.appendChild(inner);
  const nested = inner.attachShadow({ mode: "open" });
  let n = 0;
  const t = setInterval(() => { nested.textContent = "tick " + (++n); if (n === 8) clearInterval(t); }, 100);
}
</script>
</body></html>`

// TestWaitSettledShadowDOM_Chrome checks that WaitSettled sees changes inside
// (nested) shadow roots, which a MutationObserver on the document does not, and
// that a wait that ends leaves no observer behind.
//
// Opt-in: SIGHTMAP_CHROME_E2E=1 (uses FindChrome, or SIGHTMAP_CHROME).
func TestWaitSettledShadowDOM_Chrome(t *testing.T) {
	ctx, conn := fullPageChrome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, settleFixture)
	}))
	t.Cleanup(srv.Close)
	if err := NavigateAndWaitIdle(ctx, conn, srv.URL+"/", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	// Start the shadow content 25 ms from now, so the wait is already watching.
	if _, err := EvalJSON(ctx, conn, `setTimeout(start, 25), true`); err != nil {
		t.Fatal(err)
	}

	r, err := WaitSettled(ctx, conn, 300*time.Millisecond, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Quiet {
		t.Fatalf("WaitSettled never went quiet (%v)", r.Elapsed)
	}
	// Settled means the shadow text has stopped at its last value; an observer that
	// can't see into shadow trees settles mid-count.
	raw, err := EvalJSON(ctx, conn, `(() => { const r = document.getElementById("host").shadowRoot.firstChild.shadowRoot; return r.textContent; })()`)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(raw); got != `"tick 8"` {
		t.Errorf("shadow text when settled = %s, want \"tick 8\" (settled after %v while the shadow root was still changing)", got, r.Elapsed)
	}
}

// TestAwaitNavigationUntil_Chrome checks the early stop navigate uses: a stop
// before any navigation ends the wait at once with no move, and a stop that
// comes after a client-side redirect has started is ignored, so the redirect is
// still reported.
func TestAwaitNavigationUntil_Chrome(t *testing.T) {
	ctx, conn := fullPageChrome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><body>ok</body></html>`)
	}))
	t.Cleanup(srv.Close)
	if err := NavigateAndWaitIdle(ctx, conn, srv.URL+"/", 5*time.Second); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	time.AfterFunc(100*time.Millisecond, func() { close(stop) })
	start := time.Now()
	if url, moved := AwaitNavigationUntil(ctx, conn, 3*time.Second, stop); moved {
		t.Fatalf("no navigation happened, but it reported a move to %q", url)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("a stop with no navigation should end the wait at once; it took %v", d)
	}

	// The redirect starts at 50 ms; the stop fires at 100 ms, mid-grace.
	if _, err := EvalJSON(ctx, conn, `setTimeout(() => history.pushState(null, "", "/landed"), 50), true`); err != nil {
		t.Fatal(err)
	}
	stop = make(chan struct{})
	time.AfterFunc(100*time.Millisecond, func() { close(stop) })
	url, moved := AwaitNavigationUntil(ctx, conn, 3*time.Second, stop)
	if !moved || url != srv.URL+"/landed" {
		t.Errorf("a redirect under way when the stop fires is still reported: got %q, %v; want %q, true", url, moved, srv.URL+"/landed")
	}
}

// TestWaitSettledXHRThrow_Chrome checks that an XMLHttpRequest whose send throws
// does not leave the in-flight counter up, which would make every later settle
// run to its cap.
func TestWaitSettledXHRThrow_Chrome(t *testing.T) {
	ctx, conn := fullPageChrome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><body>ok</body></html>`)
	}))
	t.Cleanup(srv.Close)
	if err := NavigateAndWaitIdle(ctx, conn, srv.URL+"/", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := WaitSettled(ctx, conn, 100*time.Millisecond, 2*time.Second); err != nil { // installs the tracker
		t.Fatal(err)
	}
	// send() before open() throws InvalidStateError synchronously.
	if _, err := EvalJSON(ctx, conn, `(() => { try { new XMLHttpRequest().send(); } catch (e) {} return window.__smInflight; })()`); err != nil {
		t.Fatal(err)
	}
	raw, err := EvalJSON(ctx, conn, `window.__smInflight`)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "0" {
		t.Fatalf("in-flight count after a throwing send = %s, want 0", raw)
	}
	r, err := WaitSettled(ctx, conn, 100*time.Millisecond, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Quiet || r.Elapsed > time.Second {
		t.Errorf("settle after a throwing send = %+v, want quiet well before the 2 s cap", r)
	}
}
