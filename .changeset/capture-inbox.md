---
"@sightmap/sightmap": minor
---

Capture inbox: `sightmap capture --inbox` keeps a page in `.sightmap/inbox/<stamp>.snap`, with its tree and a full-page screenshot, without matching a view or applying the novelty gate. This lets a tool that learns views from captures queue them first and sort them later. The overlay side panel has a **Capture** button (and `Alt+Shift+C`) that writes the tab you are looking at to the same inbox, through a new `POST /capture` endpoint on the `browser start` daemon.
