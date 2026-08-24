# Signals — worked examples / semantics test-bed

Draft working notes on the `proposal/range-signals` branch.

This catalog is the shared ground truth for the range-signals design. Every proposed
semantic rule must be checked against these traces. Syntax is illustrative, not
final — we're pinning *behavior*, not spelling. Grammar sketch lives beside this
in `range-signals-grammar.md`.

---

## Working operational model (v1 draft) — "continuous eval, emit-once"

A **rule** is:

- an ordered list of **bindings** — each a per-input predicate + a quantifier
  (`one` default / `+` one-or-more / `*` zero-or-more / `?` optional);
- an optional `within: W` window;
- an optional `until:` **abort** binding;
- a `where:` guard — a boolean over the bindings' attrs/aggregates;
- an `emit:` projection.

**Domain (the robustness invariant).** An input is visible to a rule **iff it
matches one of that rule's bindings** (including `until`). Any other input is
invisible: it cannot open, advance, or close an instance. A stray console log
never breaks a run — the only way to make an input matter is to *bind* it.

**Match-instance lifecycle.**
1. **Open** at the first input matching a binding → sets `t0`.
2. **Accumulate** later matching inputs (skip-till: unrelated inputs ignored).
3. **Evaluate `where`** after *every consumed input*, and again at a synthetic
   tick on `within` expiry and at session end.
4. **Emit-once:** the first time `where` is true, emit and close the instance.
5. If `until` matches first, **abort** — close without emitting, and allow a
   fresh instance to open (this *segments* runs; it changes grouping, not just
   latency).
6. If `within`/session-end is reached with `where` never true, discard.

There is **no privileged "terminal" binding.** "Fire when X arrives" is just
`where: ... and exists X` — X is the input that flips `where`. "Fire at the
window" is `where: elapsed and ...`. The *timing* is entirely a consequence of
what `where` references; there are no emission modes to choose between.

**Roles, disentangled** (the thing that bit EX4):
- *closer-that-emits* → a plain binding referenced in `where` (area's `away`).
- *closer-that-aborts* → `until:` (rage's stray click).
- *plain evidence* → a binding referenced in `where` (`decline`), nothing special.

**Two emit strategies** (EX1 vs EX4):
- *tag-in-place* — add tags/attrs onto an existing matched input; no new signal.
- *mint* — emit a new derived signal (a higher-stratum stream element), carrying
  a span `[t0, t1)`.
Default: single-input match → tag-in-place; multi-input match → mint-a-span.

---

## EX1 — point / tag-in-place (single binding)

```yaml
- name: checkout.payment.declined
  match:
    r: network where name == CheckoutPayment and status == 200 and outcome == declined
  emit: { tags: [defect] }        # default: tag r in place
```

Trace: `… network(CheckoutPayment,200,declined)@12 …`
`where` is implicitly the binding's own match; true at @12 → the existing
`network@12` gains tag `defect`. **No new signal.** Volume: zero new elements —
this is the answer to "won't we emit a ton": point classifications tag, not mint.

---

## EX2 — evidence flips `where` (rage → decline)

```yaml
- name: checkout.rage_declined
  within: 10s
  match:
    clicks:  click+  where name has PayButton
    decline: network where name == CheckoutPay and status == 200 and outcome == declined
  where: clicks.count >= 3 and exists decline
  emit:
    tags: [defect, rage]
    attrs: { attempts: clicks.count, amount: decline.amount }
```

Trace A: `click(Pay)@1, click(Pay)@2, console.log@2.5, click(Pay)@3, network(CheckoutPay,200,declined)@4`
- open@1; log@2.5 invisible; after @1/@2/@3 `where` is false (no decline);
  `decline@4` flips `where` true → **emit@4, span[1,4], attempts=3**.

Trace B (2 clicks): completes only at `within`=10s (where never true) → **no emit**.

`decline` is not a terminal — it's just the input that makes `where` true.

---

## EX3 — abort via `until` (abandoned rage)

Add to EX2:
```yaml
  until: click where not name has PayButton
```

Trace: `click(Pay)@1, click(Pay)@2, click(Pay)@3, click(OtherButton)@3.5`
- `until` matches@3.5 → **abort, no emit** (and a fresh instance may open at the
  next PayButton click). `where` never got its `exists decline`.

Logic check on the earlier `where: clicks.count >= 3 or other`: that was a bug —
it put the abort trigger *inside* the emit gate, so a lone stray click would
fire. Abort belongs in `until:`; the emit gate stays `where`. And `until` here
also *segments*: without it, a later decline could group clicks from before and
after the stray click into one `attempts`.

---

## EX4 — span (area.auth), closer EMITS → a `where` binding, NOT `until`

```yaml
- name: area.auth
  match:
    body: navigate+ where name in [LoginPage, VerifyDevicePage]
    away: navigate  where not name in [LoginPage, VerifyDevicePage]
  where: body.count >= 1 and exists away
  emit:
    tags: [area:auth]
    attrs: { span: [body.first.when, body.last.when] }
```

Trace: `navigate(MarketingHome)@0, navigate(AppHome)@5, navigate(LoginPage)@6, click@7, navigate(VerifyDevicePage)@9, navigate(AppDashboard)@12`
- @0,@5 match `away` but no instance open → ignored.
- `LoginPage@6` opens `body`; `click@7` invisible; `VerifyDevicePage@9` extends
  body; after each, `where` false (no `away` yet); `AppDashboard@12` matches
  `away` → `where` true → **emit, span[6,9]**.

**Correction from an earlier draft:** the closer here is `away` (a `where`
binding), NOT `until`. If it were `until` it would *abort* and the area would
never emit. This is the crisp rule: closer-that-emits ⇒ bind it and reference in
`where`; closer-that-aborts ⇒ `until`.

Two times: **span end = 9** (last member view) vs **completion = 12** (when we
learn it ended). Logical `when` of the emitted span = span end (9) — confirmed
intuition; streaming realizability deferred.

Rendering (out of spec): nest any signal whose `when ∈ span` under the area line
— associating the area with intervening signals by time, without stamping them.

---

## EX5 — count in a window: prompt vs final (the `elapsed` knob)

```yaml
- name: rage.paybutton
  within: 10s
  match:
    clicks: click+ where name has PayButton
  where: clicks.count >= 3            # PROMPT: fires at the 3rd click
  emit: { tags: [rage], attrs: { attempts: clicks.count } }
```

`count>=3` is monotone → `where` flips true at the 3rd click → **emit@3rd,
attempts=3**. If you instead want the *final* count over the window, gate on the
window close:

```yaml
  where: elapsed and clicks.count >= 3   # FINAL: fires at t0+10s, attempts=all
```

Same emit-once machinery; the only difference is whether `where` references
`elapsed`. This is the "can't emit until 10s" case — true only when you *ask* for
it via `elapsed`.

---

## EX6 — bounded absence ("submit not followed by success within 5s")

```yaml
- name: login.submit_no_response
  within: 5s
  match:
    submit:  change  where name has PasswordField
    success: network where name == Login and status == 200
  where: elapsed and not exists success     # elapsed is REQUIRED here
  emit: { tags: [friction] }
```

Trace A: `change(PasswordField)@1, network(Login,200)@2`
- @1: `where` = elapsed(false) → false. `success@2` arrives; still not elapsed →
  false. Window@6: elapsed true but `exists success` → **no emit**.

Trace B: `change(PasswordField)@1`, then silence
- @1: not elapsed → false. Window@6: elapsed and not exists success → **emit@6**.

Absence *needs* `elapsed` — without it `not exists success` is true the instant
`submit` opens (success hasn't happened yet) and would misfire at @1. Unbounded
/ session absence still wants a `session-end` primitive or a consumer fold —
an open item (see the absence/negation discussion).

---

## Cross-cutting decisions surfaced by these examples

- **Continuous-eval + emit-once** replaces the earlier "complete-then-eval"
  framing; no terminal role, no emission modes.
- **`until` = abort+segment; closer-that-emits = a `where` binding.**
- **Lifecycle predicate `elapsed`/`closed`** needed for absence (EX6) and
  final-aggregate (EX5). Candidate: `session.ended`.
- **`exists`/`some` unary over a binding** — EX2/EX4/EX6.
- **Ordered vs unordered `match:`** — order only matters where `where` uses
  `.when`; otherwise a set. Lean: a set, order via `.when`. Confirm.
- **Emit strategy (tag vs mint) default by arity.**
- **Overlap/start policy** — leftmost, non-overlapping; `until` re-arms.
- **`when` of a minted span = span-end, not completion instant.**
