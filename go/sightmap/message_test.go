package sightmap_test

import (
	"reflect"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func msgNames(matches []sightmap.MessageMatch) []string {
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = m.Name
	}
	return names
}

func TestMessagesForRecord(t *testing.T) {
	corpus := &sightmap.Corpus{
		Messages: []sightmap.MessageDef{
			{Name: "CartVersionMismatch", Level: "error", Message: "cart version mismatch"},
			{Name: "SlowNetworkWarning", Level: "warn", Message: `request .* took over \d+ms`},
			{Name: "AnyError", Level: "error"},              // level-only: any error text
			{Name: "MentionsCart", Message: "cart"},         // message-only: any level
			{Name: "UncaughtException", Level: "exception"}, // exception is its own level
		},
	}

	cases := []struct {
		name string
		rec  sightmap.Message
		want []string
	}{
		{
			// The specific error and the two match-any-on-one-axis defs all fire,
			// in declaration order — there is no winner.
			name: "error record matches specific + level-only + message-only",
			rec:  sightmap.Message{Level: "error", Text: "cart version mismatch detected"},
			want: []string{"CartVersionMismatch", "AnyError", "MentionsCart"},
		},
		{
			// Level is compared case-insensitively.
			name: "level match is case-insensitive",
			rec:  sightmap.Message{Level: "ERROR", Text: "unrelated failure"},
			want: []string{"AnyError"},
		},
		{
			name: "warn record matches the regex pattern",
			rec:  sightmap.Message{Level: "warn", Text: "request /api/x took over 5000ms"},
			want: []string{"SlowNetworkWarning"},
		},
		{
			// An uncaught exception arrives as level "exception", NOT "error", so
			// the error-level defs do not claim it.
			name: "exception level does not match error defs",
			rec:  sightmap.Message{Level: "exception", Text: "TypeError: x is undefined"},
			want: []string{"UncaughtException"},
		},
		{
			name: "no match",
			rec:  sightmap.Message{Level: "info", Text: "everything is fine"},
			want: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := msgNames(corpus.MessagesForRecord(tc.rec))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("MessagesForRecord() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A def declaring neither level nor message matches every record. SEP-0006 says
// this is useless in practice but not a schema violation; matching must still
// treat both-omitted as match-any.
func TestMessagesForRecord_MatchAll(t *testing.T) {
	corpus := &sightmap.Corpus{
		Messages: []sightmap.MessageDef{{Name: "Everything"}},
	}
	got := msgNames(corpus.MessagesForRecord(sightmap.Message{Level: "debug", Text: "anything at all"}))
	if want := []string{"Everything"}; !reflect.DeepEqual(got, want) {
		t.Errorf("MessagesForRecord() = %v, want %v", got, want)
	}
}

// The multi-match MUST: a record matching more than one entry returns all of
// them, in declaration order, so a consumer can surface the ambiguity rather
// than the library silently resolving to a first match.
func TestMessagesForRecord_MultiMatchReturnsAll(t *testing.T) {
	corpus := &sightmap.Corpus{
		Messages: []sightmap.MessageDef{
			{Name: "First", Level: "error", Message: "boom"},
			{Name: "Second", Level: "error", Message: "boom"},
		},
	}
	got := corpus.MessagesForRecord(sightmap.Message{Level: "error", Text: "boom"})
	if len(got) != 2 {
		t.Fatalf("expected 2 matches (ambiguous), got %d: %v", len(got), msgNames(got))
	}
	if want := []string{"First", "Second"}; !reflect.DeepEqual(msgNames(got), want) {
		t.Errorf("order = %v, want %v", msgNames(got), want)
	}
}

// A malformed regex on an (unvalidated) in-memory corpus is skipped, not
// panicked on: the bad def simply never matches, and a valid sibling still does.
func TestMessagesForRecord_InvalidRegexSkipped(t *testing.T) {
	corpus := &sightmap.Corpus{
		Messages: []sightmap.MessageDef{
			{Name: "Bad", Message: "("}, // unbalanced group — regexp.Compile fails
			{Name: "Good", Message: "ok"},
		},
	}
	got := msgNames(corpus.MessagesForRecord(sightmap.Message{Level: "info", Text: "ok"}))
	if want := []string{"Good"}; !reflect.DeepEqual(got, want) {
		t.Errorf("MessagesForRecord() = %v, want %v", got, want)
	}
}
