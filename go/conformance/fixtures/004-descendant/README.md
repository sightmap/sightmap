# 004-descendant

A five-level tree: root → nav.main-nav → ul → li (×2) → a.nav-link. The sightmap uses the selector `nav.main-nav a.nav-link`, which requires the anchor to be a descendant (at any depth) of the nav. This fixture verifies that the descendant combinator correctly bridges multiple intermediate nodes (`ul` and `li`) that are not mentioned in the selector. Both anchors (a1 and a2) must appear in the output.
