package match

import "github.com/sightmap/sightmap/go/sightmap"

// Child frames. An extracted tree holds each child frame's document as the
// single child of its owning <iframe> node, marked by ComponentNode.FrameURL.
// A frame is a page of its own, so matching treats it as one:
//
//   - a selector never crosses the boundary, in either direction: an ancestor
//     outside the frame does not scope a selector inside it, and a child
//     selector's scope ends at the frame's edge;
//   - the frame's document is matched at its own URL, so the view that URL
//     selects (and the global components) supply its components, not the
//     page's view;
//   - what the corpus declares about privacy is inherited across the boundary:
//     the document of a masked or blocked <iframe> is masked or blocked, as the
//     iframe's own subtree would be.
//
// Every entry point that walks a tree matches the top document first and then
// each frame document, outermost first, and merges the answers: they are
// keyed by node, and no node is in two documents.

// frameDoc is one document of an extracted tree: its root, the URL it is
// matched at, and the privacy rank it inherits from the element that hosts it.
type frameDoc struct {
	root    *sightmap.ComponentNode
	url     string
	inherit int
}

// framesOf lists the frame documents hosted directly by n's children,
// inheriting rank.
func framesOf(n *sightmap.ComponentNode, rank int) []frameDoc {
	var out []frameDoc
	for _, c := range n.Children {
		if c.IsFrameRoot() {
			out = append(out, frameDoc{c, c.FrameURL, rank})
		}
	}
	return out
}

// framesIn lists every frame document directly under root's own document (not
// those nested in a frame), inheriting rank. It is the walk a document needs
// when nothing else walks it.
func framesIn(root *sightmap.ComponentNode, rank int) []frameDoc {
	var out []frameDoc
	sightmap.Walk(root, func(n *sightmap.ComponentNode, depth int) bool {
		if depth > 0 && n.IsFrameRoot() {
			out = append(out, frameDoc{n, n.FrameURL, rank})
			return false
		}
		return true
	})
	return out
}

// eachDoc calls visit for the top document of root and then for each frame
// document under it, outermost first. visit returns the frame documents it
// found in the document it was given.
func eachDoc(root *sightmap.ComponentNode, pageURL string, visit func(frameDoc) []frameDoc) {
	queue := []frameDoc{{root: root, url: pageURL, inherit: rankNone}}
	for i := 0; i < len(queue); i++ {
		queue = append(queue, visit(queue[i])...)
	}
}
