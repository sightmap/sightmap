---
"@sightmap/sightmap": patch
---

Reframe the sightmap-authoring skill's "Property extraction principles" around
**purpose**: a property earns its place only as a discriminator (makes an
otherwise-ambiguous instance addressable) or as signal (a value a downstream
event/agent needs). A property that does neither — most commonly a `text`/`label`
that just restates a node's accessible name, or a `text` on a nameless container
that dumps its whole subtree — is noise. Rule 1 now requires a link/button to be
*identifiable* (by its accessible name, or a useful property) rather than
mandating a property (which invited restating-the-name busywork); Rule 2's
per-instance discriminator is preserved, with an explicit carve-out for responsive
duplicates (narrow the selector, don't invent a discriminator). Validated against
the JetBlue corpus (drove ~90% of property decisions cleanly; 21 noise properties
removed with zero tool breakage).
