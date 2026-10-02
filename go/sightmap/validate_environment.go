package sightmap

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// These mirror $defs.definitionName, $defs.originUrl, and the environment
// app_id pattern in sightmap.schema.json, so the Go SDK and ajv agree.
var (
	definitionNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	originURLRe      = regexp.MustCompile(`^https?://(\*\*\.|[A-Za-z0-9-]*\*[A-Za-z0-9-]*\.)?[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*(:([1-9][0-9]{0,4}|\*))?$`)
	appIDRe          = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)
)

// checkEnvironments validates SEP-0014 environments, shared origins, and the
// view/request references to them. Name collisions are reported by the loader,
// which is the only place that still sees the dropped definitions.
func checkEnvironments(c *Corpus) []ValidationError {
	var errs []ValidationError
	for _, env := range c.Environments {
		errs = append(errs, checkEnvironmentShape(c, env)...)
	}
	for _, name := range sortedKeys(c.SharedOrigins) {
		errs = append(errs, checkOrigin("", name, c.SharedOrigins[name], "shared origin")...)
	}
	errs = append(errs, checkEnvironmentRefs(c)...)
	errs = append(errs, checkOriginEnvironmentGaps(c)...)
	errs = append(errs, checkSharedOriginHosts(c)...)
	errs = append(errs, checkDuplicateNativeEnvironments(c)...)
	return errs
}

func checkEnvironmentShape(c *Corpus, env EnvironmentDef) []ValidationError {
	var errs []ValidationError
	invalid := func(format string, args ...any) {
		errs = append(errs, ValidationError{
			File:      env.SourceFile,
			Component: env.Name,
			Code:      "environment-invalid",
			Severity:  SeverityError,
			Message:   fmt.Sprintf(format, args...),
		})
	}
	switch {
	case env.Name == "":
		invalid("environment is missing a name")
	case !definitionNameRe.MatchString(env.Name):
		invalid("environment name %q must match ^[a-z][a-z0-9_-]*$", env.Name)
	}

	switch env.Platform {
	case PlatformWeb:
		if len(env.Origins) == 0 {
			invalid("web environment %q must define at least one origin", env.Name)
		}
		var native []string
		if env.AppID != "" {
			native = append(native, "app_id")
		}
		if env.BuildType != "" {
			native = append(native, "build_type")
		}
		if env.Backend != "" {
			native = append(native, "backend")
		}
		if len(native) > 0 {
			invalid("web environment %q must not set %s; set platform: ios or android for a native environment",
				env.Name, strings.Join(native, ", "))
		}
	case PlatformIOS, PlatformAndroid:
		switch {
		case env.AppID == "":
			invalid("%s environment %q must set app_id", env.Platform, env.Name)
		case !appIDRe.MatchString(env.AppID):
			invalid("%s environment %q has app_id %q, which is not a dot-separated identifier", env.Platform, env.Name, env.AppID)
		}
		if env.Backend != "" {
			b := c.EnvironmentByName(env.Backend)
			if b == nil || b.Platform != PlatformWeb {
				reason := "names no environment"
				if b != nil {
					reason = "names a non-web environment, and backends never chain"
				}
				errs = append(errs, ValidationError{
					File:      env.SourceFile,
					Component: env.Name,
					Code:      "environment-backend-invalid",
					Severity:  SeverityError,
					Message:   fmt.Sprintf("environment %q backend %q %s; backend must name a web environment", env.Name, env.Backend, reason),
				})
			}
		}
	default:
		invalid("environment %q has platform %q; must be web, ios, or android", env.Name, env.Platform)
	}

	for _, name := range sortedKeys(env.Origins) {
		errs = append(errs, checkOrigin(env.SourceFile, name, env.Origins[name], fmt.Sprintf("environment %q origin", env.Name))...)
	}
	return errs
}

// checkOrigin reports an origin whose name or URL breaks the origin grammar.
// The schema pattern admits a five-digit port, so the range check lives here.
func checkOrigin(file, name, rawURL, what string) []ValidationError {
	var errs []ValidationError
	invalid := func(format string, args ...any) {
		errs = append(errs, ValidationError{
			File:      file,
			Component: name,
			Code:      "origin-invalid",
			Severity:  SeverityError,
			Message:   fmt.Sprintf(format, args...),
		})
	}
	if !definitionNameRe.MatchString(name) {
		invalid("%s name %q must match ^[a-z][a-z0-9_-]*$", what, name)
	}
	if err := validateOriginURL(rawURL); err != "" {
		invalid("%s %q has URL %q: %s", what, name, rawURL, err)
	}
	return errs
}

// validateOriginURL returns why u is not a valid origin URL, or "" if it is.
func validateOriginURL(u string) string {
	if !originURLRe.MatchString(u) {
		return "must be http(s)://host[:port] with no path, query, or fragment, and wildcards only in the leftmost host label or the port"
	}
	if _, port := splitOriginHostPort(u); port != "" && port != "*" {
		if n, err := strconv.Atoi(port); err != nil || n > 65535 {
			return "port must be 1 to 65535"
		}
	}
	return ""
}

// splitOriginHostPort splits an origin URL that already matches originURLRe.
func splitOriginHostPort(u string) (host, port string) {
	rest := u[strings.Index(u, "://")+3:]
	if i := strings.LastIndexByte(rest, ':'); i >= 0 {
		return rest[:i], rest[i+1:]
	}
	return rest, ""
}

// originMembershipKey is the form SEP-0014 compares origins in: host and port
// only, host lowercased, a port of 80 or 443 dropped, scheme ignored.
func originMembershipKey(u string) string {
	host, port := splitOriginHostPort(u)
	host = strings.ToLower(host)
	if port == "" || port == "80" || port == "443" {
		return host
	}
	return host + ":" + port
}

// checkEnvironmentRefs resolves every view and request reference and warns on
// an explicit empty list, which reads as a constraint but declares none.
func checkEnvironmentRefs(c *Corpus) []ValidationError {
	envNames := map[string]bool{}
	for _, e := range c.Environments {
		envNames[e.Name] = true
	}
	originNames := map[string]bool{}
	for name := range c.SharedOrigins {
		originNames[name] = true
	}
	for _, e := range c.Environments {
		for name := range e.Origins {
			originNames[name] = true
		}
	}

	var errs []ValidationError
	check := func(what, entity string, envs, origins []string) {
		errs = append(errs, checkRefList(what, entity, "environments", envs, envNames, "environment-ref-unresolved", "environment")...)
		errs = append(errs, checkRefList(what, entity, "origins", origins, originNames, "origin-ref-unresolved", "origin")...)
	}
	for _, r := range c.Requests {
		check("request", r.Name, r.Environments, r.Origins)
	}
	for _, v := range c.Views {
		check("view", v.Name, v.Environments, v.Origins)
		for _, r := range v.Requests {
			check("request", r.Name, r.Environments, r.Origins)
		}
	}
	return errs
}

func checkRefList(what, entity, field string, refs []string, defined map[string]bool, code, kind string) []ValidationError {
	if refs != nil && len(refs) == 0 {
		return []ValidationError{{
			Component: entity,
			Code:      field + "-empty",
			Severity:  SeverityWarning,
			Message:   fmt.Sprintf("%s %q declares %s: [], which means the same as omitting it (unconstrained); remove it or list names", what, entity, field),
		}}
	}
	var errs []ValidationError
	seen := map[string]bool{}
	for _, ref := range refs {
		if defined[ref] || seen[ref] {
			continue
		}
		seen[ref] = true
		errs = append(errs, ValidationError{
			Component: entity,
			Code:      code,
			Severity:  SeverityError,
			Message:   fmt.Sprintf("%s %q references %s %q, which is not defined", what, entity, kind, ref),
		})
	}
	return errs
}

func webEnvironments(c *Corpus) []EnvironmentDef {
	var out []EnvironmentDef
	for _, e := range c.Environments {
		if e.Platform == PlatformWeb && e.Name != "" {
			out = append(out, e)
		}
	}
	return out
}

// checkOriginEnvironmentGaps warns when an origin name resolves in some web
// environments but not others: an entity on that surface has no URL in the
// rest. A shared-map name resolves everywhere, so it is never a gap.
func checkOriginEnvironmentGaps(c *Corpus) []ValidationError {
	webs := webEnvironments(c)
	seen := map[string]bool{}
	var names []string
	for _, e := range webs {
		for name := range e.Origins {
			if _, shared := c.SharedOrigins[name]; !shared && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	var errs []ValidationError
	for _, name := range names {
		var missing []string
		for _, e := range webs {
			if _, ok := e.Origins[name]; !ok {
				missing = append(missing, e.Name)
			}
		}
		if len(missing) == 0 {
			continue
		}
		errs = append(errs, ValidationError{
			Component: name,
			Code:      "origin-environment-gap",
			Severity:  SeverityWarning,
			Message: fmt.Sprintf("origin %q resolves in some web environments but not in %s; define it there or in the shared origins map",
				name, strings.Join(missing, ", ")),
		})
	}
	return errs
}

// checkSharedOriginHosts warns when two web environments define the same
// origin. A shared host can never identify a session, which is usually a sign
// it belongs in the shared map.
func checkSharedOriginHosts(c *Corpus) []ValidationError {
	byKey := map[string][]string{}
	var order []string
	for _, e := range webEnvironments(c) {
		for _, name := range sortedKeys(e.Origins) {
			u := e.Origins[name]
			if validateOriginURL(u) != "" {
				continue
			}
			key := originMembershipKey(u)
			envs := byKey[key]
			if len(envs) > 0 && envs[len(envs)-1] == e.Name {
				continue
			}
			if envs == nil {
				order = append(order, key)
			}
			byKey[key] = append(envs, e.Name)
		}
	}
	var errs []ValidationError
	for _, key := range order {
		envs := byKey[key]
		if len(envs) < 2 {
			continue
		}
		errs = append(errs, ValidationError{
			Code:     "origin-host-shared",
			Severity: SeverityWarning,
			Message: fmt.Sprintf("web environments %s all define origin %s, so it identifies no session; a host several environments call belongs in the shared origins map",
				strings.Join(envs, ", "), key),
		})
	}
	return errs
}

// checkDuplicateNativeEnvironments warns when two native environments share
// platform, app_id, and build_type: no session could tell them apart.
func checkDuplicateNativeEnvironments(c *Corpus) []ValidationError {
	byKey := map[string][]string{}
	var order []string
	for _, e := range c.Environments {
		if !e.IsNative() || e.AppID == "" || e.Name == "" {
			continue
		}
		key := e.Platform + "\x00" + e.AppID + "\x00" + e.BuildType
		if _, seen := byKey[key]; !seen {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], e.Name)
	}
	var errs []ValidationError
	for _, key := range order {
		names := byKey[key]
		if len(names) < 2 {
			continue
		}
		parts := strings.SplitN(key, "\x00", 3)
		build := parts[2]
		if build == "" {
			build = "(none)"
		}
		errs = append(errs, ValidationError{
			Component: names[0],
			Code:      "environment-duplicate",
			Severity:  SeverityWarning,
			Message: fmt.Sprintf("environments %s share platform %s, app_id %s, and build_type %s, so they are one target under several names",
				strings.Join(names, ", "), parts[0], parts[1], build),
		})
	}
	return errs
}
