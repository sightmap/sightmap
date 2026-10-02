---
"@sightmap/sightmap": minor
---

Add named environments and origins (SEP-0014).

A file-root `environments` array defines deploy targets. A web environment maps surface names (`app`, `api`) to the `scheme://host[:port]` URL each surface has there; a native one (`platform: ios | android`) is identified by `app_id` and an optional `build_type`, and borrows a web environment's origins through `backend`. A file-root `origins` map holds shared origins, hosts that are the same in every environment such as a tracking pixel or a vendor endpoint, and an environment's own entry of the same name overrides it. Origin URLs may be patterns, with wildcards only in the leftmost host label (`https://deploy-preview-*--acme.netlify.app`, `https://**.acme.com`) or the port (`http://localhost:*`), so preview deploys and local dev servers belong to an environment without being enumerated.

Views and requests reference both by name through `environments: [...]` and `origins: [...]`. Absent or empty means unconstrained. A view-scoped request's environments are intersected with its view's; its origins are never inherited. The fields are declarative only and take no part in route matching, so adding a list to an existing entity never changes what traffic it matches.

Both registries are project-wide, first file by source path wins. The Go SDK loads the new fields (defaulting `platform` to `web`), exposes `Corpus.EnvironmentByName`, `Corpus.ResolveOrigin`, and `RequestEnvironments`, and validates them with new diagnostics: `environment-invalid`, `origin-invalid`, `environment-backend-invalid`, `environment-ref-unresolved`, `origin-ref-unresolved` (errors), and `environment-name-collision`, `origin-name-collision`, `origin-environment-gap`, `origin-host-shared`, `environment-duplicate`, `environments-empty`, `origins-empty` (warnings).
