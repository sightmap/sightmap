package sightmap

// Platform values for EnvironmentDef.Platform (SEP-0014).
const (
	PlatformWeb     = "web"
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
)

// EnvironmentDef is a named deploy target (SEP-0014). A web environment is
// identified by its Origins; a native one by AppID and BuildType, borrowing a
// web environment's origins through Backend. Environments are declarative
// metadata: nothing in route matching reads them.
type EnvironmentDef struct {
	Name string `json:"name"`
	// Platform is always set after loading: the loader supplies the schema's
	// "web" default itself, since schema validators do not apply defaults.
	Platform  string `json:"platform"`
	AppID     string `json:"app_id,omitempty"`
	BuildType string `json:"build_type,omitempty"`
	Backend   string `json:"backend,omitempty"`
	// Origins maps a surface name to its origin URL in this environment.
	Origins map[string]string `json:"origins,omitempty"`

	SourceFile string `json:"-"` // source YAML filename, for diagnostics
}

// IsNative reports whether e is an iOS or Android environment. An unknown
// platform is neither web nor native; validation reports it.
func (e EnvironmentDef) IsNative() bool {
	return e.Platform == PlatformIOS || e.Platform == PlatformAndroid
}

// EnvironmentByName returns the registered environment with the given name, or
// nil. The registry already holds only the first definition of each name.
func (c *Corpus) EnvironmentByName(name string) *EnvironmentDef {
	for i := range c.Environments {
		if c.Environments[i].Name == name {
			return &c.Environments[i]
		}
	}
	return nil
}

// ResolveOrigin returns the URL origin resolves to in the named environment:
// the environment's own origins, then (native only) its backend's, then the
// shared map, each overriding the ones after it. ok is false when the
// environment is unknown or the name resolves nowhere in it.
func (c *Corpus) ResolveOrigin(environment, origin string) (originURL string, ok bool) {
	env := c.EnvironmentByName(environment)
	if env == nil {
		return "", false
	}
	if u, ok := env.Origins[origin]; ok {
		return u, true
	}
	if env.IsNative() && env.Backend != "" {
		// A native backend is invalid and never chains, so it contributes nothing.
		if b := c.EnvironmentByName(env.Backend); b != nil && !b.IsNative() {
			if u, ok := b.Origins[origin]; ok {
				return u, true
			}
		}
	}
	u, ok := c.SharedOrigins[origin]
	return u, ok
}

// RequestEnvironments returns the environments a request exists in. view is
// the request's enclosing view, or nil for a global request. A view-scoped
// request exists only where its view does, so its own list is intersected with
// the view's; an absent or empty list on either side adds no constraint.
// constrained is false when the request exists in every environment; a
// constrained result can be empty when the two lists are disjoint.
func RequestEnvironments(view *ViewDef, r RequestDef) (envs []string, constrained bool) {
	var outer []string
	if view != nil {
		outer = view.Environments
	}
	switch {
	case len(r.Environments) == 0 && len(outer) == 0:
		return nil, false
	case len(outer) == 0:
		return dedupeStrings(r.Environments), true
	case len(r.Environments) == 0:
		return dedupeStrings(outer), true
	}
	inOuter := make(map[string]bool, len(outer))
	for _, e := range outer {
		inOuter[e] = true
	}
	out := []string{}
	for _, e := range dedupeStrings(r.Environments) {
		if inOuter[e] {
			out = append(out, e)
		}
	}
	return out, true
}

// dedupeStrings returns ss without repeats, keeping first occurrences in order.
func dedupeStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
