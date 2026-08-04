package sightmap

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// checkMessages validates console/exception patterns (SEP-0006).
//
// The name-uniqueness check here is load-bearing beyond messages themselves: a
// signals rule resolves its ref against entity names, and that resolution counts
// distinct entity KINDS rather than entries. Two messages sharing a name
// therefore collapse to one kind, so neither signal-ref-unresolved nor
// signal-ref-ambiguous fires and the ref silently resolves to an ambiguous pair.
// Catching the duplicate at its source is what keeps that state from existing.
func checkMessages(msgs []MessageDef) []ValidationError {
	var errs []ValidationError

	for _, m := range msgs {
		if m.Name == "" {
			errs = append(errs, ValidationError{
				Code:     "missing-name",
				Severity: SeverityError,
				Message:  fmt.Sprintf("message is missing a name (level %q, message %q)", m.Level, m.Message),
			})
		}
		// A `message:` is a regex an author wrote by hand. The corpus already
		// compiles author-supplied component selectors at validation time for
		// exactly this reason; do the same rather than storing a pattern nobody
		// has proven is a pattern.
		if m.Message != "" {
			if _, err := regexp.Compile(m.Message); err != nil {
				errs = append(errs, ValidationError{
					Component: m.Name,
					Code:      "message-regex-invalid",
					Severity:  SeverityError,
					Message:   fmt.Sprintf("message %q: message regex parse error: %v", m.Name, err),
				})
			}
		}
	}

	errs = append(errs, checkMessageNameCollisions(msgs)...)
	errs = append(errs, checkMessageConflicts(msgs)...)
	return errs
}

// checkMessageNameCollisions warns when two or more messages share a name.
// Mirrors checkViewNameCollisions: the corpus still loads, but a reference to
// that name is ambiguous and resolves silently.
func checkMessageNameCollisions(msgs []MessageDef) []ValidationError {
	byName := map[string]int{}
	for _, m := range msgs {
		if m.Name == "" {
			continue // reported as missing-name
		}
		byName[m.Name]++
	}

	names := make([]string, 0, len(byName))
	for name, n := range byName {
		if n > 1 {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var errs []ValidationError
	for _, name := range names {
		errs = append(errs, ValidationError{
			Component: name,
			Code:      "merge-collision-message",
			Severity:  SeverityWarning,
			Message: fmt.Sprintf("message name %q is declared %d times; a signals rule referencing it cannot distinguish them",
				name, byName[name]),
		})
	}
	return errs
}

// checkMessageConflicts warns when two entries can match the same record, which
// SEP-0006 requires be surfaced rather than silently resolved to a first match.
//
// Only the statically decidable half is checked: same level (or one omitting
// it) plus an identical or absent message pattern. Deciding whether two
// different regexes can both match some record is undecidable in general, and
// full multi-match ambiguity needs a live record, which is a consumer-side
// obligation. This catches the realistic authoring mistake: the same pattern
// declared twice under different names.
func checkMessageConflicts(msgs []MessageDef) []ValidationError {
	var errs []ValidationError
	for i := range msgs {
		for j := i + 1; j < len(msgs); j++ {
			a, b := msgs[i], msgs[j]
			if a.Name == "" || b.Name == "" || a.Name == b.Name {
				continue // reported as missing-name / merge-collision-message
			}
			if !levelsOverlap(a.Level, b.Level) {
				continue
			}
			if a.Message != "" && b.Message != "" && a.Message != b.Message {
				continue
			}
			errs = append(errs, ValidationError{
				Component: a.Name,
				Code:      "message-conflict",
				Severity:  SeverityWarning,
				Message: fmt.Sprintf("messages %q and %q can match the same record (level %s, pattern %s); a consumer must surface the ambiguity rather than pick one",
					a.Name, b.Name, narrower(a.Level, b.Level), narrower(a.Message, b.Message)),
			})
		}
	}
	return errs
}

// levelsOverlap reports whether two declared levels can describe one record. An
// empty level is match-any, so it overlaps everything.
func levelsOverlap(a, b string) bool {
	if a == "" || b == "" {
		return true
	}
	return strings.EqualFold(a, b)
}

// narrower renders the more specific of two overlapping constraints for a
// diagnostic message. Both empty means neither constrains anything.
func narrower(a, b string) string {
	if a == "" && b == "" {
		return "any"
	}
	if a == "" {
		return fmt.Sprintf("%q", b)
	}
	return fmt.Sprintf("%q", a)
}
