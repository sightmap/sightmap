# Android sample

A small shop app for exercising `sightmap device` on Android: the same
screens built with Jetpack Compose and with classic views, so a corpus can be
authored and checked against both. Package `dev.sightmap.sample`.

| Screen | Built with | What it has |
|---|---|---|
| Shop (bottom nav "Shop") | Compose, `testTagsAsResourceId` | header, search field, segmented filter, a list of rows with title, brand and price texts (`product.1`, …), a bottom sheet per product |
| Plain | Compose, no `testTagsAsResourceId` | the same Shop screen without identifiers |
| Settings | Compose, `testTagsAsResourceId` | tab row, a whole-row toggle, a standalone `Switch`, a checkbox row, a sign-out button and dialog |
| Views | classic views (Material Components) | toolbar, `TextInputLayout`, `MaterialButtonToggleGroup`, `MaterialSwitch`, a `RecyclerView` of rows, `BottomNavigationView` |

The `uiautomator-compose*.xml` and `uiautomator-views-shop.xml` fixtures are
dumps of these screens.

```bash
./build.sh                      # prints the .apk path
sightmap device start --app "$(./build.sh)" --detach
```

Needs JDK 17 and the Android SDK; see `build.sh`.
