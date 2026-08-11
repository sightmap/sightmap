package sightmap

import (
	"strconv"
	"strings"
)

// PropertyValue is one property value extracted from live observation of a
// matched entity. Value is the extracted, post-transform string; the typed
// accessors parse it on demand. The extraction itself (which selector/field to
// read, and any transform) is described by the entity's property definitions
// (ComponentPropertyDef / RequestPropertyDef) — a PropertyValue is only the
// output.
type PropertyValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// String returns the raw extracted value.
func (v PropertyValue) String() string { return v.Value }

// Int parses the value as a base-10 integer. ok is false when it doesn't parse.
func (v PropertyValue) Int() (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(v.Value))
	return n, err == nil
}

// Float parses the value as a float64. ok is false when it doesn't parse.
func (v PropertyValue) Float() (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(v.Value), 64)
	return f, err == nil
}

// Bool parses the value as a boolean (per strconv.ParseBool: 1/t/true/…,
// 0/f/false/…). ok is false when it doesn't parse.
func (v PropertyValue) Bool() (bool, bool) {
	b, err := strconv.ParseBool(strings.TrimSpace(v.Value))
	return b, err == nil
}
