package sightmap

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
	// Message is a regex matched against the record's text. Match-any when
	// empty. Compiled at validation time so a malformed pattern is reported to
	// the author rather than failing later.
	Message     string `json:"message,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
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
