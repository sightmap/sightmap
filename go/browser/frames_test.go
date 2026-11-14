package browser

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sightmap/sightmap/go/extract"
	"github.com/sightmap/sightmap/go/sightmap"
)

func TestIDFramePrefix(t *testing.T) {
	for id, want := range map[string]int{"5": 0, "": 0, "1_5": 1, "12_3": 12, "x_1": 0, "_4": 0} {
		if got := idFramePrefix(id); got != want {
			t.Errorf("idFramePrefix(%q) = %d, want %d", id, got, want)
		}
	}
}

// stitchFrame rebases a frame's bounds onto the top page and clips inViewport to
// what the top viewport shows of the frame.
func TestStitchFrame(t *testing.T) {
	owner := &sightmap.ComponentNode{Id: "11"}
	inside := &sightmap.ComponentNode{Id: "1_3", InViewport: true, Bounds: &sightmap.Bounds{X: 10, Y: 20, Width: 100, Height: 30}}
	clipped := &sightmap.ComponentNode{Id: "1_4", InViewport: true, Bounds: &sightmap.Bounds{X: 10, Y: 500, Width: 100, Height: 30}}
	off := &sightmap.ComponentNode{Id: "1_5", InViewport: false, Bounds: &sightmap.Bounds{X: 0, Y: 0, Width: 5, Height: 5}}
	froot := &sightmap.ComponentNode{Id: "1_1", Children: []*sightmap.ComponentNode{inside, clipped, off}}

	// The frame starts at (22.4, 102.6) on the page; the page shows y < 600 of
	// it, and the frame's own viewport is taller.
	stitchFrame(owner, froot, "http://x/y", 22.4, 102.6, rect{0, 0, 800, 600})

	if len(owner.Children) != 1 || owner.Children[0] != froot || froot.FrameURL != "http://x/y" || froot.NthChild != 1 {
		t.Fatalf("frame not attached under its owner: %+v", owner.Children)
	}
	if *inside.Bounds != (sightmap.Bounds{X: 32, Y: 123, Width: 100, Height: 30}) {
		t.Errorf("inside bounds = %+v, want origin added and rounded: {32 123 100 30}", *inside.Bounds)
	}
	if !inside.InViewport {
		t.Error("a control visible through the frame must stay inViewport")
	}
	if clipped.InViewport {
		t.Errorf("a control at y=%d is below the 600px top viewport but reports inViewport", clipped.Bounds.Y)
	}
	if off.InViewport {
		t.Error("a control that was out of its frame's viewport became inViewport")
	}
}

func TestIndexIframesOnlyTakesIframeElements(t *testing.T) {
	doc := &extract.DOMNode{NodeName: "#document", FrameID: "top", Children: []*extract.DOMNode{{
		NodeName: "HTML", FrameID: "top", Children: []*extract.DOMNode{
			{NodeName: "IFRAME", FrameID: "child", ContentDocument: &extract.DOMNode{NodeName: "#document", FrameID: "child",
				Children: []*extract.DOMNode{{NodeName: "IFRAME", FrameID: "grandchild"}}}},
		},
	}}}
	got := indexIframes(doc)
	if len(got) != 2 || got["child"] == nil || got["grandchild"] == nil {
		t.Fatalf("indexIframes = %v, want the child and grandchild <iframe> elements only", got)
	}
	if got["top"] != nil {
		t.Error("the top document carries the top frame's id but is not an owner")
	}
}

func TestTargetBookkeeping(t *testing.T) {
	c := &CDPConn{}
	attach := func(session, target, parent, typ string) {
		p, _ := json.Marshal(map[string]interface{}{"sessionId": session, "targetInfo": map[string]string{"targetId": target, "type": typ, "parentFrameId": parent}})
		c.noteTargetEvent("", "Target.attachedToTarget", p)
	}
	attach("s1", "t1", "top", "iframe")
	attach("s2", "t2", "t1", "iframe")
	attach("s3", "t3", "top", "service_worker") // not a frame
	if got := c.attachedTargetsFor("top"); len(got) != 1 || got[0].sessionID != "s1" {
		t.Fatalf("targets under top = %+v, want s1 only", got)
	}
	if got := c.attachedTargetsFor("t1"); len(got) != 1 || got[0].sessionID != "s2" {
		t.Fatalf("targets under t1 = %+v, want s2", got)
	}
	p, _ := json.Marshal(map[string]string{"sessionId": "s1"})
	c.noteTargetEvent("", "Target.detachedFromTarget", p)
	if got := c.attachedTargetsFor("top"); len(got) != 0 {
		t.Errorf("targets under top after detach = %+v, want none", got)
	}
}

// A command for a child session carries its sessionId; one for the connection's
// own target carries none.
func TestCallSessionAddressesTheSession(t *testing.T) {
	var sessions []string
	conn := newFakeCDPSessions(t, func(session, method string, _ json.RawMessage) json.RawMessage {
		sessions = append(sessions, session+"|"+method)
		return json.RawMessage(`{}`)
	})
	if _, err := conn.call(context.Background(), "A.one", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.callSession(context.Background(), "S9", "B.two", nil); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0] != "|A.one" || sessions[1] != "S9|B.two" {
		t.Errorf("sessions seen = %v, want [|A.one S9|B.two]", sessions)
	}
}

func TestFrameOptionsDefaults(t *testing.T) {
	c := &CDPConn{}
	o := c.frameOptions()
	if o.Disabled || o.MaxNodes != DefaultFrameMaxNodes || o.MaxFrames != DefaultMaxFrames {
		t.Errorf("default options = %+v", o)
	}
	c.SetFrameOptions(FrameOptions{MaxNodes: 7})
	if o := c.frameOptions(); o.MaxNodes != 7 || o.MaxFrames != DefaultMaxFrames {
		t.Errorf("options after set = %+v, want MaxNodes 7 and the default frame cap", o)
	}
}
