package browser

// Child frames. A page's iframes are other documents, some in other processes
// (cross-origin frames are "out-of-process iframes", OOPIFs, each its own CDP
// target). This file finds them and gives each a handle that can evaluate script
// in its document and say where that document sits on the top page; the rest of
// the package builds snapshots (frames_extract.go) and interactions
// (frames_actions.go) on it.
//
// A frame is reached one of two ways. A same-process child is a frame of its
// parent's own CDP target: Page.getFrameTree lists it, and a script runs in it
// through an isolated world made for it. A cross-process child is a separate
// target; Target.setAutoAttach with flatten attaches it as a session of the same
// websocket, addressed by the sessionId on each command. Nothing here opens
// another connection.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// FrameOptions says how page extractions treat child frames.
type FrameOptions struct {
	// Disabled leaves child frames out: an <iframe> is a leaf, as before.
	Disabled bool
	// MaxNodes caps the nodes (elements and text) taken from each child frame;
	// past it the frame's tree is cut and the extraction says so. 0 is
	// DefaultFrameMaxNodes. The top document is never capped.
	MaxNodes int
	// MaxFrames caps the child frames taken from a page, in document order. 0 is
	// DefaultMaxFrames.
	MaxFrames int
}

const (
	// DefaultFrameMaxNodes is a size an application frame, such as a widget or a
	// dashboard, fits in many times over; an ad or an embedded document that does
	// not is cut rather than allowed to swamp the snapshot.
	DefaultFrameMaxNodes = 3000
	// DefaultMaxFrames bounds a page full of ad and tracker frames.
	DefaultMaxFrames = 32
	// maxActionFrames bounds the frames an interaction searches for its target.
	maxActionFrames = 128
)

// SetFrameOptions sets how this connection's page extractions treat child
// frames. The zero value, which is the default, descends with the defaults.
func (c *CDPConn) SetFrameOptions(o FrameOptions) {
	c.targetsMu.Lock()
	c.frameOpts = &o
	c.targetsMu.Unlock()
}

func (c *CDPConn) frameOptions() FrameOptions {
	c.targetsMu.Lock()
	defer c.targetsMu.Unlock()
	var o FrameOptions
	if c.frameOpts != nil {
		o = *c.frameOpts
	}
	if o.MaxNodes <= 0 {
		o.MaxNodes = DefaultFrameMaxNodes
	}
	if o.MaxFrames <= 0 {
		o.MaxFrames = DefaultMaxFrames
	}
	return o
}

// attachedTarget is an iframe target Chrome attached to this connection as a
// flattened session.
type attachedTarget struct {
	sessionID     string
	targetID      string
	parentFrameID string
}

// noteTargetEvent records Target.attachedToTarget and detachedFromTarget. It
// runs on the read loop, so it only touches the bookkeeping.
func (c *CDPConn) noteTargetEvent(session, method string, params json.RawMessage) {
	switch method {
	case "Target.attachedToTarget":
		var ev struct {
			SessionID  string `json:"sessionId"`
			TargetInfo struct {
				TargetID      string `json:"targetId"`
				Type          string `json:"type"`
				ParentFrameID string `json:"parentFrameId"`
			} `json:"targetInfo"`
		}
		if json.Unmarshal(params, &ev) != nil || ev.TargetInfo.Type != "iframe" {
			return
		}
		c.targetsMu.Lock()
		c.targets = append(c.targets, &attachedTarget{
			sessionID:     ev.SessionID,
			targetID:      ev.TargetInfo.TargetID,
			parentFrameID: ev.TargetInfo.ParentFrameID,
		})
		c.targetsMu.Unlock()
	case "Target.detachedFromTarget":
		var ev struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(params, &ev) != nil {
			return
		}
		c.targetsMu.Lock()
		kept := c.targets[:0]
		for _, t := range c.targets {
			if t.sessionID != ev.SessionID {
				kept = append(kept, t)
			}
		}
		c.targets = kept
		delete(c.autoAttached, ev.SessionID)
		c.targetsMu.Unlock()
	}
}

// autoAttachFrames has Chrome attach every iframe target of the page, and of
// each iframe target in turn (a cross-origin frame can hold another), as a
// flattened session. Chrome reports the targets that already exist before it
// answers, and any that appear later as they do, so the bookkeeping is current
// when it returns. Each session is switched on once per connection.
func (c *CDPConn) autoAttachFrames(ctx context.Context) error {
	for {
		var pending []string
		c.targetsMu.Lock()
		if c.autoAttached == nil {
			c.autoAttached = map[string]bool{}
		}
		if !c.autoAttached[""] {
			pending = append(pending, "")
		}
		for _, t := range c.targets {
			if !c.autoAttached[t.sessionID] {
				pending = append(pending, t.sessionID)
			}
		}
		for _, s := range pending {
			c.autoAttached[s] = true
		}
		c.targetsMu.Unlock()
		if len(pending) == 0 {
			return nil
		}
		for _, s := range pending {
			if _, err := c.callSession(ctx, s, "Target.setAutoAttach", map[string]interface{}{
				"autoAttach":             true,
				"waitForDebuggerOnStart": false,
				"flatten":                true,
				"filter":                 []map[string]interface{}{{"type": "iframe"}},
			}); err != nil && s == "" {
				return err
			}
		}
	}
}

func (c *CDPConn) attachedTargetsFor(frameID string) []*attachedTarget {
	c.targetsMu.Lock()
	defer c.targetsMu.Unlock()
	var out []*attachedTarget
	for _, t := range c.targets {
		if t.parentFrameID == frameID {
			out = append(out, t)
		}
	}
	return out
}

// Frame is one document of a page: the top document, or a child frame at any
// depth. Its methods address the document it names, in whichever process
// it lives.
type Frame struct {
	conn *CDPConn
	// session is the CDP session whose target holds this frame ("" for the
	// connection's own target). A cross-origin child is the root frame of its
	// own session; a same-process child shares its parent's.
	session string
	// ID is the CDP frame id.
	ID string
	// URL is the document's URL as the frame tree reports it.
	URL      string
	Parent   *Frame
	Children []*Frame

	world int64 // isolated-world execution context, made on first use
}

// ownRoot reports whether f is the root frame of its CDP session, and so has
// the session's default execution context and its own DOM document.
func (f *Frame) ownRoot() bool { return f.Parent == nil || f.Parent.session != f.session }

// descendants lists f's children, depth-first in discovery order.
func (f *Frame) descendants() []*Frame {
	var out []*Frame
	for _, c := range f.Children {
		out = append(out, c)
		out = append(out, c.descendants()...)
	}
	return out
}

type frameTreeNode struct {
	Frame struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"frame"`
	ChildFrames []frameTreeNode `json:"childFrames"`
}

// discoverFrames returns the page's frame tree rooted at the top frame, or nil
// when the target reports none. Frames beyond maxFrames (the top frame counts)
// are left out, and cut says some were; a session that has gone away is skipped.
func (c *CDPConn) discoverFrames(ctx context.Context, maxFrames int) (top *Frame, cut bool, err error) {
	// A target that cannot auto-attach still lists its same-process frames below.
	_ = c.autoAttachFrames(ctx)
	count := 0
	var build func(session string, parent *Frame) (*Frame, error)
	build = func(session string, parent *Frame) (*Frame, error) {
		raw, err := c.callSession(ctx, session, "Page.getFrameTree", map[string]interface{}{})
		if err != nil {
			return nil, err
		}
		var resp struct {
			FrameTree frameTreeNode `json:"frameTree"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, err
		}
		if resp.FrameTree.Frame.ID == "" {
			return nil, nil
		}
		var walk func(n frameTreeNode, parent *Frame) *Frame
		walk = func(n frameTreeNode, parent *Frame) *Frame {
			f := &Frame{conn: c, session: session, ID: n.Frame.ID, URL: n.Frame.URL, Parent: parent}
			count++
			for _, cn := range n.ChildFrames {
				if count >= maxFrames {
					cut = true
					break
				}
				f.Children = append(f.Children, walk(cn, f))
			}
			for _, t := range c.attachedTargetsFor(f.ID) {
				if count >= maxFrames {
					cut = true
					break
				}
				if sub, err := build(t.sessionID, f); err == nil && sub != nil {
					f.Children = append(f.Children, sub)
				}
			}
			return f
		}
		return walk(resp.FrameTree, parent), nil
	}
	top, err = build("", nil)
	return top, cut, err
}

// ChildFrames lists the frames nested in the page, outermost first, or none
// when the page has no iframes. It is how an interaction looks for an element
// that is not in the top document.
func ChildFrames(ctx context.Context, conn *CDPConn) []*Frame {
	top, _, err := conn.discoverFrames(ctx, maxActionFrames)
	if err != nil || top == nil {
		return nil
	}
	return top.descendants()
}

// evalContext is the execution context a script for f runs in: 0, the session's
// default, for a session's root frame; an isolated world of the frame
// otherwise. An isolated world shares the frame's DOM, so the ids a probe
// stamped are visible to it and to the page, and page script cannot reach it.
func (f *Frame) evalContext(ctx context.Context) (int64, error) {
	if f.ownRoot() {
		return 0, nil
	}
	if f.world != 0 {
		return f.world, nil
	}
	raw, err := f.conn.callSession(ctx, f.session, "Page.createIsolatedWorld", map[string]interface{}{
		"frameId":   f.ID,
		"worldName": "sightmap",
	})
	if err != nil {
		return 0, err
	}
	var resp struct {
		ExecutionContextID int64 `json:"executionContextId"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, err
	}
	f.world = resp.ExecutionContextID
	return f.world, nil
}

// EvalJSON is the package's EvalJSON run in f's document.
func (f *Frame) EvalJSON(ctx context.Context, script string) (json.RawMessage, error) {
	contextID, err := f.evalContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("frame %s: %w", f.ID, err)
	}
	return evalJSONIn(ctx, f.conn, f.session, contextID, script)
}

// ownerBox is the content box of the <iframe> element that hosts f, in its
// parent document's viewport coordinates: the border box less border and
// padding, which is where f's own viewport starts.
func (f *Frame) ownerBox(ctx context.Context) (x, y float64, err error) {
	backendID, err := f.ownerBackendID(ctx)
	if err != nil {
		return 0, 0, err
	}
	raw, err := f.conn.callSession(ctx, f.Parent.session, "DOM.getBoxModel", map[string]interface{}{
		"backendNodeId": backendID,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("frame %s: owner box: %w", f.ID, err)
	}
	var resp struct {
		Model struct {
			Content []float64 `json:"content"`
		} `json:"model"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Model.Content) < 2 {
		return 0, 0, fmt.Errorf("frame %s: owner box: unreadable", f.ID)
	}
	return resp.Model.Content[0], resp.Model.Content[1], nil
}

func (f *Frame) ownerBackendID(ctx context.Context) (int64, error) {
	raw, err := f.conn.callSession(ctx, f.Parent.session, "DOM.getFrameOwner", map[string]interface{}{"frameId": f.ID})
	if err != nil {
		return 0, fmt.Errorf("frame %s: owner: %w", f.ID, err)
	}
	var resp struct {
		BackendNodeID int64 `json:"backendNodeId"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || resp.BackendNodeID == 0 {
		return 0, fmt.Errorf("frame %s: owner: unreadable", f.ID)
	}
	return resp.BackendNodeID, nil
}

// Origin is where f's document starts on the top page, in top-page viewport
// coordinates: a point (x, y) in f's viewport is at (ox+x, oy+y) there. It is
// read from the live layout every time, so it is right at the moment of the
// call whatever scrolled since the last one; the cost is a round trip per level
// of nesting.
func (f *Frame) Origin(ctx context.Context) (ox, oy float64, err error) {
	for g := f; g.Parent != nil; g = g.Parent {
		x, y, err := g.ownerBox(ctx)
		if err != nil {
			return 0, 0, err
		}
		ox, oy = ox+x, oy+y
	}
	return ox, oy, nil
}

// ownersShow reports whether the point (x, y), in top-page viewport
// coordinates, falls on f's own <iframe> element and on each enclosing one in
// turn, rather than on something drawn over it in the document that hosts it
// (an overlay, a modal, a sticky header). A hit test inside f cannot see that.
func (f *Frame) ownersShow(ctx context.Context, x, y float64) (bool, error) {
	for g := f; g.Parent != nil; g = g.Parent {
		px, py, err := g.Parent.Origin(ctx)
		if err != nil {
			return false, err
		}
		backendID, err := g.ownerBackendID(ctx)
		if err != nil {
			return false, err
		}
		obj, err := g.conn.callSession(ctx, g.Parent.session, "DOM.resolveNode", map[string]interface{}{"backendNodeId": backendID})
		if err != nil {
			return false, err
		}
		var node struct {
			Object struct {
				ObjectID string `json:"objectId"`
			} `json:"object"`
		}
		if err := json.Unmarshal(obj, &node); err != nil || node.Object.ObjectID == "" {
			return false, fmt.Errorf("frame %s: owner: unresolvable", g.ID)
		}
		raw, err := g.conn.callSession(ctx, g.Parent.session, "Runtime.callFunctionOn", map[string]interface{}{
			"objectId": node.Object.ObjectID,
			"functionDeclaration": `function(x, y) {
				const at = this.ownerDocument.elementFromPoint(x, y);
				return !!at && (at === this || this.contains(at));
			}`,
			"arguments":     []map[string]interface{}{{"value": x - px}, {"value": y - py}},
			"returnByValue": true,
		})
		if err != nil {
			return false, err
		}
		var res struct {
			Result struct {
				Value bool `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &res); err != nil || !res.Result.Value {
			return false, nil
		}
	}
	return true, nil
}

// isBlankURL reports whether a frame's document has no URL of its own: it takes
// its parent's, as a srcdoc document's base URL does.
func isBlankURL(u string) bool {
	return u == "" || strings.HasPrefix(u, "about:")
}
