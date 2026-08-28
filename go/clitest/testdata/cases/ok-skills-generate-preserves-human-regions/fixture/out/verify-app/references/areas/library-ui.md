# Library UI

> HAND-AUTHORED SUMMARY: the object library and saved objects

<!-- sightmap:begin sub-features -->
## Sub-features

Views:

- `LibraryView` — "library view", "view" · route `/library`: Object library listing all saved objects

<!-- sightmap:end sub-features -->

## How to get to it (user POV)

HAND-AUTHORED USER POV: users call this "the library".

<!-- sightmap:begin driving -->
## Driving it with sightmap browser

```bash
sightmap browser navigate '<url matching /library>'
sightmap browser wait-for --view LibraryView
sightmap snapshot --coverage
```

Address components by **query** (shown above), never by a raw CSS selector or a probe ID from an earlier snapshot.

<!-- sightmap:end driving -->

## Gotchas

HAND-AUTHORED GOTCHA: BulkActionTable also exists in dqm-ui.
