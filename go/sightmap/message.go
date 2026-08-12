package sightmap

import (
	"regexp"
	"strings"
)

// MessageDef is a named console-output or runtime-exception pattern from the
// sightmap corpus (SEP-0006). It gives console activity the same thing
// RequestDef gives network activity: a named entity other parts of the corpus
// can reference, so "a cart version mismatch broke checkout" can be stated once
// and pointed at by name.
//
// A record matches when both declared constraints hold: Level compared
// case-insensitively for equality, and Message as a regex against the record's
// text. An omitted constraint matches anything.
//
// Messages are file-root only. Unlike components and requests there is no
// view-scoped form, because a console record is not scoped to a page the way a
// DOM element is.
type MessageDef struct {
	Name string `json:"name"`
	// Level is compared case-insensitively for equality against the observed
	// record's level. Match-any when empty.
	//
	// The reference capture emits log, debug, info, warn, error, and exception.
	// An uncaught exception arrives as level "exception", NOT as "error", so
	// `level: ERROR` does not match one. See LevelException.
	Level string `json:"level,omitempty"`
	// Message is an RE2 regex (Go's regexp; no backreferences or lookaround)
	// matched against the record's text. Match-any when empty. Compiled at
	// validation time so a malformed pattern is reported to the author rather
	// than failing later.
	Message     string `json:"message,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`

	// re is the compiled Message pattern, cached by the loader (precompile) so
	// MessagesForRecord doesn't recompile per record. Nil when Message is empty,
	// the pattern is invalid (validation reports that), or the def was built in
	// memory without going through the loader; MessagesForRecord compiles on the
	// fly in that last case. Unexported, so it never serializes.
	re *regexp.Regexp
}

// precompile caches the compiled Message pattern on the def. The loader calls it
// as each MessageDef is built, so a corpus loaded once and matched against many
// records compiles each pattern once rather than once per record. A no-op for an
// empty pattern; an invalid pattern leaves re nil (checkMessages reports it, and
// MessagesForRecord then skips the def).
func (m *MessageDef) precompile() {
	if m.Message == "" {
		return
	}
	if re, err := regexp.Compile(m.Message); err == nil {
		m.re = re
	}
}

// Console levels the reference capture emits. An observed record carries one of
// these; a MessageDef.Level is matched against it case-insensitively.
//
// LevelException is the one worth noting: SEP-0006 argues that console output
// and exceptions need one entity rather than two, and that holds because the
// origin is carried as a level value. It does not mean an exception is folded
// into "error" — the two stay distinguishable, so a corpus that wants
// exceptions must say so.
const (
	LevelLog       = "log"
	LevelDebug     = "debug"
	LevelInfo      = "info"
	LevelWarn      = "warn"
	LevelError     = "error"
	LevelException = "exception"
)

// KnownMessageLevels is the vocabulary above, in increasing-severity order. The
// `level` field is deliberately open (a corpus may name a level this capture
// never emits), so this drives an advisory lint rather than validation.
var KnownMessageLevels = []string{
	LevelLog, LevelDebug, LevelInfo, LevelWarn, LevelError, LevelException,
}

// MessageMatch records which MessageDef classified an observed Message. It is
// the message-side analogue of ComponentMatch: a projection of the matched
// definition carrying the fields a consumer needs to link a classification back
// to what produced it. Messages have no live-extracted properties, so — unlike
// ComponentMatch — it carries no property values.
type MessageMatch struct {
	Name        string
	Description string
	Source      string
}

// MessagesForRecord returns every MessageDef that classifies the observed record
// rec, as MessageMatch projections in corpus (declaration) order. A def matches
// when both its declared constraints hold: Level compared case-insensitively for
// equality, and Message as an RE2 regex (Go's regexp) against rec.Text. An
// omitted constraint matches anything, so a def declaring neither matches every
// record.
//
// All matches apply — there is no winner. Per SEP-0006 a record matching more
// than one entry (len(result) > 1) is an ambiguity a consumer MUST surface
// rather than silently resolving to a first match; this method never picks one
// for you, mirroring how Matcher.Conflicts returns every colliding claim.
//
// A def whose Message fails to compile is skipped (it never matches) rather than
// panicking: an in-memory Corpus may not have been validated, and matching stays
// silent the way value omission does. Validation reports the malformed pattern
// separately (message-regex-invalid).
func (c *Corpus) MessagesForRecord(rec Message) []MessageMatch {
	var out []MessageMatch
	for i := range c.Messages {
		m := &c.Messages[i]
		if m.Level != "" && !strings.EqualFold(m.Level, rec.Level) {
			continue
		}
		if m.Message != "" {
			re := m.re
			if re == nil {
				// Not precompiled: an in-memory corpus that skipped the loader, or
				// an invalid pattern. Compile on the fly; a bad pattern never matches.
				var err error
				if re, err = regexp.Compile(m.Message); err != nil {
					continue
				}
			}
			if !re.MatchString(rec.Text) {
				continue
			}
		}
		out = append(out, MessageMatch{
			Name:        m.Name,
			Description: m.Description,
			Source:      m.Source,
		})
	}
	return out
}
