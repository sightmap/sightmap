package webmcp

// Small accessors over the ordered manifest/IR values, for the CLI layer.

// ManifestSightmapDir returns the manifest's "sightmap:" field, if any.
func ManifestSightmapDir(manifest any) string {
	return asString(omGet(manifest, "sightmap"))
}

// SiteOf returns the compiled IR's site slug.
func SiteOf(ir *OM) string {
	return asString(omGet(metaOf(ir), "site"))
}

// ToolNames returns the compiled IR's tool names, in order.
func ToolNames(ir *OM) []string {
	var names []string
	for _, t := range asList(omGet(ir, "tools")) {
		names = append(names, asString(omGet(asOM(t), "name")))
	}
	return names
}
