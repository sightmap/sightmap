package browser

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The drag fixture is a page with a sortable list in a same-origin iframe and
// another in a cross-origin one (the same server under another host name, so
// Chrome gives it its own process), the cross-origin one below the 600px window
// so a drag in it has to scroll first. A list is driven by pointer events, as
// sortable-list libraries are: a press does nothing until the pointer has moved
// 4px, and the drop is decided by what is under the pointer on release.
//
//	#same   at (20,100), 300x200
//	#cross  at (300,900), 300x200, in a 1500px page
//	items   A, B, C: 40px tall, 4px apart, from (10,10) in their frame, 200px wide
const dragFixtureTop = `<!doctype html><html><body style="margin:0;height:1500px">
<iframe id="same" src="/drag-same" style="position:absolute;left:20px;top:100px;width:300px;height:200px;border:0"></iframe>
<iframe id="cross" src="%s/drag-cross" style="position:absolute;left:300px;top:900px;width:300px;height:200px;border:0"></iframe>
</body></html>`

const dragFixtureList = `<!doctype html><html><body style="margin:0" data-order="A,B,C">
<div id="list" style="position:absolute;left:10px;top:10px;width:200px">
<button id="PFX-a" data-k="A" style="display:block;box-sizing:border-box;width:200px;height:40px;margin:0 0 4px">A</button>
<button id="PFX-b" data-k="B" style="display:block;box-sizing:border-box;width:200px;height:40px;margin:0 0 4px">B</button>
<button id="PFX-c" data-k="C" style="display:block;box-sizing:border-box;width:200px;height:40px;margin:0 0 4px">C</button>
</div>
<script>
const list = document.getElementById("list");
let drag = null;
list.addEventListener("pointerdown", (e) => {
  const it = e.target.closest("button");
  if (it) drag = { it, x: e.clientX, y: e.clientY, on: false };
});
addEventListener("pointermove", (e) => {
  if (!drag || drag.on) return;
  if (Math.hypot(e.clientX - drag.x, e.clientY - drag.y) >= 4) drag.on = true;
});
addEventListener("pointerup", (e) => {
  const d = drag;
  drag = null;
  if (!d || !d.on) return;
  const at = document.elementFromPoint(e.clientX, e.clientY);
  const t = at && at.closest("button");
  if (!t || t === d.it) return;
  const r = t.getBoundingClientRect();
  list.insertBefore(d.it, e.clientY < r.top + r.height / 2 ? t : t.nextSibling);
  document.body.dataset.order = [...list.children].map((b) => b.dataset.k).join();
});
</script></body></html>`

// loadDragFixture opens the drag fixture in a real Chrome and waits for both
// frames to have loaded. It returns the URL prefixes of the two lists' frames.
func loadDragFixture(t *testing.T) (ctx context.Context, conn *CDPConn, sameURL, crossURL string) {
	t.Helper()
	ctx, conn = fullPageChrome(t)
	topURL, crossOrigin := dragFixtureServer(t)
	if err := NavigateAndWaitIdle(ctx, conn, topURL+"/", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	for {
		if ctx.Err() != nil {
			t.Fatal("the frames never loaded")
		}
		if len(ChildFrames(ctx, conn)) == 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ctx, conn, topURL + "/drag-same", crossOrigin + "/drag-cross"
}

func dragFixtureServer(t *testing.T) (top, cross string) {
	t.Helper()
	srv := httptest.NewUnstartedServer(nil)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	cross = fmt.Sprintf("http://127.0.0.1:%d", port)
	top = fmt.Sprintf("http://localhost:%d", port)
	mux := http.NewServeMux()
	page := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, body)
		}
	}
	mux.HandleFunc("/", page(fmt.Sprintf(dragFixtureTop, cross)))
	mux.HandleFunc("/drag-same", page(strings.ReplaceAll(dragFixtureList, "PFX", "same")))
	mux.HandleFunc("/drag-cross", page(strings.ReplaceAll(dragFixtureList, "PFX", "cross")))
	srv.Config.Handler = mux
	srv.Start()
	t.Cleanup(srv.Close)
	return top, cross
}

// order reads the list order the frame whose URL starts with prefix reports.
func dragOrder(t *testing.T, ctx context.Context, conn *CDPConn, prefix string) string {
	t.Helper()
	return frameValue(t, ctx, conn, prefix, `document.body.dataset.order`)
}

// TestFrameDrag_Chrome drags the items of a sortable list inside a same-origin
// and a cross-origin iframe: by a drop on another item (at its bottom, then its
// top edge) and by a delta. The page is scrolled between the drags, so the
// bounds a snapshot recorded are stale; each drag must find the live geometry,
// in the frame, send the pointer to the right place in the page, and on the
// cross-origin frame's own session.
func TestFrameDrag_Chrome(t *testing.T) {
	defer func(d time.Duration) { dragStepDelay = d }(dragStepDelay)
	dragStepDelay = 4 * time.Millisecond

	ctx, conn, sameURL, crossURL := loadDragFixture(t)
	root, _ := extractFixture(t, ctx, conn)
	for _, tc := range []struct{ name, prefix, url string }{
		{"same-origin", "same", sameURL},
		{"cross-origin", "cross", crossURL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b, c := byDOMID(root, tc.prefix+"-a"), byDOMID(root, tc.prefix+"-b"), byDOMID(root, tc.prefix+"-c")
			if got := dragOrder(t, ctx, conn, tc.url); got != "A,B,C" {
				t.Fatalf("start order = %q", got)
			}
			// Start from the top of the page, so the cross-origin list is off
			// screen and the drag has to bring it into view.
			if _, err := EvalJSON(ctx, conn, `scrollTo(0,0)`); err != nil {
				t.Fatal(err)
			}

			if err := DragWith(ctx, conn, a, DragOptions{To: c, ToPosition: "below"}); err != nil {
				t.Fatalf("drag A below C: %v", err)
			}
			if got := dragOrder(t, ctx, conn, tc.url); got != "B,C,A" {
				t.Errorf("order after dragging A below C = %q, want B,C,A", got)
			}

			if err := DragWith(ctx, conn, a, DragOptions{To: b, ToPosition: "above"}); err != nil {
				t.Fatalf("drag A above B: %v", err)
			}
			if got := dragOrder(t, ctx, conn, tc.url); got != "A,B,C" {
				t.Errorf("order after dragging A above B = %q, want A,B,C", got)
			}

			// By a delta: one item and a half down is over the next item's
			// lower half, so the dragged one lands after it.
			if err := DragWith(ctx, conn, a, DragOptions{DeltaY: 56}); err != nil {
				t.Fatalf("drag A by a delta: %v", err)
			}
			if got := dragOrder(t, ctx, conn, tc.url); got != "B,A,C" {
				t.Errorf("order after dragging A down by 56px = %q, want B,A,C", got)
			}
		})
	}
}

// A drop target that something in the page above is drawn over is refused, the
// check running in the page that holds the frame, as Click's does.
func TestFrameDrag_CoveredTargetErrors_Chrome(t *testing.T) {
	defer func(d time.Duration) { dragStepDelay = d }(dragStepDelay)
	dragStepDelay = 0
	ctx, conn, sameURL, _ := loadDragFixture(t)
	root, _ := extractFixture(t, ctx, conn)
	a, c := byDOMID(root, "same-a"), byDOMID(root, "same-c")
	// A bar over C (page y 198..238) but not A (110..150).
	if _, err := EvalJSON(ctx, conn, `(() => { const d = document.createElement("div");
		d.style.cssText = "position:fixed;left:0;top:190px;width:100%;height:80px;background:rgba(0,0,0,.2)";
		document.body.appendChild(d); return true; })()`); err != nil {
		t.Fatal(err)
	}
	err := DragWith(ctx, conn, a, DragOptions{To: c, ToPosition: "above"})
	if err == nil || !strings.Contains(err.Error(), "not over the target") {
		t.Fatalf("drop onto a covered target: err = %v, want a not-over-the-target error", err)
	}
	if got := dragOrder(t, ctx, conn, sameURL); got != "A,B,C" {
		t.Errorf("order changed to %q although the drag was refused", got)
	}
}

// A drag cannot carry the pointer from one process to another: it is refused
// before the button goes down, saying why.
func TestFrameDrag_AcrossProcessesErrors_Chrome(t *testing.T) {
	defer func(d time.Duration) { dragStepDelay = d }(dragStepDelay)
	dragStepDelay = 0
	ctx, conn, sameURL, _ := loadDragFixture(t)
	root, _ := extractFixture(t, ctx, conn)
	err := DragWith(ctx, conn, byDOMID(root, "same-a"), DragOptions{To: byDOMID(root, "cross-b")})
	if err == nil || !strings.Contains(err.Error(), "different processes") {
		t.Fatalf("drag across processes: err = %v, want a different-processes error", err)
	}
	if got := dragOrder(t, ctx, conn, sameURL); got != "A,B,C" {
		t.Errorf("order changed to %q although the drag was refused", got)
	}
}
