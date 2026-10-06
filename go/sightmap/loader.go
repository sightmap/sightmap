package sightmap

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Loader is the source of sightmap corpus data.
type Loader interface {
	Load() (*Corpus, error)
}

// LoaderFunc adapts any func() (*Corpus, error) to the Loader interface.
type LoaderFunc func() (*Corpus, error)

// Load implements Loader.
func (f LoaderFunc) Load() (*Corpus, error) { return f() }

// StaticLoader returns a Loader backed by an already-constructed Corpus.
func StaticLoader(c *Corpus) Loader {
	return LoaderFunc(func() (*Corpus, error) { return c, nil })
}

// DirLoader reads all YAML files from a .sightmap/ directory tree.
// Files with a top-level components: key are treated as global component
// files; files with a top-level views: key are treated as view files.
func DirLoader(path string) Loader {
	return LoaderFunc(func() (*Corpus, error) {
		return loadDir(path)
	})
}

// ---- raw YAML types (unexported) --------------------------------------------

type rawFile struct {
	Version      int               `yaml:"version"`
	Environments []rawEnvironment  `yaml:"environments"`
	Origins      map[string]string `yaml:"origins"`
	Memory       []string          `yaml:"memory"`
	Components   []rawComponent    `yaml:"components"`
	Definitions  []rawComponent    `yaml:"definitions"`
	Views        []rawView         `yaml:"views"`
	Requests     []rawRequest      `yaml:"requests"`
	Messages     []rawMessage      `yaml:"messages"`
	Signals      []rawSignal       `yaml:"signals"`
	URL          string            `yaml:"url"`
	Snapshots    []rawSnapshot     `yaml:"snapshots"`
}

type rawEnvironment struct {
	Name      string            `yaml:"name"`
	Platform  string            `yaml:"platform"`
	AppID     string            `yaml:"app_id"`
	BuildType string            `yaml:"build_type"`
	Backend   string            `yaml:"backend"`
	Origins   map[string]string `yaml:"origins"`
}

type rawSignal struct {
	Name string   `yaml:"name"`
	Ref  string   `yaml:"ref"`
	Tags []string `yaml:"tags"`
}

type rawMessage struct {
	Name        string               `yaml:"name"`
	Level       string               `yaml:"level"`
	Message     string               `yaml:"message"`
	Description string               `yaml:"description"`
	Source      string               `yaml:"source"`
	Tags        []string             `yaml:"tags"`
	Properties  []rawMessageProperty `yaml:"properties"`
}

type rawMessageProperty struct {
	Name    string     `yaml:"name"`
	Extract rawExtract `yaml:"extract"`
	Source  string     `yaml:"source"`
	Field   string     `yaml:"field"`
	Pattern string     `yaml:"pattern"`
}

type rawSnapshot struct {
	Name  string `yaml:"name"`
	Notes string `yaml:"notes"`
	URL   string `yaml:"url"`
}

type rawView struct {
	Name        string           `yaml:"name"`
	Route       string           `yaml:"route"`
	URL         string           `yaml:"url"`
	Description string           `yaml:"description"`
	Memory      []string         `yaml:"memory"`
	Components  []rawComponent   `yaml:"components"`
	Requests    []rawRequest     `yaml:"requests"`
	Stability   string           `yaml:"stability"`
	Tags        []string         `yaml:"tags"`
	Properties  []rawURLProperty `yaml:"properties"`

	Access *rawAccess `yaml:"access"`
	// An explicit `[]` decodes to a non-nil empty slice and an absent key to
	// nil, which is how validation tells environments-empty from omission.
	Environments []string `yaml:"environments"`
	Origins      []string `yaml:"origins"`
}

type rawAccess struct {
	Status string `yaml:"status"`
	Reason string `yaml:"reason"`
}

type rawProperty struct {
	Name    string     `yaml:"name"`
	Extract rawExtract `yaml:"extract"`
}

type rawRequest struct {
	Name         string               `yaml:"name"`
	Route        string               `yaml:"route"`
	Method       string               `yaml:"method"`
	Description  string               `yaml:"description"`
	Source       string               `yaml:"source"`
	Request      *rawPayload          `yaml:"request"`
	Response     *rawPayload          `yaml:"response"`
	Headers      []string             `yaml:"headers"`
	Memory       []string             `yaml:"memory"`
	Tags         []string             `yaml:"tags"`
	Properties   []rawRequestProperty `yaml:"properties"`
	Environments []string             `yaml:"environments"`
	Origins      []string             `yaml:"origins"`
}

// toURLPropertyDefs converts raw view URL properties verbatim. Nil when none are
// declared: a :name segment in the route binds implicitly and needs no entry.
func toURLPropertyDefs(raws []rawURLProperty) []URLPropertyDef {
	if len(raws) == 0 {
		return nil
	}
	out := make([]URLPropertyDef, 0, len(raws))
	for _, rp := range raws {
		out = append(out, URLPropertyDef{Name: rp.Name, Extract: rp.Extract.url()})
	}
	return out
}

type rawURLProperty struct {
	Name    string     `yaml:"name"`
	Extract rawExtract `yaml:"extract"`
}

type rawRequestProperty struct {
	Name    string     `yaml:"name"`
	Extract rawExtract `yaml:"extract"`
	Source  string     `yaml:"source"`
	Field   string     `yaml:"field"`
	Pattern string     `yaml:"pattern"`
}

type rawPayload struct {
	Fields []rawField `yaml:"fields"`
}

type rawField struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

type rawComponent struct {
	Name        string         `yaml:"name"`
	Ref         string         `yaml:"$ref"`
	Selector    rawSelector    `yaml:"selector"`
	Description string         `yaml:"description"`
	Source      string         `yaml:"source"`
	Memory      []string       `yaml:"memory"`
	Tags        []string       `yaml:"tags"`
	Watch       bool           `yaml:"watch"`
	Privacy     string         `yaml:"privacy"`
	Children    []rawComponent `yaml:"children"`
	Properties  []rawProperty  `yaml:"properties"`
	Stability   string         `yaml:"stability"`
}

// rawSelector handles the selector field as either a scalar string or a YAML
// sequence of strings, joining them with a comma so splitSelectors can handle both.
type rawSelector string

func (r *rawSelector) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*r = rawSelector(value.Value)
		return nil
	case yaml.SequenceNode:
		var parts []string
		if err := value.Decode(&parts); err != nil {
			return err
		}
		*r = rawSelector(strings.Join(parts, ","))
		return nil
	default:
		return fmt.Errorf("unexpected YAML node kind %v for selector", value.Kind)
	}
}

// ---- DirLoader implementation -----------------------------------------------

func loadDir(path string) (*Corpus, error) {
	// Collect all .yaml / .yml files in lexical order.
	var yamlPaths []string
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip non-corpus subdirectories: review/ holds punch-list YAML
			// sequences (not corpus files), snapshots/ holds snapshot blobs.
			if p != path && (d.Name() == "review" || d.Name() == "snapshots") {
				return fs.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".yaml" || ext == ".yml" {
			yamlPaths = append(yamlPaths, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sightmap: walk %q: %w", path, err)
	}

	var memory []string
	var globalRaws []rawComponent
	var definitionRaws []rawComponent
	var globalRequestRaws []rawRequest
	var messageRaws []rawMessage
	var signalRaws []rawSignal
	type viewFileWithPath struct {
		rf   rawFile
		path string
	}
	var viewFiles []viewFileWithPath
	var fieldDiags []ValidationError
	envReg := newEnvironmentRegistry()

	for _, p := range yamlPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("sightmap: read %q: %w", p, err)
		}
		var rf rawFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			return nil, fmt.Errorf("sightmap: parse %q: %w", p, err)
		}
		// Tooling files (config.yaml, survey.yaml) have their own schemas and are
		// not corpus files — don't run the corpus unknown-field check over them.
		if base := filepath.Base(p); base != "config.yaml" && base != "config.yml" && base != "survey.yaml" && base != "survey.yml" {
			fieldDiags = append(fieldDiags, unknownFieldWarnings(data, base)...)
		}
		memory = append(memory, rf.Memory...)
		envReg.add(rf, filepath.Base(p))
		if len(rf.Components) > 0 {
			globalRaws = append(globalRaws, rf.Components...)
		}
		if len(rf.Definitions) > 0 {
			definitionRaws = append(definitionRaws, rf.Definitions...)
		}
		if len(rf.Requests) > 0 {
			globalRequestRaws = append(globalRequestRaws, rf.Requests...)
		}
		if len(rf.Messages) > 0 {
			messageRaws = append(messageRaws, rf.Messages...)
		}
		if len(rf.Signals) > 0 {
			signalRaws = append(signalRaws, rf.Signals...)
		}
		if len(rf.Views) > 0 {
			viewFiles = append(viewFiles, viewFileWithPath{rf: rf, path: p})
		}
	}

	// Build the registry used for $ref resolution: file-root globals and
	// file-root definitions (SEP-0019) share one namespace. A global wins a
	// name clash, so adding a definition never changes what an existing $ref
	// expands to.
	reg := make(map[string]rawComponent, len(globalRaws)+len(definitionRaws))
	for _, gc := range globalRaws {
		if gc.Name != "" {
			if _, dup := reg[gc.Name]; !dup {
				reg[gc.Name] = gc
			}
		}
	}
	for _, dc := range definitionRaws {
		if dc.Name != "" {
			if _, dup := reg[dc.Name]; !dup {
				reg[dc.Name] = dc
			}
		}
	}

	// One flatten context is shared across the global list and every view so
	// that $ref cycle detection and diagnostics accumulate in one place.
	ctx := &flattenCtx{reg: reg}

	// Warn on duplicate top-level global component names. This must run on the
	// RAW globals, not the flattened list: flattening a child component reused
	// under several parents yields multiple same-name entries, which would
	// otherwise look like a collision.
	ctx.diagnostics = append(ctx.diagnostics, globalNameCollisions(globalRaws)...)
	ctx.diagnostics = append(ctx.diagnostics, definitionNameCollisions(globalRaws, definitionRaws)...)

	// Flatten global components (hierarchy → compound descendant selectors).
	globalComps := flattenAll(globalRaws, ctx, 0)

	// Flatten definitions too, so validation and lint see a definition no view
	// references yet. They are never matched on their own (not in
	// GlobalComponents); a view receives one only through a $ref.
	definitionComps := flattenAll(definitionRaws, ctx, 0)

	// Global (file-root) request definitions. Requests are flat — no $ref,
	// hierarchy, or selector cascade — so they convert directly.
	globalRequests := toRequestDefs(globalRequestRaws, ctx)

	// Build views, expanding $refs and flattening each view's component list.
	var views []ViewDef
	for _, vfp := range viewFiles {
		vf := vfp.rf
		// Extract source file basename (without extension)
		basename := filepath.Base(vfp.path)
		basename = strings.TrimSuffix(basename, filepath.Ext(basename))

		// Convert snapshots
		var snapshots []Snapshot
		for _, rs := range vf.Snapshots {
			snapshots = append(snapshots, Snapshot{
				Name:  rs.Name,
				Notes: rs.Notes,
				URL:   rs.URL,
			})
		}

		for _, rv := range vf.Views {
			var access Access
			if rv.Access != nil {
				access = Access{Status: rv.Access.Status, Reason: rv.Access.Reason}
			}
			// Per-view url wins; fall back to the file-level url as a default.
			viewURL := rv.URL
			if viewURL == "" {
				viewURL = vf.URL
			}
			views = append(views, ViewDef{
				Name:       rv.Name,
				Route:      rv.Route,
				Memory:     rv.Memory,
				Components: flattenAll(rv.Components, ctx, -1),
				Requests:   toRequestDefs(rv.Requests, ctx),
				Tags:       rv.Tags,
				Properties: toURLPropertyDefs(rv.Properties),

				Environments: rv.Environments,
				Origins:      rv.Origins,
				Stability:    rv.Stability,
				Access:       access,
				URL:          viewURL,
				Snapshots:    snapshots,
				SourceFile:   basename,
			})
		}
	}

	return &Corpus{
		Memory:           memory,
		GlobalComponents: globalComps,
		Definitions:      definitionComps,
		Views:            views,
		Requests:         globalRequests,
		Messages:         toMessageDefs(messageRaws),
		Signals:          toSignalDefs(signalRaws),
		Environments:     envReg.envs,
		SharedOrigins:    envReg.shared,
		loadDiagnostics:  append(append(ctx.diagnostics, envReg.diags...), fieldDiags...),
	}, nil
}

// environmentRegistry merges file-root environments and shared origins into the
// project-wide registries SEP-0014 requires. Unlike the component $ref registry,
// the first definition of a name wins, so add must be called in file-path order.
type environmentRegistry struct {
	envs      []EnvironmentDef
	envFile   map[string]string // environment name -> file that won it
	shared    map[string]string
	sharedSrc map[string]string // shared origin name -> file that won it
	diags     []ValidationError
}

func newEnvironmentRegistry() *environmentRegistry {
	return &environmentRegistry{envFile: map[string]string{}, sharedSrc: map[string]string{}}
}

func (r *environmentRegistry) add(rf rawFile, file string) {
	for _, re := range rf.Environments {
		// Nameless entries are kept so validation can report them.
		if re.Name != "" {
			if winner, dup := r.envFile[re.Name]; dup {
				r.diags = append(r.diags, ValidationError{
					File:      file,
					Component: re.Name,
					Code:      "environment-name-collision",
					Severity:  SeverityWarning,
					Message:   fmt.Sprintf("environment %q is already defined in %s; the first definition by source path wins and this one is ignored", re.Name, winner),
				})
				continue
			}
			r.envFile[re.Name] = file
		}
		platform := re.Platform
		if platform == "" {
			platform = PlatformWeb
		}
		r.envs = append(r.envs, EnvironmentDef{
			Name:       re.Name,
			Platform:   platform,
			AppID:      re.AppID,
			BuildType:  re.BuildType,
			Backend:    re.Backend,
			Origins:    re.Origins,
			SourceFile: file,
		})
	}
	// Sorted so collision diagnostics come out in a stable order.
	for _, name := range sortedKeys(rf.Origins) {
		if winner, dup := r.sharedSrc[name]; dup {
			r.diags = append(r.diags, ValidationError{
				File:      file,
				Component: name,
				Code:      "origin-name-collision",
				Severity:  SeverityWarning,
				Message:   fmt.Sprintf("shared origin %q is already defined in %s; the first definition by source path wins and this one is ignored", name, winner),
			})
			continue
		}
		if r.shared == nil {
			r.shared = map[string]string{}
		}
		r.shared[name] = rf.Origins[name]
		r.sharedSrc[name] = file
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---- flattening helpers -----------------------------------------------------

// rawPropsToMatch converts a slice of rawProperty to ComponentPropertyDef.
func rawPropsToMatch(rps []rawProperty) []ComponentPropertyDef {
	if len(rps) == 0 {
		return nil
	}
	ps := make([]ComponentPropertyDef, len(rps))
	for i, rp := range rps {
		ps[i] = ComponentPropertyDef{Name: rp.Name, Extract: rp.Extract.component()}
	}
	return ps
}

// toRequestDefs converts raw request definitions into RequestDefs. A request
// missing its required name or route is dropped with a diagnostic (the schema
// requires both), mirroring how flattenOne surfaces missing component fields.
func toRequestDefs(rrs []rawRequest, ctx *flattenCtx) []RequestDef {
	if len(rrs) == 0 {
		return nil
	}
	out := make([]RequestDef, 0, len(rrs))
	for _, rr := range rrs {
		if rr.Name == "" {
			ctx.addDiag("request-missing-name\x00"+rr.Route, ValidationError{
				Code:     "missing-name",
				Severity: SeverityError,
				Message:  fmt.Sprintf("request is missing a name (route %q)", rr.Route),
			})
			continue
		}
		if rr.Route == "" {
			ctx.addDiag("request-missing-route\x00"+rr.Name, ValidationError{
				Component: rr.Name,
				Code:      "missing-route",
				Severity:  SeverityError,
				Message:   fmt.Sprintf("request %q is missing a route", rr.Name),
			})
			continue
		}
		out = append(out, RequestDef{
			Name:         rr.Name,
			Route:        rr.Route,
			Method:       rr.Method,
			Description:  rr.Description,
			Source:       rr.Source,
			Request:      toPayload(rr.Request),
			Response:     toPayload(rr.Response),
			Headers:      rr.Headers,
			Memory:       rr.Memory,
			Tags:         rr.Tags,
			Properties:   toRequestProperties(rr.Properties),
			Environments: rr.Environments,
			Origins:      rr.Origins,
		})
	}
	return out
}

// toRequestProperties converts raw property declarations (SEP-0005). Shape
// constraints (a valid name, exactly one of field/pattern) are reported by
// checkRequestProperties at validation time rather than dropped here, so an
// author sees every problem at once instead of losing entries silently.
func toRequestProperties(rps []rawRequestProperty) []RequestPropertyDef {
	if len(rps) == 0 {
		return nil
	}
	out := make([]RequestPropertyDef, 0, len(rps))
	for _, rp := range rps {
		out = append(out, RequestPropertyDef{
			Name:    rp.Name,
			Extract: rp.Extract.sourced(rp.Source, rp.Field, rp.Pattern),
		})
	}
	return out
}

// toMessageDefs converts raw console/exception patterns (SEP-0006). Shape
// problems (a missing name, a duplicate, an uncompilable regex) are reported by
// checkMessages at validation time rather than dropped here, so an author sees
// every problem at once.
// toSignalDefs converts raw signal definitions into SignalDefs. Signals are flat
// (name + ref + tags; no $ref, hierarchy, or selector cascade), so they convert
// directly; ref resolution and the Component/View restriction are checked by
// validate_signal.go, not here.
func toSignalDefs(rss []rawSignal) []SignalDef {
	if len(rss) == 0 {
		return nil
	}
	out := make([]SignalDef, 0, len(rss))
	for _, rs := range rss {
		out = append(out, SignalDef{Name: rs.Name, Ref: rs.Ref, Tags: rs.Tags})
	}
	return out
}

func toMessageDefs(rms []rawMessage) []MessageDef {
	if len(rms) == 0 {
		return nil
	}
	out := make([]MessageDef, 0, len(rms))
	for _, rm := range rms {
		md := MessageDef{
			Name:        rm.Name,
			Level:       rm.Level,
			Message:     rm.Message,
			Description: rm.Description,
			Source:      rm.Source,
			Tags:        rm.Tags,
			Properties:  toMessageProperties(rm.Properties),
		}
		md.precompile() // cache the compiled pattern once, at load time
		out = append(out, md)
	}
	return out
}

// toMessageProperties converts raw stack-addressing property declarations
// (SEP-0006 follow-on). Shape problems (a bad name, a wrong source, a missing
// field) are reported by checkMessageProperties at validation time rather than
// dropped here, so an author sees every problem at once.
func toMessageProperties(rps []rawMessageProperty) []MessagePropertyDef {
	if len(rps) == 0 {
		return nil
	}
	out := make([]MessagePropertyDef, 0, len(rps))
	for _, rp := range rps {
		out = append(out, MessagePropertyDef{
			Name:    rp.Name,
			Extract: rp.Extract.sourced(rp.Source, rp.Field, rp.Pattern),
		})
	}
	return out
}

// toPayload converts an optional raw payload (request or response body shape).
func toPayload(rp *rawPayload) *Payload {
	if rp == nil {
		return nil
	}
	p := &Payload{}
	for _, rf := range rp.Fields {
		p.Fields = append(p.Fields, Field{Name: rf.Name, Type: rf.Type, Description: rf.Description})
	}
	return p
}

// globalNameCollisions warns when two or more top-level global components share a
// name with different selectors. A duplicated global name is ambiguous — both
// match every view and resolution falls back to declaration order. (Same
// name + same selector is a true duplicate, reported as an error elsewhere, so
// it is skipped here.)
func globalNameCollisions(globals []rawComponent) []ValidationError {
	type acc struct {
		count int
		sels  map[string]bool
	}
	byName := map[string]*acc{}
	var order []string
	for _, g := range globals {
		if g.Name == "" {
			continue
		}
		a := byName[g.Name]
		if a == nil {
			a = &acc{sels: map[string]bool{}}
			byName[g.Name] = a
			order = append(order, g.Name)
		}
		a.count++
		a.sels[string(g.Selector)] = true
	}
	var out []ValidationError
	for _, name := range order {
		a := byName[name]
		if a.count < 2 || len(a.sels) < 2 {
			continue
		}
		out = append(out, ValidationError{
			Component: name,
			Code:      "merge-collision-component",
			Severity:  SeverityWarning,
			Message: fmt.Sprintf("global component name %q is defined %d times with different selectors; only the first applies to a given node",
				name, a.count),
		})
	}
	return out
}

// definitionNameCollisions warns when a file-root definition shares a name
// with another definition (first by path wins) or with a file-root global
// (the global wins, and stays matched on every view).
func definitionNameCollisions(globals, defs []rawComponent) []ValidationError {
	isGlobal := map[string]bool{}
	for _, g := range globals {
		isGlobal[g.Name] = true
	}
	seen := map[string]bool{}
	var out []ValidationError
	for _, d := range defs {
		if d.Name == "" {
			continue
		}
		switch {
		case isGlobal[d.Name]:
			out = append(out, ValidationError{
				Component: d.Name,
				Code:      "definition-shadowed-by-global",
				Severity:  SeverityWarning,
				Message:   fmt.Sprintf("definition %q has the same name as a global component; $ref resolves to the global, and the definition is unused", d.Name),
			})
		case seen[d.Name]:
			out = append(out, ValidationError{
				Component: d.Name,
				Code:      "merge-collision-definition",
				Severity:  SeverityWarning,
				Message:   fmt.Sprintf("definition name %q is defined more than once; $ref resolves to the first (by source-file path)", d.Name),
			})
		}
		seen[d.Name] = true
	}
	return out
}

// flattenCtx carries the shared state for a flattening pass: the $ref registry
// and any structural diagnostics discovered along the way (currently circular
// $ref chains, which are expanded away and so invisible downstream).
type flattenCtx struct {
	reg         map[string]rawComponent
	diagnostics []ValidationError
	seen        map[string]bool // dedupe key (code + identity) → already-reported
}

// addDiag appends a diagnostic once per distinct key, so a component reused via
// $ref at several sites does not report the same problem repeatedly.
func (ctx *flattenCtx) addDiag(key string, ve ValidationError) {
	if ctx.seen == nil {
		ctx.seen = map[string]bool{}
	}
	if ctx.seen[key] {
		return
	}
	ctx.seen[key] = true
	ctx.diagnostics = append(ctx.diagnostics, ve)
}

// recordCircular records a $ref cycle diagnostic once per distinct chain.
func (ctx *flattenCtx) recordCircular(chain []string) {
	ctx.addDiag("ref-circular\x00"+strings.Join(chain, "\x00"), ValidationError{
		Component: chain[len(chain)-1],
		Code:      "ref-circular",
		Severity:  SeverityError,
		Message:   "circular $ref chain " + strings.Join(chain, " → "),
	})
}

// flattenAll flattens a slice of rawComponents into a flat list of
// ComponentDefs with compound descendant selectors. Order is deterministic and
// stable: components in declaration order, each parent immediately before its
// flattened children (pre-order). Combined with loadDir's lexical file walk this
// gives the corpus a reproducible flattened/wire ordering. originDepth is 0 for
// the file-root globals and -1 for a view's components; see flattenOne.
func flattenAll(rcs []rawComponent, ctx *flattenCtx, originDepth int) []ComponentDef {
	var result []ComponentDef
	for _, rc := range rcs {
		result = append(result, flattenOne(rc, nil, ctx, nil, nil, originDepth)...)
	}
	return result
}

// flattenOne recursively flattens a single rawComponent.
// parentSels holds the already-computed selectors of the nearest ancestor;
// they are prepended (with a space) to every alternative in this component's
// selector. parentChain is the slice of ancestor component names (root-first)
// carried through recursion and stored on each ComponentDef so the
// extension can scope child selectors to their parent's DOM subtree.
// refStack is the chain of $ref names currently being expanded; it guards
// against circular references, which would otherwise recurse forever.
// originDepth is the index in parentChain where the enclosing global instance
// begins, or -1 outside any global; it derives ComponentDef.Origin.
func flattenOne(rc rawComponent, parentSels []string, ctx *flattenCtx, parentChain []string, refStack []string, originDepth int) []ComponentDef {
	// Expand $ref: replace the placeholder with a deep copy of the named global.
	if rc.Ref != "" {
		for _, prev := range refStack {
			if prev == rc.Ref {
				// Cycle: stop expanding rather than recurse forever.
				ctx.recordCircular(append(append([]string(nil), refStack...), rc.Ref))
				return nil
			}
		}
		global, ok := ctx.reg[rc.Ref]
		if !ok {
			// Unresolved $ref: the referenced global does not exist. The spec
			// requires this to be a hard error, so surface it instead of
			// silently dropping the reference.
			ctx.addDiag("ref-unresolved\x00"+rc.Ref, ValidationError{
				Component: rc.Ref,
				Code:      "ref-unresolved",
				Severity:  SeverityError,
				Message:   fmt.Sprintf("$ref %q does not resolve to any global component or definition", rc.Ref),
			})
			return nil
		}
		refStack = append(refStack, rc.Ref)
		rc = global
		originDepth = len(parentChain)
	}

	// A real (non-$ref) component that lacks a required field would otherwise be
	// dropped silently, so it never reaches Validate. Surface it as an error
	// (schema requires both name and selector).
	if rc.Name == "" {
		ctx.addDiag("missing-name\x00"+string(rc.Selector), ValidationError{
			Selector: string(rc.Selector),
			Code:     "missing-name",
			Severity: SeverityError,
			Message:  "component is missing a name",
		})
		return nil
	}
	if string(rc.Selector) == "" {
		ctx.addDiag("missing-selector\x00"+rc.Name, ValidationError{
			Component: rc.Name,
			Code:      "missing-selector",
			Severity:  SeverityError,
			Message:   "component is missing a selector",
		})
		return nil
	}

	rawSels := splitSelectors(string(rc.Selector))
	if len(rawSels) == 0 {
		return nil
	}

	// Combine with parent selectors: for every parent × every child alternative,
	// produce "parent child" (descendant combinator).
	var mySels []string
	if len(parentSels) == 0 {
		mySels = rawSels
	} else {
		mySels = make([]string, 0, len(parentSels)*len(rawSels))
		for _, ps := range parentSels {
			for _, cs := range rawSels {
				mySels = append(mySels, ps+" "+cs)
			}
		}
	}

	result := []ComponentDef{{
		Name:        rc.Name,
		Selectors:   mySels,
		Source:      rc.Source,
		Memory:      rc.Memory,
		Tags:        rc.Tags,
		Watch:       rc.Watch,
		Privacy:     rc.Privacy,
		Properties:  rawPropsToMatch(rc.Properties),
		ParentChain: parentChain, // nil for top-level; omitted from JSON
		Stability:   rc.Stability,
	}}
	if originDepth >= 0 {
		result[0].Origin = strings.Join(append(append([]string(nil), parentChain[originDepth:]...), rc.Name), "\x00")
	}

	// Recurse into children: extend the parent chain with this component's name.
	childChain := append(append([]string(nil), parentChain...), rc.Name)
	for _, child := range rc.Children {
		result = append(result, flattenOne(child, mySels, ctx, childChain, refStack, originDepth)...)
	}

	return result
}

// splitSelectors splits a comma-separated selector string and trims whitespace
// from each alternative, returning only non-empty parts. The split ignores
// commas that are not list separators: those inside a bracket or paren group
// (`[attr="a,b"]`, `:is()`, `:where()`, `:not()`, `:nth-child()`, …) or inside a
// quoted string. A backslash escapes the following character.
func splitSelectors(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	depth, start := 0, 0
	var quote byte // 0 outside a quoted string, else the opening quote char
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' {
			i++ // skip the escaped character
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				if p := strings.TrimSpace(s[start:i]); p != "" {
					parts = append(parts, p)
				}
				start = i + 1
			}
		}
	}
	if p := strings.TrimSpace(s[start:]); p != "" {
		parts = append(parts, p)
	}
	return parts
}
