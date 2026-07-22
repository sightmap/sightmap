# 008-first-match-wins

A single `button` node (id `btn`) with both `primary` and `submit` classes. The sightmap defines two components in order: `Primary` (selector `button.primary`) followed by `Submit` (selector `button.submit`). Both selectors would match the node, but the first-match-wins rule means `Primary` claims it and `Submit` receives nothing. This fixture verifies that definition order is respected and that a node matched by an earlier component is never re-assigned to a later one, even when the later selector is equally valid.
