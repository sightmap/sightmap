package sightmap

// Load parses the .sightmap/ corpus at dir and returns the parsed Corpus. It is
// shorthand for DirLoader(dir).Load(). Callers hold the returned *Corpus for the
// life of a run; reloading is just another Load (the old Corpus is discarded).
// For a long-lived process that needs to pick up on-disk edits, call Load again
// and swap the handle. To match a live tree against the corpus, wrap it in a
// Matcher (see NewMatcher).
func Load(dir string) (*Corpus, error) {
	return DirLoader(dir).Load()
}

// GlobalComponentNames returns the set of component names declared at corpus
// scope (components.yaml). Callers use it to exclude globally-shared components
// (Header, Footer, Nav, …) from page-specific checks.
func (c *Corpus) GlobalComponentNames() map[string]bool {
	names := make(map[string]bool, len(c.GlobalComponents))
	for _, gc := range c.GlobalComponents {
		if gc.Name != "" {
			names[gc.Name] = true
		}
	}
	return names
}
