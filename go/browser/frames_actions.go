package browser

// Interaction inside child frames. A component id from a snapshot names an
// element by the data-sightmap-id the probe stamped on it; an id minted in a
// child frame carries that frame's prefix ("2_17"), so only ids with a prefix
// send an action looking beyond the top document.
//
// An action runs its scripts in the frame that holds the element (Frame.EvalJSON)
// and sends input on the top page's session, in top-page viewport coordinates:
// Chrome routes a mouse or key event to the frame under the point or in focus,
// whatever process it is in. The coordinates are computed from the live layout
// after the element is scrolled into view, immediately before the event, and
// never from the bounds a snapshot recorded.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/sightmap"
)

// idFramePrefix returns the frame number an id was minted in: 0 for the top
// document's bare ids.
func idFramePrefix(id string) int {
	i := strings.IndexByte(id, '_')
	if i <= 0 {
		return 0
	}
	n, err := strconv.Atoi(id[:i])
	if err != nil {
		return 0
	}
	return n
}

func idSelectorJSON(id string) string {
	sel, _ := json.Marshal(fmt.Sprintf("[data-sightmap-id=%q]", id))
	return string(sel)
}

// frameOfID finds the frame holding the element stamped with id: nil for the
// top document, a frame for a child, an error when no document has it. The
// frame numbered by the id's prefix is tried first; the others are searched in
// case the page's frames moved since the snapshot.
func frameOfID(ctx context.Context, conn *CDPConn, id string) (*Frame, error) {
	if idFramePrefix(id) == 0 {
		return nil, nil
	}
	frames := ChildFrames(ctx, conn)
	if n := idFramePrefix(id); n <= len(frames) {
		frames[0], frames[n-1] = frames[n-1], frames[0]
	}
	sel := idSelectorJSON(id)
	for _, f := range frames {
		raw, err := f.EvalJSON(ctx, actionsJS+fmt.Sprintf("\n__smSelectorExists(%s)", sel))
		if err != nil {
			continue
		}
		var found bool
		if json.Unmarshal(raw, &found) == nil && found {
			return f, nil
		}
	}
	return nil, notFoundErr(id)
}

func notFoundErr(id string) error {
	return fmt.Errorf("component %q not found in live DOM (data-sightmap-id absent; re-run sightmap snapshot)", id)
}

// evalForID runs script in the document holding the element stamped with id
// and reports that document's frame (nil for the top document).
func evalForID(ctx context.Context, conn *CDPConn, id, script string) (json.RawMessage, *Frame, error) {
	f, err := frameOfID(ctx, conn, id)
	if err != nil {
		return nil, nil, err
	}
	if f == nil {
		raw, err := EvalJSON(ctx, conn, script)
		return raw, nil, err
	}
	raw, err := f.EvalJSON(ctx, script)
	return raw, f, err
}

// settleOrigin is f's origin on the top page once layout has stopped moving. A
// scroll of an element inside a cross-process frame reaches the enclosing
// documents asynchronously, so the origin read straight after it can be the
// one from before; the origin is read until two reads 40 ms apart agree. A
// frame in the top process settles on the second read.
func settleOrigin(ctx context.Context, f *Frame) (ox, oy float64, err error) {
	ox, oy, err = f.Origin(ctx)
	for range 25 {
		if err != nil {
			return 0, 0, err
		}
		select {
		case <-ctx.Done():
			return 0, 0, ctx.Err()
		case <-time.After(40 * time.Millisecond):
		}
		nx, ny, nerr := f.Origin(ctx)
		if nerr == nil && nx == ox && ny == oy {
			return ox, oy, nil
		}
		ox, oy, err = nx, ny, nerr
	}
	return ox, oy, err
}

// frameSelectorExists reports whether any child frame has an element matching
// selector.
func frameSelectorExists(ctx context.Context, conn *CDPConn, selector string) bool {
	sel, _ := json.Marshal(selector)
	for _, f := range ChildFrames(ctx, conn) {
		raw, err := f.EvalJSON(ctx, actionsJS+fmt.Sprintf("\n__smSelectorExists(%s)", sel))
		if err != nil {
			continue
		}
		var found bool
		if json.Unmarshal(raw, &found) == nil && found {
			return true
		}
	}
	return false
}

// Focus gives the element node names keyboard focus, in whichever frame holds
// it, without clicking it. Key events are sent to the top page and reach the
// focused element of the focused frame, so a keypress after Focus lands there.
func Focus(ctx context.Context, conn *CDPConn, node *sightmap.ComponentNode) error {
	id := node.Id
	if id == "" {
		return fmt.Errorf("focus: node has no id")
	}
	raw, _, err := evalForID(ctx, conn, id, actionsJS+fmt.Sprintf("\n__smFocusBySelector(%s)", idSelectorJSON(id)))
	if err != nil {
		return err
	}
	var ok bool
	if json.Unmarshal(raw, &ok) != nil || !ok {
		return fmt.Errorf("focus: %q not found in the live DOM (re-run sightmap snapshot)", id)
	}
	return nil
}
