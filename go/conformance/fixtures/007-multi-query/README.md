# 007-multi-query

A tree containing a `nav` subtree (with a nav-link anchor) and a `form.search` subtree (with an email input). The sightmap defines two components simultaneously: `NavLink` targeting `nav a.nav-link` and `SearchInput` targeting `[type="email"]`. This fixture verifies that two independent queries can be evaluated in a single `ApplySightmap` pass with no state leakage between them — each query matches exactly its own node (a1 and inp1 respectively) and neither interferes with the other.
