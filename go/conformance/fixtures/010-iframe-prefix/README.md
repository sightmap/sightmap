# 010-iframe-prefix

A component tree whose node IDs carry frame-prefix notation: the root is `0_root` (main frame), the nav is `1_nav`, and the two anchor links are `1_5` and `1_6` (both in frame 1). The sightmap selector `nav a.nav-link` is structurally identical to the one in 004-descendant. This fixture verifies that the NFA traversal is unaffected by frame-prefixed IDs — the matcher treats them as ordinary opaque string identifiers and traverses into those subtrees normally. Both 1_5 and 1_6 must appear in the output.
