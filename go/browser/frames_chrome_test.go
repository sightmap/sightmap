package browser

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// The frame fixture is the shape of an MCP Apps host: a page with a same-origin
// iframe, and a cross-origin iframe (the same server under another host name, so
// Chrome gives it its own process) that holds a further, srcdoc iframe with the
// widget in it. Everything is absolutely positioned in whole pixels, so where each
// control must be reported is arithmetic:
//
//	#same     at (20,100), 2px border      → its content starts at (22,102)
//	  #same-btn at (5,7) in it, 100x30       → (27,109)
//	#cross    at (300,1200), no border     → below the 600px window
//	  #inner    at (50,80) in it             → (350,1280)
//	    #nested-btn at (10,20) in it, 100x30 → (360,1300)
//	    #nested-in  at (10,70) in it, 150x30 → (360,1350)
//
// The page is 2000px tall, so reaching the cross-origin controls takes a scroll.
const frameFixtureTop = `<!doctype html><html><body style="margin:0;height:2000px">
<button id="top-btn" style="position:absolute;left:0;top:0;width:100px;height:30px">Top</button>
<iframe id="same" src="/same" style="position:absolute;left:20px;top:100px;width:300px;height:120px;border:2px solid red;padding:0"></iframe>
<iframe id="cross" src="%s/cross" style="position:absolute;left:300px;top:1200px;width:400px;height:300px;border:0"></iframe>
</body></html>`

const frameFixtureSame = `<!doctype html><html><body style="margin:0">
<button id="same-btn" style="position:absolute;left:5px;top:7px;width:100px;height:30px" onclick="document.body.dataset.clicks=(+document.body.dataset.clicks||0)+1">Same</button>
</body></html>`

const frameFixtureCross = `<!doctype html><html><body style="margin:0">
<iframe id="inner" style="position:absolute;left:50px;top:80px;width:300px;height:150px;border:0" srcdoc="&lt;body style='margin:0'&gt;&lt;button id='nested-btn' style='position:absolute;left:10px;top:20px;width:100px;height:30px' onclick='document.body.dataset.clicks=(+document.body.dataset.clicks||0)+1'&gt;Nested&lt;/button&gt;&lt;input id='nested-in' style='box-sizing:border-box;position:absolute;left:10px;top:70px;width:150px;height:30px' oninput='document.body.dataset.value=this.value' onkeydown='document.body.dataset.key=event.key'&gt;&lt;/body&gt;"></iframe>
</body></html>`

// frameFixture serves the fixture and returns the top page's URL (host
// localhost) and the other origin's (127.0.0.1).
func frameFixture(t *testing.T) (top, cross string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	cross = fmt.Sprintf("http://127.0.0.1:%d", port)
	top = fmt.Sprintf("http://localhost:%d", port)
	mux := http.NewServeMux()
	page := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, body)
		}
	}
	mux.HandleFunc("/", page(fmt.Sprintf(frameFixtureTop, cross)))
	mux.HandleFunc("/same", page(frameFixtureSame))
	mux.HandleFunc("/cross", page(frameFixtureCross))
	srv := httptest.NewUnstartedServer(mux)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return top, cross
}

// loadFrameFixture opens the fixture in a real Chrome and waits for all three
// levels of frames to have loaded.
func loadFrameFixture(t *testing.T) (ctx context.Context, conn *CDPConn, topURL, crossURL string) {
	t.Helper()
	ctx, conn = fullPageChrome(t)
	topURL, crossURL = frameFixture(t)
	if err := NavigateAndWaitIdle(ctx, conn, topURL+"/", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	for {
		if ctx.Err() != nil {
			t.Fatal("the nested frame never loaded")
		}
		if len(ChildFrames(ctx, conn)) == 3 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return
}

// byDOMID finds the node whose element has the given id attribute.
func byDOMID(root *sightmap.ComponentNode, id string) *sightmap.ComponentNode {
	var found *sightmap.ComponentNode
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
		if n.Element != nil && n.Element.Attrs["id"] == id {
			found = n
		}
		return found == nil
	})
	if found == nil {
		panic("no node with id " + id)
	}
	return found
}

func extractFixture(t *testing.T, ctx context.Context, conn *CDPConn) (*sightmap.ComponentNode, []string) {
	t.Helper()
	page, _ := conn.DefaultPage()
	root, notes, err := ExtractComponentsNotes(ctx, page)
	if err != nil {
		t.Fatal(err)
	}
	return root, notes
}

// frameValue reads an expression in the (single) child frame whose URL has the
// given prefix.
func frameValue(t *testing.T, ctx context.Context, conn *CDPConn, urlPrefix, expr string) string {
	t.Helper()
	for _, f := range ChildFrames(ctx, conn) {
		if strings.HasPrefix(f.URL, urlPrefix) {
			raw, err := f.EvalJSON(ctx, expr)
			if err != nil {
				t.Fatal(err)
			}
			return strings.Trim(string(raw), `"`)
		}
	}
	t.Fatalf("no frame with URL %s*", urlPrefix)
	return ""
}

// TestFrameSnapshot_Chrome checks, against a real Chrome, that a snapshot holds
// the controls of the same-origin frame, of the cross-origin (out-of-process)
// frame and of the srcdoc frame nested in it; that each is where the page draws
// it; and that ids are unique and say which frame they are in.
//
// Opt-in: SIGHTMAP_CHROME_E2E=1 (uses FindChrome, or SIGHTMAP_CHROME).
func TestFrameSnapshot_Chrome(t *testing.T) {
	ctx, conn, topURL, crossURL := loadFrameFixture(t)
	root, notes := extractFixture(t, ctx, conn)
	if len(notes) != 0 {
		t.Errorf("notes = %v, want none", notes)
	}

	want := []struct {
		domID    string
		x, y     int
		w, h     int
		idPrefix string
	}{
		{"top-btn", 0, 0, 100, 30, ""},
		{"same-btn", 27, 109, 100, 30, "1_"},
		{"cross", 300, 1200, 400, 300, ""},
		{"nested-btn", 360, 1300, 100, 30, "3_"},
		{"nested-in", 360, 1350, 150, 30, "3_"},
	}
	seen := map[string]bool{}
	for _, w := range want {
		n := byDOMID(root, w.domID)
		if n.Bounds == nil || *n.Bounds != (sightmap.Bounds{X: w.x, Y: w.y, Width: w.w, Height: w.h}) {
			t.Errorf("%s bounds = %+v, want {%d %d %d %d} in top-page coordinates", w.domID, n.Bounds, w.x, w.y, w.w, w.h)
		}
		if !strings.HasPrefix(n.Id, w.idPrefix) || (w.idPrefix == "" && strings.Contains(n.Id, "_")) {
			t.Errorf("%s id = %q, want prefix %q", w.domID, n.Id, w.idPrefix)
		}
		if seen[n.Id] {
			t.Errorf("id %q is not unique", n.Id)
		}
		seen[n.Id] = true
	}

	// The controls are in the tree under the <iframe> that hosts them, through the
	// roots marked with the URL each document is matched at: a srcdoc document
	// takes its parent's URL.
	nested := byDOMID(root, "nested-btn")
	var chain []string
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
		found := false
		sightmap.Walk(n, func(m *sightmap.ComponentNode, _ int) bool { found = found || m == nested; return !found })
		if found && n.IsFrameRoot() {
			chain = append(chain, n.FrameURL)
		}
		return found
	})
	if len(chain) != 2 || chain[0] != crossURL+"/cross" || chain[1] != crossURL+"/cross" {
		t.Errorf("frame documents above the nested button = %v, want [%s/cross %s/cross]", chain, crossURL, crossURL)
	}
	if d := byDOMID(root, "same-btn"); d == nil {
		t.Fatal("same-origin button missing")
	}
	same := byDOMID(root, "same")
	if len(same.Children) != 1 || same.Children[0].FrameURL != topURL+"/same" {
		t.Errorf("same-origin iframe children = %+v, want one frame document at %s/same", same.Children, topURL)
	}

	// The nested button is below the fold of the top page: not in the viewport.
	if nested.InViewport {
		t.Error("nested button below the fold reports inViewport")
	}
	if !byDOMID(root, "same-btn").InViewport {
		t.Error("same-origin button on screen reports not inViewport")
	}
}

// TestFrameInteraction_Chrome clicks and types into the control of a frame two
// levels down in another process, below the fold, by the id a snapshot gave it,
// and checks the page received them. The click must scroll the page, so it also
// checks the point is worked out after the scroll rather than from the
// snapshot's bounds.
func TestFrameInteraction_Chrome(t *testing.T) {
	ctx, conn, _, _ := loadFrameFixture(t)
	root, _ := extractFixture(t, ctx, conn)
	btn, in := byDOMID(root, "nested-btn"), byDOMID(root, "nested-in")

	x, y, err := Click(ctx, conn, btn)
	if err != nil {
		t.Fatal(err)
	}
	if got := frameValue(t, ctx, conn, "about:srcdoc", `document.body.dataset.clicks||"0"`); got != "1" {
		t.Errorf("nested button clicks = %q, want 1 (clicked at %d,%d)", got, x, y)
	}
	if y < 0 || y >= 600 {
		t.Errorf("click at y=%d, want a point in the 600px viewport after scrolling", y)
	}

	// A second click, now that the page has scrolled and the bounds in btn are
	// stale, still lands.
	if _, _, err := Click(ctx, conn, btn); err != nil {
		t.Fatal(err)
	}
	if got := frameValue(t, ctx, conn, "about:srcdoc", `document.body.dataset.clicks||"0"`); got != "2" {
		t.Errorf("nested button clicks after a second click = %q, want 2", got)
	}

	if err := Fill(ctx, conn, in, "hello"); err != nil {
		t.Fatal(err)
	}
	if got := frameValue(t, ctx, conn, "about:srcdoc", `document.body.dataset.value||""`); got != "hello" {
		t.Errorf("nested input = %q, want hello", got)
	}
	if err := ClearAndFill(ctx, conn, in, "hi"); err != nil {
		t.Fatal(err)
	}
	if got := frameValue(t, ctx, conn, "about:srcdoc", `document.body.dataset.value||""`); got != "hi" {
		t.Errorf("nested input after ClearAndFill = %q, want hi", got)
	}

	// Focus by id, then a key: the key must reach the focused element of the
	// frame, whichever process it is in. (KeyPress sends no text, so it does not
	// edit; the frame's keydown listener is what shows the key arrived.)
	if err := Focus(ctx, conn, in); err != nil {
		t.Fatal(err)
	}
	if err := KeyPress(ctx, conn, "ArrowLeft"); err != nil {
		t.Fatal(err)
	}
	if got := frameValue(t, ctx, conn, "about:srcdoc", `document.body.dataset.key||""`); got != "ArrowLeft" {
		t.Errorf("key seen by the nested input after Focus+KeyPress = %q, want ArrowLeft", got)
	}

	// The same-origin frame, scrolled back to the top.
	if _, err := EvalJSON(ctx, conn, `scrollTo(0,0)`); err != nil {
		t.Fatal(err)
	}
	sameBtn := byDOMID(root, "same-btn")
	if _, _, err := Click(ctx, conn, sameBtn); err != nil {
		t.Fatal(err)
	}
	if got := frameValue(t, ctx, conn, topURLPrefix(ctx, conn)+"/same", `document.body.dataset.clicks||"0"`); got != "1" {
		t.Errorf("same-origin button clicks = %q, want 1", got)
	}

	// Resolved bounds are in top-page coordinates too.
	node, err := ResolveBySightmapID(ctx, conn, btn.Id)
	if err != nil {
		t.Fatal(err)
	}
	var scrollY float64
	if raw, err := EvalJSON(ctx, conn, `scrollY`); err == nil {
		fmt.Sscan(string(raw), &scrollY)
	}
	if node.Bounds.X != 360 || node.Bounds.Y != 1300-int(scrollY) {
		t.Errorf("resolved bounds = %+v at scrollY %v, want x=360 y=%d", node.Bounds, scrollY, 1300-int(scrollY))
	}

	// Waiting for a selector finds an element in a frame.
	wctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := WaitForSelector(wctx, conn, "#nested-btn"); err != nil {
		t.Errorf("WaitForSelector(#nested-btn): %v", err)
	}
}

func topURLPrefix(ctx context.Context, conn *CDPConn) string {
	url, _ := GetURL(ctx, conn)
	return strings.TrimSuffix(url, "/")
}

// TestFrameClick_CoveredErrors_Chrome checks that a click on a control in a
// frame refuses when something in the page above is drawn over the frame at that
// point, as it would for a control in the page itself: the hit test inside the
// frame cannot see an overlay in its parent.
func TestFrameClick_CoveredErrors_Chrome(t *testing.T) {
	ctx, conn, _, _ := loadFrameFixture(t)
	root, _ := extractFixture(t, ctx, conn)
	if _, err := EvalJSON(ctx, conn, `(() => { const d = document.createElement("div");
		d.style.cssText = "position:fixed;left:0;top:0;width:100%;height:100%;background:rgba(0,0,0,.2)";
		document.body.appendChild(d); return true; })()`); err != nil {
		t.Fatal(err)
	}
	_, _, err := Click(ctx, conn, byDOMID(root, "same-btn"))
	if err == nil || !strings.Contains(err.Error(), "covered") {
		t.Fatalf("click under an overlay: err = %v, want a covered error", err)
	}
	if got := frameValue(t, ctx, conn, topURLPrefix(ctx, conn)+"/same", `document.body.dataset.clicks||"0"`); got != "0" {
		t.Errorf("the covered button was clicked anyway (%s clicks)", got)
	}
}

// TestFrameOptions_Chrome checks the opt-out and the caps: frames off leaves the
// <iframe> a leaf; a node cap cuts a frame and says so.
func TestFrameOptions_Chrome(t *testing.T) {
	ctx, conn, _, _ := loadFrameFixture(t)

	conn.SetFrameOptions(FrameOptions{Disabled: true})
	root, notes := extractFixture(t, ctx, conn)
	if len(notes) != 0 {
		t.Errorf("notes with frames off = %v, want none", notes)
	}
	if len(byDOMID(root, "cross").Children) != 0 {
		t.Error("with frames off the <iframe> still has children")
	}

	conn.SetFrameOptions(FrameOptions{MaxNodes: 3})
	root, notes = extractFixture(t, ctx, conn)
	if joined := strings.Join(notes, "\n"); !strings.Contains(joined, "cut at 3 nodes") {
		t.Errorf("notes with a 3-node cap = %v, want a cut note", notes)
	}
	if len(byDOMID(root, "cross").Children) != 1 {
		t.Error("a cut frame should still be attached, short")
	}

	conn.SetFrameOptions(FrameOptions{MaxFrames: 1})
	_, notes = extractFixture(t, ctx, conn)
	if joined := strings.Join(notes, "\n"); !strings.Contains(joined, "more than 1 frames") {
		t.Errorf("notes with a 1-frame cap = %v, want a frame-cap note", notes)
	}
}

// TestFrameMatching_Chrome applies a corpus to a live snapshot: a view whose
// route is the cross-origin frame's path names the controls in it, though the
// page itself is another URL, and a rule of the page's view does not reach in.
func TestFrameMatching_Chrome(t *testing.T) {
	ctx, conn, topURL, _ := loadFrameFixture(t)
	root, _ := extractFixture(t, ctx, conn)

	dir := t.TempDir()
	corpus := `
version: 1
views:
  - name: Page
    route: /
    components:
      - name: PageButton
        selector: button
  - name: Widget
    route: /cross
    components:
      - name: NestedButton
        selector: '#nested-btn'
      - name: NestedInput
        selector: '#nested-in'
`
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(corpus), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	res := match.NewMatcher(c).Match(root, topURL+"/")
	name := func(domID string) string {
		if m := res[byDOMID(root, domID)]; m != nil {
			return m.Name
		}
		return ""
	}
	for domID, want := range map[string]string{
		"top-btn":    "PageButton",
		"same-btn":   "PageButton", // /same has no view of its own: only what is global (nothing here)
		"nested-btn": "NestedButton",
		"nested-in":  "NestedInput",
	} {
		if domID == "same-btn" {
			want = "" // the page's view does not reach into the same-origin frame
		}
		if got := name(domID); got != want {
			t.Errorf("%s matched as %q, want %q", domID, got, want)
		}
	}
}
