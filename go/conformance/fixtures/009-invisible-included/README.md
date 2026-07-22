# 009-invisible-included

A `div` root with two `button` children: btn1 has `isVisible: false` (and `inViewport: false`), while btn2 has `isVisible: true`. The sightmap defines `Btn` targeting all `button` elements. This fixture verifies that sightmap matching operates on the raw component tree before any visibility filtering — both nodes must appear in the output regardless of their `isVisible` state. Callers that want to hide invisible matches from display should apply that filter after receiving the match result, not before.
