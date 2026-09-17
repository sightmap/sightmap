# explore benchmarks

Goal suites for `sightmap browser explore --bench`. Each suite is ten goals on
one site with a reset between goals. Runs are reproducible from this directory
with a running session and the keys in the environment (`TYPESAFE_API_KEY`;
`ANTHROPIC_API_KEY` only for `--picker anthropic`).

| suite | site | corpus | what it tests |
|---|---|---|---|
| `saucedemo.json` | saucedemo.com | `saucedemo/.sightmap` (39 components, from the sightkick example) | a mapped site: login, cart, a 12-step checkout, a select, a menu, an error state |
| `books.json` | books.toscrape.com | `books/.sightmap` (empty) | an unmapped site with 114 raw links on the home page: categories, pagination, detail pages |
| `clay.json` | app.clay.com | `sightmap/sites` `app.clay.com` corpus, logged in | a real SaaS app with about 110 candidates per page; carries an `avoid` list |

## Run

```bash
cd go/explore/bench

sightmap browser start --detach --url https://www.saucedemo.com/ --sightmap-dir saucedemo/.sightmap \
  --profile ~/.sightmap/profiles/explore-saucedemo --port 7901 --cdp-port 7902
sightmap browser explore --bench saucedemo.json                       # Jev
sightmap browser explore --bench saucedemo.json --picker anthropic    # Claude in the same seat

sightmap browser start --detach --url https://books.toscrape.com/ --sightmap-dir books/.sightmap \
  --profile ~/.sightmap/profiles/explore-books --port 7911 --cdp-port 7912
sightmap browser explore --bench books.json

# Clay needs a logged-in session on the sites corpus; pass --tab when several tabs are open.
sightmap browser explore --bench clay.json --sightmap-dir ../../../../sites/app.clay.com/.sightmap --tab <id>
```

Saucedemo's published password trips Chrome's breach warning, a native dialog
the page cannot see. Launch the profile once, stop, set
`profile.password_manager_leak_detection: false` in the profile's
`Default/Preferences`, and start again.

Each run writes a JSON file (`--out`) with every step's pick, probabilities,
timings, and the observed transitions.

## Results (2026-09-17, sightmap at this commit)

Same loop for both pickers. The Claude rows are `claude-sonnet-5` asked for the
same pick as a JSON reply, a bare picker rather than a full agent, so read them
as "a big model in the same seat", not as the best a big model can do.

| suite | picker | goals | steps | wall / step | model / call | Anthropic bill |
|---|---|---|---|---|---|---|
| saucedemo | jev-latest | 10/10 | 59 | 0.24 s | 160 ms | $0 |
| saucedemo | claude-sonnet-5 | 10/10 | 59 | 1.16 s | 1,230 ms | $0.25 |
| books (empty corpus) | jev-latest | 10/10 | 23 | 0.30 s | 184 ms | $0 |
| books (empty corpus) | claude-sonnet-5 | 9/10 | 44 | 2.54 s | 1,802 ms | $0.81 |
| clay (logged in) | jev-latest | 10/10 | 28 | 0.48 s | 209 ms | $0 |
| clay (logged in) | claude-sonnet-5 | 10/10 | 27 | 1.66 s | 1,952 ms | $0.42 |

The full run files are in `results/`. The 12-step saucedemo checkout (login,
add to cart, cart, checkout, three fields, continue, finish) took 3.2 s with
Jev. On Clay, "create a new blank table and land in it" took six steps in 3.3 s.
The one Claude miss was a books pagination goal that looped between the logo
and category links until the step cap.

Where a Jev step's time goes on saucedemo: snapshot 7 to 30 ms, Jev 130 to 250
ms, the DOM click or native-setter fill under 50 ms, settle about 120 ms (two
quiet samples 40 ms apart). The model is now most of the step.

Jev token use for whoever prices it: about 1.2k input tokens per call on
saucedemo, 3.7k on the books home page, 4.7k on Clay; output under 200.

## What the numbers do and do not show

- A typed model at about 200 ms is enough to pick every step on a mapped site,
  an unmapped site, and a real SaaS app when the page is offered as named
  actions with properties.
- The goals are short (2 to 12 steps) on cooperative sites. Nothing here needed
  backtracking, a modal that eats clicks, or an infinite-scroll feed.
- Jev never types free text. Every value came from the suite's spec.
- One run per configuration. Treat single-goal differences as noise.

Related work: [browser-use/jev-ultrafast](https://github.com/browser-use/jev-ultrafast)
runs the same idea over a raw element table with an operation head plus target
heads in one request, and a small text model for typed input. This loop
differs in offering sightmap component names and properties as the options,
and in taking typed values from a spec instead of generating them.
