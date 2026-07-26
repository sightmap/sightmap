package browser

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/comps"
)

// evalResult wraps a JS return value in the CDP Runtime.evaluate envelope the
// fake server hands back.
func evalResult(valueJSON string) json.RawMessage {
	return json.RawMessage(`{"result":{"value":` + valueJSON + `},"exceptionDetails":null}`)
}

func exprOf(params json.RawMessage) string {
	var p struct {
		Expression string `json:"expression"`
	}
	_ = json.Unmarshal(params, &p)
	return p.Expression
}

func nodeWithID(id string) *comps.ComponentNode {
	return &comps.ComponentNode{Id: id, Bounds: &comps.Bounds{X: 0, Y: 0, Width: 20, Height: 10}}
}

func TestClick_OffScreenErrors(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			return evalResult(`{"found":true,"inViewport":false,"hit":false,"cx":0,"cy":0}`)
		}
		return json.RawMessage(`{}`)
	})
	_, _, err := Click(context.Background(), conn, nodeWithID("5"))
	if err == nil || !strings.Contains(err.Error(), "viewport") {
		t.Fatalf("expected an off-screen/viewport error, got %v", err)
	}
}

func TestClick_CoveredErrors(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			return evalResult(`{"found":true,"inViewport":true,"hit":false,"cx":100,"cy":50}`)
		}
		return json.RawMessage(`{}`)
	})
	_, _, err := Click(context.Background(), conn, nodeWithID("5"))
	if err == nil || !strings.Contains(err.Error(), "covered") {
		t.Fatalf("expected a covered/overlay error, got %v", err)
	}
}

func TestClick_SuccessReturnsPostScrollCoords(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			return evalResult(`{"found":true,"inViewport":true,"hit":true,"cx":123,"cy":45}`)
		}
		return json.RawMessage(`{}`) // Input.dispatchMouseEvent
	})
	x, y, err := Click(context.Background(), conn, nodeWithID("5"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if x != 123 || y != 45 {
		t.Errorf("Click returned (%d,%d), want the post-scroll center (123,45)", x, y)
	}
}

func TestFill_DidNotStickErrors(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			switch {
			case strings.Contains(exprOf(params), "elementFromPoint"):
				return evalResult(`{"found":true,"inViewport":true,"hit":true,"cx":10,"cy":5}`)
			case strings.Contains(exprOf(params), "el.value"):
				return evalResult(`""`) // field still empty → didn't stick
			}
		}
		return json.RawMessage(`{}`)
	})
	err := Fill(context.Background(), conn, nodeWithID("3"), "hello")
	if err == nil || !strings.Contains(err.Error(), "--clear") {
		t.Fatalf("expected a 'didn't stick / --clear' error, got %v", err)
	}
}

func TestFill_StuckSucceeds(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			switch {
			case strings.Contains(exprOf(params), "elementFromPoint"):
				return evalResult(`{"found":true,"inViewport":true,"hit":true,"cx":10,"cy":5}`)
			case strings.Contains(exprOf(params), "el.value"):
				return evalResult(`"hello"`)
			}
		}
		return json.RawMessage(`{}`)
	})
	if err := Fill(context.Background(), conn, nodeWithID("3"), "hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
