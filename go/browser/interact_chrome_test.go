package browser

// Real-Chrome tests of how input is dispatched: native-value inputs (dates).
// Opt-in: SIGHTMAP_CHROME_E2E=1 (uses FindChrome, or SIGHTMAP_CHROME).

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A page with a date input, a text input and two frames (same-origin and
// cross-origin, so Chrome gives the second its own process), each holding a date
// input of its own. A listener on every input records the events it saw, so a
// test can tell the value was set the way a user's edit would have set it.
const interactFixtureTop = `<!doctype html><html><body style="margin:0">
<input id="d-top" type="date" value="2027-04-09" style="position:absolute;left:10px;top:10px;width:160px;height:30px">
<input id="t-top" type="text" style="position:absolute;left:10px;top:60px;width:160px;height:30px">
<input id="time-top" type="time" value="09:30" style="position:absolute;left:10px;top:110px;width:160px;height:30px">
<input id="range-top" type="range" min="0" max="10" value="2" style="position:absolute;left:10px;top:160px;width:160px;height:30px">
<iframe id="same" src="/frame?id=d-same" style="position:absolute;left:200px;top:10px;width:220px;height:80px;border:0"></iframe>
<iframe id="cross" src="%s/frame?id=d-cross" style="position:absolute;left:200px;top:110px;width:220px;height:80px;border:0"></iframe>
<script>
addEventListener('input', e => { document.body.dataset.events = (document.body.dataset.events || '') + e.target.id + ':input,'; }, true);
addEventListener('change', e => { document.body.dataset.events = (document.body.dataset.events || '') + e.target.id + ':change,'; }, true);
</script>
</body></html>`

const interactFixtureFrame = `<!doctype html><html><body style="margin:0">
<input id="%s" type="date" value="2027-04-09" style="position:absolute;left:10px;top:10px;width:160px;height:30px">
<script>
addEventListener('input', e => { document.body.dataset.events = (document.body.dataset.events || '') + e.target.id + ':input,'; }, true);
addEventListener('change', e => { document.body.dataset.events = (document.body.dataset.events || '') + e.target.id + ':change,'; }, true);
</script>
</body></html>`

// interactFixture serves the fixture and returns the top page's URL (host
// localhost) and the other origin's (127.0.0.1).
func interactFixture(t *testing.T) (top, cross string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	cross = fmt.Sprintf("http://127.0.0.1:%d", port)
	top = fmt.Sprintf("http://localhost:%d", port)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, interactFixtureTop, cross)
	})
	mux.HandleFunc("/frame", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, interactFixtureFrame, r.URL.Query().Get("id"))
	})
	srv := httptest.NewUnstartedServer(mux)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return top, cross
}

// loadInteractFixture opens the fixture in a tab of the browser at addr and
// waits for its two frames to have loaded.
func loadInteractFixture(t *testing.T, ctx context.Context, conn *CDPConn) {
	t.Helper()
	top, _ := interactFixture(t)
	if err := NavigateAndWaitIdle(ctx, conn, top+"/", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	for len(ChildFrames(ctx, conn)) != 2 {
		if ctx.Err() != nil {
			t.Fatal("the frames never loaded")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// evalString evaluates expr in the top document and returns its string value.
func evalString(t *testing.T, ctx context.Context, conn *CDPConn, expr string) string {
	t.Helper()
	raw, err := EvalJSON(ctx, conn, expr)
	if err != nil {
		t.Fatal(err)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("%s: %v (%s)", expr, err, raw)
	}
	return s
}

// frameString evaluates expr in the child frame whose URL contains urlPart.
func frameString(t *testing.T, ctx context.Context, conn *CDPConn, urlPart, expr string) string {
	t.Helper()
	for _, f := range ChildFrames(ctx, conn) {
		if strings.Contains(f.URL, urlPart) {
			raw, err := f.EvalJSON(ctx, expr)
			if err != nil {
				t.Fatal(err)
			}
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				t.Fatalf("%s: %v (%s)", expr, err, raw)
			}
			return s
		}
	}
	t.Fatalf("no frame with URL containing %q", urlPart)
	return ""
}

// TestFillNativeInputs_Chrome fills date, time and range inputs, in the page and
// in a same-origin and a cross-origin frame. Typed keystrokes would go into a
// date control's segmented editor ("2027-04-16" into 2027-04-09 leaves
// "270416-04-09"); the value must be set whole, with input and change fired, and
// a text input must still be typed into.
func TestFillNativeInputs_Chrome(t *testing.T) {
	ctx, conn := fullPageChrome(t)
	loadInteractFixture(t, ctx, conn)
	root, _ := extractFixture(t, ctx, conn)

	if err := Fill(ctx, conn, byDOMID(root, "d-top"), "2027-04-16"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, conn, `document.getElementById("d-top").value`); got != "2027-04-16" {
		t.Errorf("date in the page = %q, want 2027-04-16", got)
	}
	if got := evalString(t, ctx, conn, `document.body.dataset.events`); got != "d-top:input,d-top:change," {
		t.Errorf("events seen by the page = %q, want one input then one change", got)
	}

	// ClearAndFill takes the same path.
	if err := ClearAndFill(ctx, conn, byDOMID(root, "d-top"), "2028-01-02"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, conn, `document.getElementById("d-top").value`); got != "2028-01-02" {
		t.Errorf("date after ClearAndFill = %q, want 2028-01-02", got)
	}

	if err := Fill(ctx, conn, byDOMID(root, "time-top"), "14:45"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, conn, `document.getElementById("time-top").value`); got != "14:45" {
		t.Errorf("time = %q, want 14:45", got)
	}
	if err := Fill(ctx, conn, byDOMID(root, "range-top"), "7"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, conn, `document.getElementById("range-top").value`); got != "7" {
		t.Errorf("range = %q, want 7", got)
	}

	// A value the control cannot take is an error, not a silent empty field.
	err := Fill(ctx, conn, byDOMID(root, "d-top"), "next tuesday")
	if err == nil || !strings.Contains(err.Error(), "did not accept") {
		t.Errorf("fill of a malformed date: err = %v, want a did-not-accept error", err)
	}

	// A text input is still typed into.
	if err := Fill(ctx, conn, byDOMID(root, "t-top"), "hello"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, conn, `document.getElementById("t-top").value`); got != "hello" {
		t.Errorf("text input = %q, want hello", got)
	}

	// The frames, in their own documents.
	for _, c := range []struct{ domID, urlPart, value string }{
		{"d-same", "localhost", "2027-04-16"},
		{"d-cross", "127.0.0.1", "2029-12-31"},
	} {
		if err := Fill(ctx, conn, byDOMID(root, c.domID), c.value); err != nil {
			t.Fatalf("fill %s: %v", c.domID, err)
		}
		if got := frameString(t, ctx, conn, c.urlPart+":", fmt.Sprintf(`document.getElementById(%q).value`, c.domID)); got != c.value {
			t.Errorf("date %s in its frame = %q, want %s", c.domID, got, c.value)
		}
		want := c.domID + ":input," + c.domID + ":change,"
		if got := frameString(t, ctx, conn, c.urlPart+":", `document.body.dataset.events`); got != want {
			t.Errorf("events seen in the frame of %s = %q, want %q", c.domID, got, want)
		}
	}
}
