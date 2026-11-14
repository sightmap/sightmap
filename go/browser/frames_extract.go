package browser

// Snapshotting child frames: probe each frame's document, rebase its bounds onto
// the top page, and hang its tree under the <iframe> element that hosts it.
//
// It runs in two phases around the top document's own extraction, because the
// top document's DOM tree (fetched after every probe has stamped its ids) is
// what shows the ids of same-process frames:
//
//  1. probeFrames, before the top DOM is read: find the frames, and run
//     probe.js in each with an id prefix "N_" (N = its place in the discovery
//     order) so ids are unique across the page. The ids stay stamped on the DOM,
//     which is how an interaction later finds the frame holding an id.
//  2. attachFrames, after: build each frame's tree from its probe, its
//     accessibility tree and its DOM, and attach it.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/sightmap/sightmap/go/extract"
	"github.com/sightmap/sightmap/go/sightmap"
)

// framedPage is a Page that can also extract the frames nested in it. The
// CDP adapter is one; another adapter that is not simply gets a page without
// frames.
type framedPage interface {
	probeFrames(ctx context.Context, top *extract.ProbeResult) (*frameSet, error)
	attachFrames(ctx context.Context, fs *frameSet, topDOM *extract.DOMNode, root *sightmap.ComponentNode)
}

// rect is a box in top-page viewport coordinates.
type rect struct{ x0, y0, x1, y1 float64 }

func (r rect) intersect(o rect) rect {
	return rect{math.Max(r.x0, o.x0), math.Max(r.y0, o.y0), math.Min(r.x1, o.x1), math.Min(r.y1, o.y1)}
}

func (r rect) contains(x, y float64) bool { return x >= r.x0 && x < r.x1 && y >= r.y0 && y < r.y1 }

// frameProbe is one frame's probe, with where the frame sits on the top page.
type frameProbe struct {
	frame *Frame
	res   *extract.ProbeResult
	// ox, oy is the frame's origin on the top page; visible the part of it a
	// reader of the top viewport can see (clipped by every enclosing frame).
	ox, oy  float64
	visible rect
}

// frameSet is what probeFrames found, handed to attachFrames, and the notes
// for what it left out.
type frameSet struct {
	probes []*frameProbe // parents before children
	notes  []string
	opts   FrameOptions
}

func (p *cdpPage) probeFrames(ctx context.Context, topProbe *extract.ProbeResult) (*frameSet, error) {
	opts := p.conn.frameOptions()
	if opts.Disabled {
		return nil, nil
	}
	// The top frame counts toward the cap.
	top, cut, err := p.conn.discoverFrames(ctx, opts.MaxFrames+1)
	if err != nil || top == nil || len(top.Children) == 0 {
		return nil, err
	}
	fs := &frameSet{opts: opts}
	if cut {
		fs.notes = append(fs.notes, fmt.Sprintf("more than %d frames on the page: the rest are left out (raise --max-frames, or --no-frames to leave frames out)", opts.MaxFrames))
	}

	visible := map[*Frame]rect{top: {0, 0, float64(topProbe.Viewport.Width), float64(topProbe.Viewport.Height)}}
	skipped := map[*Frame]bool{}
	for i, f := range top.descendants() {
		if skipped[f.Parent] {
			skipped[f] = true
			continue
		}
		// A frame with no box (its <iframe> is display:none, say) shows nothing and
		// is left out without comment: hidden tracker frames are everywhere.
		ox, oy, err := f.Origin(ctx)
		if err != nil {
			skipped[f] = true
			continue
		}
		prefix := fmt.Sprintf("%d_", i+1)
		raw, err := f.EvalJSON(ctx, probeScript(false, false, prefix, opts.MaxNodes))
		if err != nil {
			skipped[f] = true
			fs.notes = append(fs.notes, fmt.Sprintf("frame %s could not be read: %v", f.URL, err))
			continue
		}
		res, err := parseProbe(raw)
		if err != nil || !res.Success {
			skipped[f] = true
			fs.notes = append(fs.notes, fmt.Sprintf("frame %s could not be read: probe failed", f.URL))
			continue
		}
		if res.Viewport.Width == 0 || res.Viewport.Height == 0 {
			skipped[f] = true // a 0x0 frame: a tracking pixel
			continue
		}
		vis := visible[f.Parent].intersect(rect{ox, oy, ox + float64(res.Viewport.Width), oy + float64(res.Viewport.Height)})
		visible[f] = vis
		if res.Truncated {
			fs.notes = append(fs.notes, fmt.Sprintf("frame %s cut at %d nodes (raise --frame-max-nodes, or --no-frames to leave frames out)", f.URL, opts.MaxNodes))
		}
		fs.probes = append(fs.probes, &frameProbe{frame: f, res: res, ox: ox, oy: oy, visible: vis})
	}
	return fs, nil
}

func (p *cdpPage) attachFrames(ctx context.Context, fs *frameSet, topDOM *extract.DOMNode, root *sightmap.ComponentNode) {
	if fs == nil || len(fs.probes) == 0 {
		return
	}
	byID := map[string]*sightmap.ComponentNode{}
	indexIDs(root, byID)

	// A session's DOM, indexed by the frame each <iframe> element hosts. The top
	// session's was read by the caller, after the probes.
	type sessionDOM struct {
		root    *extract.DOMNode
		iframes map[string]*extract.DOMNode
	}
	sessions := map[string]*sessionDOM{"": {topDOM, indexIframes(topDOM)}}
	domOf := func(session string) *sessionDOM {
		if s, ok := sessions[session]; ok {
			return s
		}
		var s *sessionDOM
		if raw, err := p.conn.callSession(ctx, session, "DOM.getDocument", map[string]interface{}{"depth": -1, "pierce": true}); err == nil {
			var resp struct {
				Root *extract.DOMNode `json:"root"`
			}
			if json.Unmarshal(raw, &resp) == nil && resp.Root != nil {
				s = &sessionDOM{resp.Root, indexIframes(resp.Root)}
			}
		}
		sessions[session] = s
		return s
	}

	// effective is the URL each attached frame is matched at.
	effective := map[*Frame]string{}
	for _, fp := range fs.probes {
		f := fp.frame
		skip := func(why string) {
			fs.notes = append(fs.notes, fmt.Sprintf("frame %s left out: %s", f.URL, why))
		}

		parentDOM := domOf(f.Parent.session)
		if parentDOM == nil {
			skip("its parent's DOM could not be read")
			continue
		}
		owner := parentDOM.iframes[f.ID]
		var ownerNode *sightmap.ComponentNode
		if owner != nil {
			ownerNode = byID[domAttrOf(owner, "data-sightmap-id")]
		}
		if ownerNode == nil {
			skip("its <iframe> is not in the snapshot")
			continue
		}

		var domRoot *extract.DOMNode
		if f.ownRoot() {
			if d := domOf(f.session); d != nil {
				domRoot = d.root
			}
		} else if own := domOf(f.session); own != nil && own.iframes[f.ID] != nil {
			domRoot = own.iframes[f.ID].ContentDocument
		}
		if domRoot == nil {
			skip("its DOM could not be read")
			continue
		}

		// A session's root frame is the whole of its accessibility tree; any other
		// frame of the session is asked for by id.
		axParams := map[string]interface{}{}
		if !f.ownRoot() {
			axParams["frameId"] = f.ID
		}
		var a11y []extract.A11YNode
		if raw, err := p.conn.callSession(ctx, f.session, "Accessibility.getFullAXTree", axParams); err == nil {
			var resp struct {
				Nodes []extract.A11YNode `json:"nodes"`
			}
			if json.Unmarshal(raw, &resp) == nil {
				a11y = resp.Nodes
			}
		}

		froot, err := extract.BuildTree(fp.res, a11y, domRoot)
		if err != nil {
			skip(err.Error())
			continue
		}

		url := f.URL
		if isBlankURL(url) {
			url = effective[f.Parent]
		}
		effective[f] = url
		stitchFrame(ownerNode, froot, url, fp.ox, fp.oy, fp.visible)
		indexIDs(froot, byID)
	}
}

// stitchFrame hangs a frame's tree under the node of its <iframe> element: it
// marks the root with the URL the frame is matched at and rebases the tree's
// bounds from the frame's viewport onto the top page's, (ox, oy) being the
// frame's origin there. A node is in the viewport only if it is in the frame's
// own and in the part of the page visible through every enclosing frame
// (visible). The tree must not yet hold any frame of its own.
func stitchFrame(owner, froot *sightmap.ComponentNode, url string, ox, oy float64, visible rect) {
	dx, dy := int(math.Round(ox)), int(math.Round(oy))
	sightmap.Walk(froot, func(n *sightmap.ComponentNode, _ int) bool {
		if n.Bounds != nil {
			n.Bounds.X += dx
			n.Bounds.Y += dy
			if n.InViewport {
				n.InViewport = visible.contains(float64(n.Bounds.X)+float64(n.Bounds.Width)/2, float64(n.Bounds.Y)+float64(n.Bounds.Height)/2)
			}
		}
		return true
	})
	froot.FrameURL = url
	owner.Children = append(owner.Children, froot)
	froot.NthChild = len(owner.Children)
}

func indexIDs(root *sightmap.ComponentNode, byID map[string]*sightmap.ComponentNode) {
	sightmap.Walk(root, func(n *sightmap.ComponentNode, _ int) bool {
		byID[n.Id] = n
		return true
	})
}

// indexIframes maps a frame id to the <iframe>/<frame> element hosting it,
// looking through shadow roots and into the documents of same-process frames.
func indexIframes(root *extract.DOMNode) map[string]*extract.DOMNode {
	out := map[string]*extract.DOMNode{}
	var walk func(n *extract.DOMNode)
	walk = func(n *extract.DOMNode) {
		if n == nil {
			return
		}
		// Chrome gives the frame id to the document and its root element too,
		// which are not the owner: only the <iframe> element itself is.
		if name := strings.ToUpper(n.NodeName); n.FrameID != "" && (name == "IFRAME" || name == "FRAME") {
			out[n.FrameID] = n
		}
		for _, c := range n.Children {
			walk(c)
		}
		for _, c := range n.ShadowRoots {
			walk(c)
		}
		walk(n.ContentDocument)
	}
	walk(root)
	return out
}

func domAttrOf(n *extract.DOMNode, name string) string {
	for i := 0; i+1 < len(n.Attributes); i += 2 {
		if n.Attributes[i] == name {
			return n.Attributes[i+1]
		}
	}
	return ""
}
