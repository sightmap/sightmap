package browser

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sightmap/sightmap/go/sightmap"
)

// dragRecorder fakes the page side of a drag: __smDragPrepare answers with
// prep, the target's bounds lookup answers with toBounds, and every mouse event
// is recorded in order.
type dragRecorder struct {
	mu     sync.Mutex
	mouse  []map[string]interface{}
	prep   string
	to     string
	script []string
}

func (d *dragRecorder) conn(t *testing.T) *CDPConn {
	return newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		d.mu.Lock()
		defer d.mu.Unlock()
		switch method {
		case "Input.dispatchMouseEvent":
			var ev map[string]interface{}
			json.Unmarshal(params, &ev)
			d.mouse = append(d.mouse, ev)
		case "Runtime.evaluate":
			var p struct {
				Expression string `json:"expression"`
			}
			json.Unmarshal(params, &p)
			call := p.Expression[strings.LastIndex(p.Expression, "\n")+1:]
			d.script = append(d.script, call)
			value := d.to
			if strings.HasPrefix(call, "__smDragPrepare") {
				value = d.prep
			}
			return json.RawMessage(`{"result":{"type":"object","value":` + value + `}}`)
		}
		return json.RawMessage(`{}`)
	})
}

const testDragPrep = `{"bounds":{"x":100,"y":100,"width":200,"height":80},"view":{"width":1280,"height":800},"x":200,"y":140}`

func TestDragWith_Delta(t *testing.T) {
	defer func(d time.Duration) { dragStepDelay = d }(dragStepDelay)
	dragStepDelay = 0
	rec := &dragRecorder{prep: testDragPrep}
	conn := rec.conn(t)

	src := &sightmap.ComponentNode{Id: "7"}
	if err := DragWith(context.Background(), conn, src, DragOptions{DeltaX: 100, DeltaY: 40}); err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(rec.script[0], `__smDragPrepare("[data-sightmap-id=\"7\"]", null, null)`) {
		t.Errorf("prepare call = %q", rec.script[0])
	}
	// move, press, 10 moves, release.
	if len(rec.mouse) != 13 {
		t.Fatalf("got %d mouse events, want 13", len(rec.mouse))
	}
	check := func(i int, typ string, x, y, buttons float64) {
		t.Helper()
		e := rec.mouse[i]
		if e["type"] != typ || e["x"] != x || e["y"] != y || e["buttons"] != buttons {
			t.Errorf("event %d = %v, want %s (%v,%v) buttons=%v", i, e, typ, x, y, buttons)
		}
	}
	check(0, "mouseMoved", 200, 140, 0)
	check(1, "mousePressed", 200, 140, 1)
	if rec.mouse[1]["button"] != "left" || rec.mouse[1]["clickCount"] != float64(1) {
		t.Errorf("press = %v", rec.mouse[1])
	}
	for i := 1; i <= 10; i++ {
		check(1+i, "mouseMoved", float64(200+10*i), float64(140+4*i), 1)
		if rec.mouse[1+i]["button"] != "left" {
			t.Errorf("move %d: button = %v, want left", i, rec.mouse[1+i]["button"])
		}
	}
	check(12, "mouseReleased", 300, 180, 0)
}

func TestDragWith_StepsAndOffset(t *testing.T) {
	defer func(d time.Duration) { dragStepDelay = d }(dragStepDelay)
	dragStepDelay = 0
	rec := &dragRecorder{prep: testDragPrep}
	conn := rec.conn(t)

	src := &sightmap.ComponentNode{Id: "7"}
	err := DragWith(context.Background(), conn, src, DragOptions{Steps: 3, FromOffset: &[2]int{12, 9}, DeltaX: 30})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.script[0], ", 12, 9)") {
		t.Errorf("offset not passed to the page: %q", rec.script[0])
	}
	if len(rec.mouse) != 3+3 {
		t.Fatalf("got %d mouse events, want 6", len(rec.mouse))
	}
	if x := rec.mouse[len(rec.mouse)-1]["x"]; x != float64(230) {
		t.Errorf("release x = %v, want 230", x)
	}
}

func TestDragWith_ToTarget(t *testing.T) {
	defer func(d time.Duration) { dragStepDelay = d }(dragStepDelay)
	dragStepDelay = 0
	// Target box: x 500..700, y 300..400.
	for _, tc := range []struct {
		pos  string
		x, y float64
	}{
		{"", 600, 350},
		{"center", 600, 350},
		{"above", 600, 304},
		{"below", 600, 396},
	} {
		t.Run("position="+tc.pos, func(t *testing.T) {
			rec := &dragRecorder{prep: testDragPrep, to: `{"x":500,"y":300,"width":200,"height":100}`}
			conn := rec.conn(t)
			err := DragWith(context.Background(), conn, &sightmap.ComponentNode{Id: "7"},
				DragOptions{To: &sightmap.ComponentNode{Id: "9"}, ToPosition: tc.pos})
			if err != nil {
				t.Fatal(err)
			}
			if last := rec.script[len(rec.script)-1]; !strings.Contains(last, `data-sightmap-id=\"9\"`) {
				t.Errorf("target not looked up: %q", last)
			}
			rel := rec.mouse[len(rec.mouse)-1]
			if rel["type"] != "mouseReleased" || rel["x"] != tc.x || rel["y"] != tc.y {
				t.Errorf("release = %v, want (%v,%v)", rel, tc.x, tc.y)
			}
		})
	}
}

func TestDragWith_Errors(t *testing.T) {
	defer func(d time.Duration) { dragStepDelay = d }(dragStepDelay)
	dragStepDelay = 0
	to := &sightmap.ComponentNode{Id: "9"}
	for name, tc := range map[string]struct {
		opts DragOptions
		prep string
		want string
	}{
		"no destination":  {DragOptions{}, testDragPrep, "nowhere to drag"},
		"both":            {DragOptions{To: to, DeltaX: 5}, testDragPrep, "not both"},
		"bad steps":       {DragOptions{DeltaX: 5, Steps: -1}, testDragPrep, "steps"},
		"bad position":    {DragOptions{To: to, ToPosition: "left"}, testDragPrep, "unknown drop position"},
		"source gone":     {DragOptions{DeltaX: 5}, `null`, "not found"},
		"drop off screen": {DragOptions{DeltaX: 5000}, testDragPrep, "outside the 1280x800 viewport"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := &dragRecorder{prep: tc.prep, to: `{"x":500,"y":300,"width":200,"height":100}`}
			conn := rec.conn(t)
			err := DragWith(context.Background(), conn, &sightmap.ComponentNode{Id: "7"}, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
			if len(rec.mouse) != 0 {
				t.Errorf("sent %d mouse events before failing", len(rec.mouse))
			}
		})
	}
}
