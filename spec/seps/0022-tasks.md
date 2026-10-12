---
sep: 0022
title: Tasks, declared workflows with succeeded, failed and abandoned outcomes
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-10-12
updated: 2026-10-12
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add a top-level `tasks:` collection. A task names a workflow a user (or an agent) sets out to complete, and declares its boundaries from entities the corpus already defines: the condition that **starts** an attempt, and the conditions that end it as **succeeded**, **failed**, or **abandoned**. A consumer evaluating a session stream produces one outcome per attempt, labeled with the condition that closed it. Like [SEP-0007](0007-signals.md) signals, a task never declares a selector, route or body pattern of its own; it only composes components, views, requests, messages and signals by name.

## Motivation

A sightmap names the parts of an app that matter. It cannot yet say what a user is *trying to do* with them, or whether they managed it.

That question, "what share of what people tried to do actually worked?", is the one every team wants answered and almost nobody can. The tools that exist each see one side of it:

- **Server health** sees requests. It reports `200 OK` while the page renders a price of `NaN`, and `202 Accepted` while a background job never finishes.
- **Error tracking** sees exceptions. Most broken workflows throw nothing: a disabled button, an empty state that should not be empty, a toast that lies.
- **Funnels** see steps. "92% reached the submit step" does not say whether submitting worked, and a funnel cannot tell a user who clicked Cancel from one who hit an error, so every exit looks the same.

The missing piece is an **outcome**, and an outcome needs three things no existing field carries: where the attempt begins, what success looks like to the user, and the difference between failing and leaving. Cancel is not failure. A team that alerts on exits pages on-call for normal behavior; a team that ignores exits misses the user who left because the page was broken.

Today, teams that want this write it outside the spec: analytics events fired from application code (`track("invite_sent")`), or hand-written rules in a monitoring tool keyed on CSS selectors and URL patterns. Both drift from the app the moment it changes, both fail silently, and neither can be checked by the tooling that already validates the corpus. The corpus already knows the names of the confirmation banner, the error state, the cancel button and the endpoint. A task is the smallest addition that turns those names into a verifiable outcome.

Because a task is data in the corpus rather than code in the app, the same declaration serves every consumer: an agent executing the workflow in a preview environment uses `succeeded` as its done-predicate, a canary gate compares outcome rates across releases, and production monitoring alerts when the failure rate moves. [End-to-end examples](#end-to-end-examples) walk through each.

## Proposal

### Shape

```yaml
# Before: the corpus names the parts, but not the workflow they form
components:
  - name: InviteModal
    selector: '[role="dialog"][aria-labelledby="invite-title"]'
  - name: InviteSentToast
    selector: '[role="status"].invite-sent'
  - name: SsoRestrictionError
    selector: '.invite-form__error--sso'
  - name: InviteCancelButton
    selector: '[role="dialog"] button[data-action="cancel"]'

# After
tasks:
  - name: InviteTeammate
    description: Invite a teammate by email from the members page.
    tags: [growth]
    start: { ref: InviteModal }                       # the modal became visible
    succeeded: { ref: InviteSentToast }
    failed:
      - { name: sso-restricted, ref: SsoRestrictionError }
    abandoned:
      - { name: cancelled, ref: InviteCancelButton, on: interacted }
      - { name: closed, ref: InviteModal, on: removed }
```

**Task**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Identity of the task. Unique across the corpus. |
| `description` | string | no | What the workflow is, for a human reading the corpus. |
| `tags` | string[] | no | Open-vocabulary labels. See [Tags](../v1/schema.md#tags). |
| `start` | Condition \| Condition[] | yes | Opens an attempt. Any listed condition opens one. |
| `succeeded` | Condition \| Condition[] | yes | Closes the open attempt as `succeeded`. |
| `failed` | Condition \| Condition[] | no | Closes the open attempt as `failed`. |
| `abandoned` | Condition \| Condition[] | no | Closes the open attempt as `abandoned`. |
| `within` | duration | no | Closes an attempt still open this long after it started as `failed`, reason `timeout`. `<integer><unit>`, unit one of `ms`, `s`, `m`, `h`. |

**Condition**

| Field | Type | Required | Description |
|---|---|---|---|
| `ref` | string | yes | Name of a component, view, request, message or signal. |
| `on` | string | no | The event on the referenced entity. Valid values and the default depend on the entity's kind (see [Events](#events)). |
| `filter` | map | no | Constraints on the referenced entity's declared properties at the moment of the event, with [SEP-0007](0007-signals.md) semantics: a scalar is equality, a list is membership, keys are ANDed. |
| `name` | string | no | The reason label reported when this condition closes an attempt. Defaults to `ref`. |

### Events

Each entity kind has a fixed set of events. `on:` omitted means the kind's default.

| Kind | Events | Default |
|---|---|---|
| Component | `visible`: an element became visible to the user. `interacted`: the user clicked, typed into, changed or submitted the element or something inside it. `changed`: a visible element's declared properties took a new set of values, including the first set when it became visible. `removed`: an element left the page. | `visible` |
| View | `entered`: the view's route became active. `left`: it stopped being active. | `entered` |
| Request | `responded`: a matching response arrived. `errored`: the request ended without a response (network failure, abort, transport timeout). | `responded` |
| Message | `logged`: a matching console record or exception was observed. | `logged` |
| Signal | `holds`: the signal's predicate became true. | `holds` |

`filter:` is evaluated against the values the event carries: a component's properties as extracted at that moment, a request's properties and its reserved identity names (`status`, `method`, `duration`), a message's properties. A filter key that does not resolve (an omitted property, a value withheld by [privacy](../v1/schema.md#privacy)) never matches. A view or signal has no properties, so a condition on one takes no `filter:`.

A component referenced by a `visible` or `removed` condition is treated as watched, exactly as if it declared [`watch: true`](0015-component-watch.md), including the rule that a consumer's interactivity heuristic must not suppress it. A task that waits for a confirmation banner must not be defeated by a capture layer that ignores banners because nobody clicks them.

### Semantics

**Attempts.** A consumer evaluates each task over one session's stream of activity, in order. A `start` condition opens an attempt. While an attempt is open, further `start` conditions are ignored: a user dragging a slider ten times is making one attempt, not ten. At most one attempt per task is open at a time.

**First terminal condition wins.** The first `succeeded`, `failed` or `abandoned` condition to fire after the attempt opened closes it, with that outcome. Nothing after the close changes it. This is what makes cleanup states safe to list: `InviteModal` removed is `abandoned` only if the toast has not already closed the attempt as `succeeded`. No `unless:` or `without:` is needed.

**Simultaneous conditions.** When one event satisfies conditions of more than one outcome, `failed` takes precedence over `succeeded`, and both over `abandoned`. A confirmation that renders with a broken price is a failure.

**Implicit closes.** Two reasons are reserved and cannot be used as a condition `name`:

- `timeout`: the attempt was still open `within` after it started. Outcome `failed`. Measured from the start condition, so a task that sets `within` should start on the user's commitment (the click on Export), not on their first touch of the form.
- `session-ended`: the session ended with the attempt open. Outcome `abandoned`. Closing the tab is leaving.

**Retries are new attempts.** An attempt closed as `failed` stays failed. If the user tries again, the next `start` opens a new attempt. A session can therefore hold a failed attempt followed by a succeeded one, which is the record a consumer needs to measure both per-attempt success and eventual success (see [Aggregation](#aggregation-informative)).

**Not every error is a failure.** A condition the author does not list does not close the attempt. A declined card is the product working; leaving it out of `failed:` means the user who tries another card and gets through counts as one succeeded attempt. List under `failed:` what the user experiences as the product breaking, not every intermediate error a retry will absorb.

**Outcome record.** For each closed attempt, a consumer reports at least: the task `name`, the outcome, the reason (the closing condition's `name`, its `ref` when it has none, or a reserved reason), and when the attempt started and closed. The record carries the reason, never the property values a filter compared, so a task adds no path for extracted content to leave the capture layer.

**The actor is not part of the task.** A task describes a workflow, not who performs it. A person, an embedded copilot, an external browser agent and a test harness driving a preview build all produce attempts against the same declaration; a consumer that knows the actor can cut outcomes by it.

### Aggregation (informative)

The spec defines attempts and outcomes. How a consumer aggregates them is its own concern, but the shape is chosen so the common measures fall out directly:

- **Success rate**: `succeeded / (succeeded + failed)`. Abandoned attempts are excluded, so a user choosing to leave does not count against the product.
- **Abandonment rate**: `abandoned / all attempts`, tracked separately. A stall shows up as both rates moving at once (see [Example 4](#4-the-export-that-never-finishes)).
- **Eventual success**: the share of sessions with at least one attempt whose last attempt succeeded. The gap between it and success rate is the cost of retries.
- **Coverage**: the share of sessions in which any declared task started. A success rate is only as representative as the workflows it covers.

### Conformance

A consumer that evaluates tasks MUST:

- Open at most one attempt per task at a time, and ignore `start` conditions while one is open.
- Close the attempt on the first terminal condition, applying `failed` > `succeeded` > `abandoned` when one event satisfies several.
- Close an attempt open longer than `within` as `failed`, reason `timeout`, and an attempt open at session end as `abandoned`, reason `session-ended`.
- Evaluate `filter:` with SEP-0007 semantics; treat an unresolved filter key as not matching.
- Treat a component referenced by a `visible` or `removed` condition as watched.
- Report each closed attempt with task name, outcome, reason, start time and close time, and never include filtered property values in the record.

A tool that does not evaluate sessions, such as an offline matcher or a documentation generator, MUST accept and validate `tasks:` and MAY otherwise ignore it.

Validation diagnostics (reference CLI):

| Code | Severity | When |
|---|---|---|
| `task-ref-unresolved` | error | A condition's `ref` names no component, view, request, message or signal. |
| `task-ref-ambiguous` | error | A `ref` names entities of more than one kind. |
| `task-event-invalid` | error | `on` is not an event of the referenced kind. |
| `task-filter-invalid` | error | A filter key is not a declared property of the referenced entity (or a reserved request identity name), or a `filter` is set on a view or signal. |
| `task-reason-reserved` | error | A condition `name` is `timeout` or `session-ended`. |
| `task-name-duplicate` | error | Two tasks share a name. |
| `task-reason-ambiguous` | warning | Two conditions in one task resolve to the same reason label. |
| `task-no-failure` | warning | A task declares neither `failed` nor `within`, so it can never fail and its success rate is always 100%. |

Fixture `036-tasks` (reserved by this SEP) covers attempt opening, first-wins closing, precedence, `within`, `session-ended`, and each diagnostic above.

### JSON Schema diff

- Root object: add `tasks: {type: array, items: {$ref: "#/$defs/task"}}` (optional). `fileRootFields` gains `"tasks"`, and the shallow-append merge list gains `tasks`.
- `$defs.task` (new): `type: object`, `additionalProperties: false`, `required: [name, start, succeeded]`. Properties: `name` (string, minLength 1), `description` (string), `tags` (array of non-empty strings), `start`, `succeeded`, `failed`, `abandoned` (each `$ref: "#/$defs/taskConditions"`), `within` (string, pattern `^[1-9][0-9]*(ms|s|m|h)$`).
- `$defs.taskConditions` (new): `oneOf: [{$ref: "#/$defs/taskCondition"}, {type: array, items: {$ref: "#/$defs/taskCondition"}, minItems: 1}]`.
- `$defs.taskCondition` (new): `type: object`, `additionalProperties: false`, `required: [ref]`. Properties: `ref` (string, minLength 1), `on` (enum `visible`, `interacted`, `changed`, `removed`, `entered`, `left`, `responded`, `errored`, `logged`, `holds`), `filter` (the same shape as `$defs.signal.filter`), `name` (string, minLength 1). Whether `on` suits the referenced kind is checked by validation, not the schema.

No existing `$defs` entry changes.

## End-to-end examples

Each example is a whole corpus fragment, a session timeline, and what the task reports next to what existing tools report. Entities defined in one example are not repeated in the next.

### 1. The canary that passed every health check

A release refactors the currency formatter. For annual plans in one locale, the per-seat price on the billing page renders as `NaN €`. The subscription endpoint computes prices server-side, so every request still returns `200`. Nothing throws.

```yaml
views:
  - name: BillingSeats
    route: /settings/billing/seats

components:
  - name: SeatSlider
    selector: '[data-testid="seat-slider"]'
  - name: SeatPrice
    selector: '.seat-summary__price'
    properties:
      - name: invalid                       # present only when the price is garbage
        extract: { from: dom.text, pattern: '(NaN|undefined|null)' }
  - name: InvoiceUpdatedConfirmation
    selector: '[role="status"].invoice-updated'

requests:
  - name: UpdateSubscription
    route: /api/billing/subscription
    method: PATCH

tasks:
  - name: ExpandSeatCount
    tags: [billing, revenue]
    start: { ref: SeatSlider, on: interacted }
    succeeded: { ref: InvoiceUpdatedConfirmation }
    failed:
      - name: price-invalid
        ref: SeatPrice
        on: changed
        filter: { invalid: ["NaN", "undefined", "null"] }
      - name: update-rejected
        ref: UpdateSubscription
        filter: { status: ["400", "402", "409", "500"] }
    abandoned:
      - { name: left-page, ref: BillingSeats, on: left }
```

| t | Activity | Task |
|---|---|---|
| 0s | User drags `SeatSlider` from 10 to 14 | attempt opens |
| 0.2s | `SeatPrice` re-renders: `NaN €` | **failed, `price-invalid`** |
| 6s | User navigates away | (attempt already closed) |

Server dashboards are flat. Error tracking is empty. A funnel sees a user who visited the billing page and left, which is indistinguishable from a user who decided not to buy seats. The task records a failure at the instant the user saw the broken price, *before* they left, so first-wins turns what looks like abandonment into the failure it was. Cut by release, `ExpandSeatCount` success drops from 96% to 71% in the canary cohort and the rollout halts on a measure of whether users succeeded, not whether servers responded.

The task needed no regex or comparison operator of its own. The property's `pattern` parses the text, and the filter only compares strings.

### 2. Failing versus leaving

The members page reports that 92% of users who open the invite modal reach the submit step. The `InviteTeammate` task from [Shape](#shape), applied to the same sessions, splits the attempts three ways:

| Outcome | Share | Reason |
|---|---|---|
| succeeded | 41% | `InviteSentToast` |
| failed | 38% | `sso-restricted` |
| abandoned | 21% | `cancelled`, `closed` |

Extending the task with the invite endpoint separates product gaps from defects:

```yaml
requests:
  - name: CreateInvite
    route: /api/orgs/:org/invites
    method: POST

tasks:
  - name: InviteTeammate
    start: { ref: InviteModal }
    succeeded: { ref: InviteSentToast }
    failed:
      - { name: sso-restricted, ref: SsoRestrictionError }
      - { name: invite-error, ref: CreateInvite, filter: { status: ["500", "502", "503"] } }
      - { name: invite-unreachable, ref: CreateInvite, on: errored }
    abandoned:
      - { name: cancelled, ref: InviteCancelButton, on: interacted }
      - { name: closed, ref: InviteModal, on: removed }
```

Nearly all the failures are `sso-restricted`: organizations that enforce single sign-on cannot invite by email. That is a product decision to revisit, routed to the product team by its reason label. On-call is paged only for `invite-error` and `invite-unreachable`. The 21% who cancelled page no one. The `closed` condition fires on every successful invite too, since the modal closes after the toast, but by then the toast has already closed the attempt.

### 3. The toast that lied

An editor publishes optimistically: clicking Publish shows a "Published" toast at once and sends the request in the background. When another editor holds a newer revision, the server answers `409` and the client quietly reverts the post to draft two seconds after the toast fades.

```yaml
components:
  - name: PublishButton
    selector: 'button[data-action="publish"]'
  - name: PublishedToast
    selector: '[role="status"].toast--published'
    watch: true

requests:
  - name: PublishPost
    route: /api/posts/:id/publish
    method: PUT

tasks:
  - name: PublishPost
    tags: [content]
    start: { ref: PublishButton, on: interacted }
    succeeded: { ref: PublishPost, filter: { status: "200" } }
    failed:
      - { name: revision-conflict, ref: PublishPost, filter: { status: "409" } }
      - { name: publish-unreachable, ref: PublishPost, on: errored }
    within: 30s
```

| t | Activity | Task |
|---|---|---|
| 0s | User clicks Publish | attempt opens |
| 0.05s | `PublishedToast` visible | (not a condition) |
| 0.4s | `PublishPost` responds `409` | **failed, `revision-conflict`** |

An analytics event fired from the button's click handler counts this as a publish. A UI-only check sees the success toast. The task anchors success on the server's acceptance, so it reports the failure, and because the toast is watched, every failed attempt's window also shows that the user was told the post was live. The failure clusters in teams with more than three editors. Example 1 is the opposite case, a healthy server behind a broken page. A task can anchor each condition on whichever side tells the truth.

`PublishPost` names both the task and the request. Task names are a separate namespace from entity names: a condition's `ref` never resolves to a task.

### 4. The export that never finishes

Exports are queued: the enqueue endpoint returns `202`, a worker builds the file, and a banner appears when it is ready. An autoscaling change leaves the queue without workers for an hour.

```yaml
components:
  - name: ExportButton
    selector: 'button[data-action="export-csv"]'
  - name: DownloadReadyBanner
    selector: '[role="status"].export-ready'
  - name: ExportFailedBanner
    selector: '[role="alert"].export-failed'
  - name: CancelExportButton
    selector: 'button[data-action="cancel-export"]'

messages:
  - name: ExportPollFailed
    level: ERROR
    message: 'export status poll failed'

tasks:
  - name: ExportReport
    tags: [reporting]
    start: { ref: ExportButton, on: interacted }
    succeeded: { ref: DownloadReadyBanner }
    failed:
      - { name: export-error, ref: ExportFailedBanner }
      - { name: poll-error, ref: ExportPollFailed }
    abandoned:
      - { name: cancelled, ref: CancelExportButton, on: interacted }
    within: 2m
```

No request fails, no message is logged, and no error banner renders. Every existing signal is green. The task records two things at once: users who wait two minutes close as **failed, `timeout`**, and users who give up first close as **abandoned, `session-ended`**. Neither rate alone proves an outage, since some people always close the tab, but both rising together on one task, in the same hour, is the signature of a stall. An alert on that task carries its own impact statement: how many attempts timed out, and links to the sessions.

### 5. A retry is a second attempt

A payment page runs an issuer challenge in an embedded frame. For one issuer, the frame intermittently fails to load.

```yaml
views:
  - name: Cart
    route: /cart

components:
  - name: PlaceOrderButton
    selector: 'button[data-action="place-order"]'
  - name: OrderConfirmation
    selector: '[data-page="order-confirmation"] h1'

messages:
  - name: ChallengeFrameTimeout
    level: ERROR
    message: 'challenge frame (timed out|failed to load)'

requests:
  - name: AuthorizePayment
    route: /api/checkout/authorize
    method: POST
    properties:
      - name: outcome
        extract: { from: rsp.body, path: status }

tasks:
  - name: PlaceOrder
    tags: [checkout, revenue]
    start: { ref: PlaceOrderButton, on: interacted }
    succeeded: { ref: OrderConfirmation }
    failed:
      - { name: challenge-timeout, ref: ChallengeFrameTimeout }
    abandoned:
      - { name: back-to-cart, ref: Cart }
    within: 5m
```

| t | Activity | Task |
|---|---|---|
| 0s | Click Place Order | attempt 1 opens |
| 12s | `ChallengeFrameTimeout` logged | attempt 1 **failed, `challenge-timeout`** |
| 20s | Click Place Order again | attempt 2 opens |
| 21s | `AuthorizePayment` responds, `outcome: declined` | (not a condition) |
| 40s | User enters another card, clicks Place Order | (attempt 2 still open, ignored) |
| 44s | `OrderConfirmation` visible | attempt 2 **succeeded** |

The decline is deliberately absent from `failed:`. It is the product working, and the user who tries another card and gets through made one successful attempt. The frame timeout is listed, because it is the product breaking. Per-attempt success is 88%; eventual success is 97%. The nine-point gap is the friction this issuer's frame costs, isolated by its reason label, while the revenue impact stays small enough that it would never show up as lost orders.

### 6. One declaration, three consumers

`InviteTeammate` from Example 2, unchanged, across the life of a change to the invite modal:

1. **In the pull request.** A coding agent changes the modal's layout and runs the task against a preview build: it drives the browser until the attempt closes, and reports the outcome and reason. `succeeded` is the test's done-predicate. A run that ends `failed, sso-restricted` against a test org with SSO disabled is a regression the agent can read and fix before review. No separate assertions exist to drift from the corpus.
2. **In the canary.** The release ships to 5% of traffic. A rollout gate compares `InviteTeammate` success between the canary and baseline cohorts and promotes or rolls back on it.
3. **In production.** An objective of "98% of `InviteTeammate` attempts that end in success or failure succeed, over a rolling 7 days" replaces an error-rate alert for the members page. The alert fires on user impact, names the reason, and links to the failed sessions.

Cut by actor, the same declaration shows external browser agents abandoning the invite at four times the human rate, nearly all with reason `session-ended`: the submit control is a `div` with a click handler and no role, so an agent cannot find it and gives up. Nothing in the task mentions agents.

## Alternatives considered

### 1. Do nothing: funnels and events in the consumer

Teams keep defining workflows in their analytics or monitoring tool, as ordered steps or as events fired from application code. Ruled out for the reasons in [Motivation](#motivation): a step funnel cannot separate failing from leaving, an event fired from a click handler counts the optimistic success in Example 3, and both drift from the app with no validator to notice. A task declared in the corpus is checked by the same tooling that checks every selector it depends on.

### 2. Ordered steps

A task as a sequence, `steps: [OpenModal, FillForm, Submit, Confirmation]`, each of which must occur in order. Ruled out for v1. Order encodes one path through a workflow, and real users take others: they paste an email before the modal finishes animating, or submit with Enter rather than the button. An outcome depends on where the attempt ended, not the route it took. Where the attempt got to before abandoning is a real question, left to a future SEP (see [Open questions](#open-questions)).

### 3. Imperative instrumentation

An SDK API in application code: `task.start("InviteTeammate")`, `task.succeed()`, `task.fail("sso")`. Ruled out: it requires code in every workflow, ships with every release, and can only measure sessions recorded after it lands. A declaration over named entities can be evaluated against sessions already captured, to the extent the capture recorded the events it references, so a team can declare a task today and see last month's success rate.

### 4. Two outcomes

`succeeded` and `failed` only, with everything unfinished counted as failure. Ruled out: it is the source of the false alarms this SEP exists to remove. A user who clicks Cancel, or closes the tab to come back tomorrow, has not been failed by the product.

### 5. Tasks as signals with a time window

Extend SEP-0007 signals with temporal composition (`A followed by B within 30s`) and express a task as a set of signals. Ruled out: a signal is a boolean, and a task's result is a three-way outcome with a reason and an attempt lifecycle (open, ignore restarts, close once). Encoding that in signal composition would make every consumer reconstruct attempts from booleans. Tasks reference signals instead, so a predicate defined once serves both.

## Migration

`tasks:` is a new top-level key and purely additive. No existing corpus uses it, and a corpus without it behaves exactly as today.

**The additive-field pin applies** (see [`spec/VERSIONING.md`](../VERSIONING.md)). An SDK older than the one shipping this SEP rejects a corpus with `tasks:` under the root's `additionalProperties: false`. Adopters pin to that SDK version before authoring tasks.

**Tasks raise capture volume** for the components they reference. A component referenced by a `visible` or `removed` condition is watched, with the record volume [SEP-0015](0015-component-watch.md) describes. Authors should reference the confirmation and error states a task needs, not every component on the page.

**Retroactive evaluation depends on what was captured.** A consumer can evaluate a new task over old sessions only where those sessions recorded the events the task references. A component that was never watched has no visibility record in sessions captured before the task existed.

Tooling:

- The validator and loader learn `tasks:` and the diagnostics above.
- `fmt` adds `tasks` to the top-level key order, after `signals`, and sorts it by `name`.
- `search` and `stats` include task names and condition refs.

## Open questions

1. **Progress before abandonment.** Where did an abandoned attempt get to? An optional, unordered `milestones:` list of conditions, each recorded when it first fires, would answer it without imposing an order. Deferred to keep this SEP to one decision.
2. **All-of success.** Some workflows succeed only when two things hold, such as a toast *and* a `200`. v1 offers any-of only, and Example 3 anchors success on the side that tells the truth. If real corpora need conjunction, an `all:` form can be added without breaking the single-condition shape.
3. **Concurrent attempts.** Two uploads in parallel are two attempts of one task, which "one open attempt at a time" cannot express. Correlating attempts to instances (by element or by request) would need an identity rule this SEP does not propose.
4. **Status classes.** Examples 1 to 3 list status codes because SEP-0007 filters support only equality and membership. A `5xx`-style class literal for the reserved `status` name would remove the lists, and belongs with SEP-0007's open question on comparison operators.
5. **Cross-session tasks.** A workflow finished from an email link opens a new session. Tasks are scoped to one session; joining attempts across sessions needs an identity a consumer may not have.
6. **Child component refs.** Component names are unique only among siblings. A `ref` resolves the way a signal's `ref:` does; a stable `id` from [SEP-0020](0020-component-id.md) may be the better handle for a nested component.

## References

- [SEP-0007](0007-signals.md): reference-based composition and `filter:` semantics, reused here unchanged.
- [SEP-0015](0015-component-watch.md): visibility reporting, implied for components a task waits on.
- [SEP-0005](0005-request-properties.md), [SEP-0006](0006-message-entity.md): the request and message entities, and their properties, that conditions filter on.
- [SEP-0009](0009-component-privacy.md): privacy governs the properties a filter reads; a withheld value never matches.
- Service level objectives as practiced in site reliability engineering: an objective over a ratio of good events to valid events, with abandoned attempts excluded from the valid set.
