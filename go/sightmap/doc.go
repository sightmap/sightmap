// Package sightmap is the model for the sightmap spec: it loads and validates
// .sightmap/ YAML corpus files and exposes the whole vocabulary from one import
// — corpus definitions (Corpus, ViewDef, ComponentDef, RequestDef, MessageDef,
// and the property/payload defs), the observed runtime records (ComponentNode
// and its Element identity, Request, Message), the match result types
// (ComponentMatch, MessageMatch, Conflict), and the atomic selector parse/match primitive
// (ParseSightmapSelector, Matches). It is a self-contained leaf: it depends on
// no other package in this module, so any consumer — including the NFA engine in
// package match — builds on it without pulling in a browser/CDP tool.
package sightmap
