package probe

import _ "embed"

// ProbeJS is the canonical cdp-probe.js source, ready to be evaluated in a
// browser context via CDP Runtime.evaluate. Downstream consumers embed this
// variable so extraction logic cannot diverge.
//
//go:embed probe.js
var ProbeJS string
