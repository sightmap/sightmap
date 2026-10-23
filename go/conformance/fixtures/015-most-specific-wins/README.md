# 015-most-specific-wins

A node claimed by several definitions goes to the strictly most specific one,
whatever the declaration order (SEP-0023). The broad definitions are declared
first here, so first-match-wins alone would hand them every node:

| Node | Claimants | Named | Why |
|---|---|---|---|
| `plain` | `Text` | `Text` | only claimant |
| `label` | `Text`, `Label` | `Label` | `span.label` implies `span` |
| `price` | `Text`, `Price` | `Price` | the attribute tests imply the bare tag |
| `total` | `Text`, `Price`, `TotalPrice` | `TotalPrice` | `$="-total-price"` implies `$="-price"` |
| `cardbtn` | `Button`, `CardButton` | `CardButton` | a chain with an extra ancestor compound implies the bare `button` |
| `both` | `Primary`, `Submit` | `Primary` | `button.primary` and `button.submit` are not ordered, so the first declared wins and the node is a conflict |

Specificity is decided from the selector text alone: what a selector requires
first (every simple selector of the less specific compound is implied by one of
the more specific's, chain embedded in chain from the subject outward), what it
excludes second. Every implementation therefore orders the same pairs.
