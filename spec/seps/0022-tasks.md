---
sep: 0022
title: Tasks, declared workflows with steps and succeeded, failed or abandoned outcomes
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-10-12
updated: 2026-10-12
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add a top-level `tasks:` collection. A task names a workflow a user (or an agent) sets out to complete, as an ordered list of **steps**, plus the conditions that mean the attempt **failed** and the conditions that mean the user **abandoned** it. A consumer evaluating a session produces one record per attempt: succeeded, failed or abandoned, with the furthest step reached and the reasons for any failure. Every condition references a component, view, request, message or signal the corpus already declares, so a task adds no selector, route or body pattern of its own.

## Motivation

A sightmap names the parts of an app that matter. It cannot yet say what a user is trying to do with them, or whether they managed it.

The usual answer is a **funnel**: an ordered list of steps, and the share of users who reach each one. A funnel shows where people stop. It cannot show why. A user who left because the page broke and a user who changed their mind are the same drop-off, so conversion moves with intent, price, traffic mix and season as much as with defects. That is why a funnel is something people look at, not something that pages anyone: an alert on conversion rate fires on a quiet Tuesday and stays silent through a broken checkout that happens to coincide with a sale.

The other tools each see one side of a broken workflow:

- **Server health** sees requests. It reports `200 OK` while the page renders a price of `NaN`.
- **Error tracking** sees exceptions. Many broken workflows throw nothing: a disabled button, a swallowed rejection, a confirmation page that redirects away.
- **Statistical drop-off analysis** compares conversion between users who saw a signal and users who did not. It finds correlations across a population. It cannot say why one attempt failed, and it cannot see a failure that emits no signal it already tracks.

What is missing is a per-attempt **outcome** with three values, because leaving is not failing:

- **succeeded**: the user reached the end of the workflow.
- **failed**: the product broke on the way, for a declared reason.
- **abandoned**: the user stopped, and nothing broke.

With abandonment separated out, `succeeded / (succeeded + failed)` moves when the product breaks and not when people change their minds. That ratio can carry an alert, an objective or a rollout gate. The steps keep everything a funnel offers, since the furthest step reached is recorded for every attempt.

Teams that want this today write it outside the spec, as analytics events fired from application code or as rules in a monitoring tool keyed on CSS selectors and URL patterns. Both drift from the app when it changes, both fail silently, and neither is checked by the tooling that validates the corpus. The corpus already names the confirmation, the error state, the cancel button and the endpoint. A task is the smallest addition that turns those names into an outcome.

## Proposal

### Shape

```yaml
# Before: the corpus names the parts, but not the workflow they form
views:
  - name: Checkout
    route: /checkout
components:
  - name: OrderNumber
    selector: '.confirmation-order-id'
  - name: EmptyCartMessage
    selector: '.cart-empty'
requests:
  - name: AddToCart
    route: /api/cart
    method: POST
  - name: PlaceOrder
    route: /api/orders
    method: POST

# After
tasks:
  - name: OrderFood
    description: Add food to the cart, check out, and see a confirmation.
    tags: [revenue]
    steps:
      - { name: added, ref: AddToCart, filter: { status: "200" } }
      - { name: checkout, ref: Checkout }
      - { name: placed, ref: PlaceOrder, filter: { status: "200" } }
      - { name: confirmed, ref: OrderNumber }
    failed:
      - { name: order-error, ref: PlaceOrder, filter: { status: ["422", "500"] } }
    abandoned:
      - { name: emptied-cart, ref: EmptyCartMessage }
```

**Task**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Identity of the task. Unique across the corpus. |
| `description` | string | no | What the workflow is, for a human reading the corpus. |
| `tags` | string[] | no | Open-vocabulary labels. See [Tags](../v1/schema.md#tags). |
| `steps` | Condition[] | yes | The workflow's milestones, in order. At least two. Reaching the last step is success. |
| `failed` | Condition[] | no | Conditions that record a failure on the open attempt. |
| `abandoned` | Condition[] | no | Conditions that close the open attempt as abandoned (or as failed, when a failure was recorded). |
| `within` | duration | no | An attempt still open this long after it opened closes as `failed`, label `timeout`. `<integer><unit>`, unit one of `ms`, `s`, `m`, `h`. |

**Condition**

| Field | Type | Required | Description |
|---|---|---|---|
| `ref` | string | yes | Name of a component, view, request, message or signal. See [Resolving `ref`](#resolving-ref). |
| `on` | string | no | The event on the referenced entity. Valid values and the default depend on its kind. See [Events](#events). |
| `filter` | map | no | Constraints on the values the event carries. See [Filters](#filters). |
| `name` | string | no | The label this condition is reported under. Defaults to `ref`. Unique within the task. |

### Events

Each entity kind has a fixed set of events. `on` omitted means the kind's default.

| Kind | Events | Default |
|---|---|---|
| Component | `visible`: an element became visible to the user. `interacted`: the user clicked, typed into, changed or submitted the element or something inside it. `changed`: a visible element's declared properties took a new set of values, including the first set when it became visible. `removed`: an element left the page. | `visible` |
| View | `entered`: the view's route became active. `left`: it stopped being active. | `entered` |
| Request | `responded`: a matching response arrived. `errored`: the request ended without a response (network failure, abort, transport timeout). | `responded` |
| Message | `logged`: a matching console record or exception was observed. | `logged` |
| Signal | `holds`: the signal's predicate became true. | `holds` |

A component referenced with `visible` or `removed` is treated as watched, exactly as if it declared [`watch: true`](0015-component-watch.md), including the rule that a consumer's interactivity heuristic must not suppress it. A task that waits for a confirmation must not be defeated by a capture layer that ignores confirmations because nobody clicks them.

### Filters

A `filter` is a map of property name to a string (equality) or a non-empty list of strings (membership). Keys are ANDed. Values are compared as strings. This is the `filter:` shape [SEP-0007](0007-signals.md) proposes for signals, defined here so tasks do not depend on that draft. If SEP-0007 lands, the two are the same construct.

A filter key names a property declared on the referenced entity: a component's [properties](../v1/schema.md#component-properties) as extracted at the moment of the event, a request's [properties](../v1/schema.md#request-properties), or a message's [properties](../v1/schema.md#message-properties). A request may also be filtered on its reserved identity names `status`, `method` and `duration`, with no declaration; `status` compares as its decimal string (`"402"`). A view or signal carries no values, so a condition on one takes no `filter`.

A key that does not resolve never matches: an omitted property, a pattern that did not match, a body the capture did not record, or a value withheld by [privacy](../v1/schema.md#privacy). A filter that reads withheld content therefore never fires. For a component property, the existing `extract-privacy-withheld` warning already flags it.

### Resolving `ref`

A `ref` names a view, a request, a message, a signal, or a component declared at file root (`components:`) or at the top level of a view. A component name declared in several of those places refers to all of them. Components nested under `children:` are not addressable, the same restriction signals have today (see [Open questions](#open-questions)). A name that matches more than one kind is ambiguous and rejected.

Task names are a separate namespace. A task may share a name with an entity, and a `ref` never resolves to a task.

### Semantics

A consumer evaluates each task over one session's activity, in order.

**Opening.** When no attempt of the task is open, any step **other than the last** opens one, with that step as its entry and furthest step. An attempt can therefore start mid-workflow: a returning user with a saved cart, a deep link, a back navigation. Reaching the last step with no attempt open (reloading a confirmation page) opens nothing. At most one attempt per task is open at a time.

**Progress.** While an attempt is open, a step is reached when its condition fires and it comes later in the list than the furthest step reached so far. Earlier steps, and the furthest step itself, are ignored when they fire again. A step may be skipped: a capture that misses one view transition does not stall an attempt that went on to finish.

**Failures are recorded, not final.** A `failed` condition that fires while an attempt is open appends its label to the attempt's failures, every time it fires, and the attempt stays open. Users retry. An attempt that fails once and then reaches the last step succeeded, and its record says what it cost to get there. A `failed` condition that fires with no attempt open is ignored.

**Closing.** An open attempt closes on the first of:

| Closes on | Outcome | `closed_by` |
|---|---|---|
| The last step is reached | `succeeded` | the last step's label |
| An `abandoned` condition fires | `failed` if any failure was recorded, else `abandoned` | that condition's label |
| `within` elapses since the attempt opened | `failed` | `timeout` |
| The session ends | `failed` if any failure was recorded, else `abandoned` | `session-ended` |

A user who hits a declared failure and then leaves was failed by the product, so the attempt is `failed`, not `abandoned`. An abandonment condition only describes how the user left.

**Simultaneous conditions.** When conditions in more than one list fire on the same event, or on events at the same instant, a consumer applies `failed` first, then `steps`, then `abandoned`. A navigation's `left` and `entered` events are simultaneous. So an attempt that abandons on leaving checkout and succeeds on entering the confirmation view succeeds when checkout hands off to confirmation, and an event that both fails and completes an attempt leaves it succeeded but not clean (see [Aggregation](#aggregation-informative)).

**Reserved labels.** `timeout` and `session-ended` are reserved and cannot be a condition's label.

**`within` starts at the opening step.** A task that sets `within` should open at the user's commitment (the click on Export), not at their first touch of the workflow. On a task that opens when an item is added to a cart, `within` would turn every slow shopper into a failure.

**Only declared failures count.** A condition the author does not list does not fail the attempt. A declined card that the page explains, and that the user gets past with another card, is the product working; leaving it out of `failed` keeps it from counting against the product. The cost is the converse: a failure nobody declared reads as abandonment. Abandonment rate is worth watching for that reason.

**The record.** For each closed attempt, a consumer reports at least: task `name`, `outcome`, `closed_by`, `entry_step`, `furthest_step`, `failures` (the labels in the order they fired, repeats kept), and the open and close times. The record never carries the property values a filter compared, so a task adds no path for extracted content to leave the capture layer.

**The actor is not part of the task.** A person, an embedded assistant, an external browser agent and a test harness driving a preview build all produce attempts against the same declaration. A consumer that knows the actor can cut outcomes by it.

### Aggregation (informative)

The spec defines attempts and their records. Aggregation belongs to the consumer, but the record is shaped so the common measures fall out directly:

- **Reliability**: attempts that succeeded with no recorded failure, over `succeeded + failed`. Abandoned attempts are excluded, so users leaving by choice do not move it. This is the measure to alert on, set an objective on, or gate a rollout with.
- **Completion**: `succeeded / (succeeded + failed)`. The gap between completion and reliability is the share of attempts that got through only after a failure.
- **Abandonment**: `abandoned / all attempts`. It rises with undeclared failures as well as with lost interest.
- **Step reach**: the share of attempts whose furthest step is at or beyond each step. This is the funnel, computed from attempts.
- **Coverage**: the share of sessions in which any declared task opened. A reliability figure is only as representative as the workflows it covers.

### Conformance

A consumer that evaluates tasks MUST:

- Open an attempt only on a step other than the last, and only when none of that task is open.
- Advance only to a step later than the furthest reached, allowing skipped steps.
- Record every `failed` condition that fires while the attempt is open, without closing it.
- Close the attempt on the first of: the last step reached, an `abandoned` condition, `within` elapsed, the session ending; with the outcomes in [Closing](#semantics).
- Apply `failed`, then `steps`, then `abandoned` to conditions firing at the same instant, treating a navigation's `left` and `entered` as simultaneous.
- Evaluate filters as in [Filters](#filters), treating an unresolved key as not matching.
- Treat a component referenced with `visible` or `removed` as watched.
- Report each closed attempt with the fields in [The record](#semantics), and never include filtered property values.

A tool that does not evaluate sessions, such as an offline matcher or a documentation generator, MUST accept and validate `tasks:` and MAY otherwise ignore it.

Validation diagnostics (reference CLI):

| Code | Severity | When |
|---|---|---|
| `task-name-duplicate` | error | Two tasks share a name. |
| `task-ref-unresolved` | error | A `ref` names no addressable component, view, request, message or signal. |
| `task-ref-ambiguous` | error | A `ref` names entities of more than one kind. |
| `task-event-invalid` | error | `on` is not an event of the referenced kind. |
| `task-filter-invalid` | error | A filter key is neither a declared property of the referenced entity nor a reserved request identity name, or a `filter` is set on a view or signal. |
| `task-label-duplicate` | error | Two conditions in one task resolve to the same label (their `name`, or their `ref` when unnamed). |
| `task-label-reserved` | error | A condition's label is `timeout` or `session-ended`. |
| `task-no-failure` | warning | A task declares neither `failed` nor `within`, so it can never fail and its reliability is always 100%. |

Fixture `036-tasks` (reserved by this SEP) covers opening mid-workflow, skipped steps, recorded failures followed by success, each closing rule, the precedence order, and each diagnostic above.

### JSON Schema diff

- Root object: add `tasks: {type: array, items: {$ref: "#/$defs/task"}}` (optional). `fileRootFields` gains `"tasks"`, and the shallow-append merge list gains `tasks`.
- `$defs.task` (new): `type: object`, `additionalProperties: false`, `required: [name, steps]`. Properties: `name` (string, minLength 1), `description` (string), `tags` (array of non-empty strings), `steps` (array of `$ref: "#/$defs/taskCondition"`, minItems 2), `failed` and `abandoned` (each an array of `$ref: "#/$defs/taskCondition"`, minItems 1), `within` (string, pattern `^[1-9][0-9]*(ms|s|m|h)$`).
- `$defs.taskCondition` (new): `type: object`, `additionalProperties: false`, `required: [ref]`. Properties: `ref` (string, minLength 1), `on` (enum `visible`, `interacted`, `changed`, `removed`, `entered`, `left`, `responded`, `errored`, `logged`, `holds`), `filter` (object whose values are `oneOf: [{type: string}, {type: array, items: {type: string}, minItems: 1}]`), `name` (string, minLength 1). Whether `on` and `filter` suit the referenced kind is checked by validation, not the schema.

No existing `$defs` entry changes.

## End-to-end examples

Each example gives a corpus fragment, the sessions it is evaluated over, and what the task reports next to what a page-view funnel or server metrics report. Entities defined in one example are not repeated in the next.

### 1. Ordering food: one task, seven sessions

A food-ordering app: menu, item page, cart, checkout, confirmation. The `OrderFood` task from [Shape](#shape) runs over it, extended with the failures this app can actually produce:

```yaml
requests:
  - name: LoadLatestOrder
    route: /api/orders/latest
    method: GET

tasks:
  - name: OrderFood
    tags: [revenue]
    steps:
      - { name: added, ref: AddToCart, filter: { status: "200" } }
      - { name: checkout, ref: Checkout }
      - { name: placed, ref: PlaceOrder, filter: { status: "200" } }
      - { name: confirmed, ref: OrderNumber }
    failed:
      - { name: declined, ref: PlaceOrder, filter: { status: "402" } }
      - { name: order-error, ref: PlaceOrder, filter: { status: ["422", "500"] } }
      - { name: confirmation-lost, ref: LoadLatestOrder, filter: { status: ["404", "500"] } }
    abandoned:
      - { name: emptied-cart, ref: EmptyCartMessage }
```

Three facts about the app's code drive the interesting rows. The checkout page awaits the order request without catching its rejection, so a `402` or `500` leaves the Place order button doing nothing visible. The confirmation page loads the latest order and, if that request fails, redirects to the menu. The order endpoint clears the cart on success, so placing an order again from a stale checkout page returns `422 Cart is empty`.

| Session | Page-view funnel (menu → confirmation) | `OrderFood` record |
|---|---|---|
| Adds an item, checks out, sees the order number | converted | **succeeded**, no failures |
| Adds an item, leaves from the cart | dropped at cart | **abandoned**, `session-ended`, furthest `added` |
| Adds an item, removes it, browses on | dropped at cart | **abandoned**, `emptied-cart`, furthest `added` |
| Card declined; clicks Place order three times; leaves | dropped at checkout | **failed**, `session-ended`, failures `declined ×3`, furthest `checkout` |
| Order endpoint returns `500`; leaves | dropped at checkout | **failed**, `session-ended`, failures `order-error`, furthest `checkout` |
| Order placed; confirmation request fails; redirected to the menu; orders again, which confirms | **converted** | **succeeded**, failures `confirmation-lost` |
| Orders successfully, presses Back to the emptied checkout, clicks Place order; leaves | converted (once) | first attempt **succeeded**; second **failed**, `session-ended`, failures `order-error`, entry `checkout` |
| Returns with a saved cart straight to checkout; places the order | not counted: never saw the menu | **succeeded**, entry `checkout` |

The funnel's two checkout drop-offs are one number; the task separates a payment problem from a server outage, and a user who left from a user who was stuck. The sixth session is the one that matters. The funnel counts a conversion because the confirmation route was entered. Server metrics show two `200` order responses. The task reports a success that cost a failure, labeled `confirmation-lost`: the first order went through, the app never said so, and this customer paid twice. Reliability excludes the attempt; completion includes it. The Back-button session is a defect the funnel cannot see at all, since that user already converted: a stale checkout page still offers Place order on an empty cart. The last session shows why attempts open on any step but the last: a funnel that requires its first step never sees returning customers.

`declined` is listed because this app swallows the rejection, so the user sees nothing and cannot recover. Once the checkout page explains a decline and lets the user try another card, the author removes the condition, and a user who gets through with a second card counts as a clean success.

One session is invisible to every column, task included: an item marked sold out renders its menu card disabled, and a click on a disabled control produces no event. That is correct here, since sold out is not a defect, but it bounds what any declaration can see.

### 2. The canary that passed every health check

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
      - name: invalid                 # resolves only when the price text is garbage
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
    steps:
      - { name: adjusted, ref: SeatSlider, on: interacted }
      - { name: confirmed, ref: InvoiceUpdatedConfirmation }
    failed:
      - { name: price-invalid, ref: SeatPrice, on: changed, filter: { invalid: ["NaN", "undefined", "null"] } }
      - { name: update-rejected, ref: UpdateSubscription, filter: { status: ["400", "402", "409", "500"] } }
    abandoned:
      - { name: left-page, ref: BillingSeats, on: left }
```

| t | Activity | Attempt |
|---|---|---|
| 0s | Drags the slider from 10 to 14 seats | opens at `adjusted` |
| 0.2s | `SeatPrice` re-renders as `NaN €` | failure `price-invalid` recorded |
| 6s | Navigates away | closes **failed**, `left-page` |

Server dashboards are flat and error tracking is empty. The funnel sees a visitor to the billing page who left, the same as one who decided against more seats. The task records a failure at the moment the user saw the broken price, and because a failure was recorded, leaving closes the attempt as failed rather than abandoned. Cut by release, `ExpandSeatCount` reliability falls from 96% to 71% in the canary cohort, and a rollout gate on that measure stops the release on whether users succeeded, not on whether servers responded.

The property's `pattern` parses the text; the filter only compares strings. No comparison operator is needed.

### 3. Failing versus leaving

A members page funnel reports that 79% of users who open the invite dialog submit it. The task splits the same attempts by outcome:

```yaml
components:
  - name: InviteDialog
    selector: '[role="dialog"][aria-labelledby="invite-title"]'
  - name: InviteSubmitButton
    selector: '[role="dialog"] button[type="submit"]'
  - name: InviteSentToast
    selector: '[role="status"].invite-sent'
  - name: SsoRestrictionError
    selector: '.invite-form__error--sso'
  - name: InviteCancelButton
    selector: '[role="dialog"] button[data-action="cancel"]'

requests:
  - name: CreateInvite
    route: /api/orgs/:org/invites
    method: POST

tasks:
  - name: InviteTeammate
    tags: [growth]
    steps:
      - { name: opened, ref: InviteDialog }
      - { name: submitted, ref: InviteSubmitButton, on: interacted }
      - { name: sent, ref: InviteSentToast }
    failed:
      - { name: sso-restricted, ref: SsoRestrictionError }
      - { name: invite-error, ref: CreateInvite, filter: { status: ["500", "502", "503"] } }
      - { name: invite-unreachable, ref: CreateInvite, on: errored }
    abandoned:
      - { name: cancelled, ref: InviteCancelButton, on: interacted }
```

| Outcome | Share | Labels |
|---|---|---|
| succeeded | 41% | `sent` |
| failed | 38% | almost all `sso-restricted`, furthest `submitted` |
| abandoned | 21% | `cancelled`, `session-ended`, furthest `opened` |

Step reach gives back the funnel's 79% (41% + 38% reached `submitted`), and the outcomes say what it hid. Organizations that enforce single sign-on cannot invite by email, a product gap that the `sso-restricted` label routes to the product team. On-call is paged only for `invite-error` and `invite-unreachable`. The 21% who left page no one.

The dialog's removal is deliberately not an abandonment condition. The dialog closes after a successful invite too, and whenever it closes before the toast renders, a `removed` condition would abandon a successful attempt. A user who dismisses the dialog with Escape is still recorded as abandoned, at `session-ended` instead of at the moment they left.

### 4. The toast that lied

An editor publishes optimistically: clicking Publish shows a "Published" toast at once and sends the request in the background. When another editor holds a newer revision, the server answers `409` and the client quietly reverts the post to draft after the toast fades.

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
  - name: PublishArticle
    tags: [content]
    steps:
      - { name: clicked, ref: PublishButton, on: interacted }
      - { name: accepted, ref: PublishPost, filter: { status: "200" } }
    failed:
      - { name: revision-conflict, ref: PublishPost, filter: { status: "409" } }
      - { name: publish-unreachable, ref: PublishPost, on: errored }
    within: 30s
```

| t | Activity | Attempt |
|---|---|---|
| 0s | Clicks Publish | opens at `clicked` |
| 0.05s | "Published" toast visible | (not a condition) |
| 0.4s | `PublishPost` responds `409` | failure `revision-conflict` recorded |
| 30s | No accepted response | closes **failed**, `timeout` |

An analytics event fired from the click handler counts a publish. A check on the page sees the success toast. The task anchors success on the server's acceptance, so it reports the failure, and because the toast is watched, the session also shows the user was told the post was live. `within` is safe here because the attempt opens at the click, which is the user's commitment. Example 2 is the opposite case, a healthy server behind a broken page. A task anchors each condition on whichever side tells the truth.

### 5. The export that never finishes

Exports are queued: the enqueue endpoint returns `202`, a worker builds the file, and a banner appears when it is ready. A scaling change leaves the queue without workers for an hour.

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
    steps:
      - { name: requested, ref: ExportButton, on: interacted }
      - { name: ready, ref: DownloadReadyBanner }
    failed:
      - { name: export-error, ref: ExportFailedBanner }
      - { name: poll-error, ref: ExportPollFailed }
    abandoned:
      - { name: cancelled, ref: CancelExportButton, on: interacted }
    within: 2m
```

No request fails, no message is logged and no error banner renders, so every existing check stays green. The task records two things at once: users who wait two minutes close as **failed**, `timeout`, and users who give up sooner close as **abandoned**, `session-ended`. Neither rate alone proves an outage, since some people always close the tab, but both moving on one task in the same hour is the signature of a stall. The alert carries its own impact: how many attempts timed out, with links to those sessions.

### 6. One declaration, three consumers

`InviteTeammate` from Example 3, unchanged, across the life of a change to the invite dialog:

1. **In the pull request.** A coding agent changes the dialog's layout and runs the task against a preview build: it drives the browser until the attempt closes and reads the record. The last step is the test's done-predicate. A record of `failed`, failures `sso-restricted`, against a test organization with single sign-on disabled is a regression the agent can read and fix before review. No separate assertions exist to drift from the corpus.
2. **In the canary.** The release ships to 5% of traffic. A rollout gate compares `InviteTeammate` reliability between the canary and baseline cohorts and promotes or rolls back on it.
3. **In production.** An objective on `InviteTeammate` reliability over a rolling 7 days replaces an error-rate alert for the members page. Until the single sign-on gap from Example 3 is fixed, the objective is set from that baseline; it measures what users experience, gap included. A breach names its labels and links to the failed sessions.

Cut by actor, the same declaration shows external browser agents abandoning the invite at four times the human rate, nearly all at `session-ended` with furthest step `opened`. The submit button stays disabled until the email field sees a keystroke, and an agent that sets the field's value directly never sends one. Nothing in the task mentions agents, and step reach shows exactly where they stop.

## Alternatives considered

### 1. Do nothing: funnels and events in the consumer

Teams keep defining workflows in an analytics or monitoring tool, as ordered steps or as events fired from application code. Ruled out for the reasons in [Motivation](#motivation): a funnel cannot separate failing from leaving, an event fired from a click handler counts the optimistic success in Example 4, and both drift from the app with no validator to notice. A task declared in the corpus is checked by the same tooling that checks every entity it references.

### 2. Outcomes without steps

A task as `start` and `succeeded` conditions only. Ruled out: it reports outcomes but not where an attempt stopped, so abandonment cannot be located, and opening only on `start` misses every attempt that begins mid-workflow, such as the saved-cart session in Example 1. Steps cost nothing extra: `start` and `succeeded` are the first and last steps.

### 3. The first terminal condition closes the attempt

A `failed` condition closes the attempt immediately. Ruled out: the user's retry happens on the same page and reaches no new step, so it opens nothing and its outcome is never seen, and a user who recovers is counted as failed though they got through. Recording failures on an open attempt keeps both facts.

### 4. Strict step order

Steps must be reached one after another, and an attempt opens only at the first. Ruled out: one missed event (a view transition the capture did not see) stalls an attempt that went on to finish, and attempts that start mid-workflow are lost. Forward skips and mid-workflow entry keep the record accurate while `entry_step` and `furthest_step` keep it honest about what was observed.

### 5. Imperative instrumentation

An SDK in application code: `task.start("OrderFood")`, `task.fail("declined")`. Ruled out: it needs code in every workflow, ships with every release, and only measures sessions recorded after it lands. A declaration over named entities can be evaluated against sessions already captured, as far as they recorded the events it references.

### 6. Two outcomes

`succeeded` and `failed` only, with everything unfinished counted as failure. Ruled out: it is the false alarm this SEP exists to remove. A user who cancels, or closes the tab to come back tomorrow, has not been failed by the product.

### 7. Tasks as signals with a time window

Extend signals with temporal composition (`A followed by B within 30s`) and express a task as signals. Ruled out: a signal is a boolean, and a task's result is a three-way outcome with an attempt lifecycle, a furthest step and a list of failures. Encoding that in signal composition would make every consumer rebuild attempts from booleans. Tasks reference signals instead, so a predicate declared once serves both.

## Migration

`tasks:` is a new top-level key and purely additive. No existing corpus uses it, and a corpus without it behaves exactly as today.

**The additive-field pin applies** (see [`spec/VERSIONING.md`](../VERSIONING.md)). An SDK older than the one shipping this SEP rejects a corpus with `tasks:` under the root's `additionalProperties: false`. Adopters pin to that SDK version before authoring tasks.

**Tasks raise capture volume** for the components they reference with `visible` or `removed`, which become watched, with the record volume [SEP-0015](0015-component-watch.md) describes. Authors should reference the confirmations and error states a task needs, not every component on the page.

**Retroactive evaluation depends on what was captured.** A consumer can evaluate a new task over old sessions only where those sessions recorded the events it references. A component that was never watched has no visibility record in sessions captured before the task existed.

Tooling:

- The validator and loader learn `tasks:` and the diagnostics above.
- `fmt` adds `tasks` to the top-level key order, after `signals`, and sorts it by `name`.
- `search` and `stats` include task names and condition refs.

## Open questions

1. **Nested component refs.** Components under `children:` are not addressable, and many confirmations and error states are nested. A dotted path (`CartBody.EmptyState`) or the stable component `id` proposed in [SEP-0020](https://github.com/sightmap/sightmap/pull/509) would fix this for signals and tasks together.
2. **Alternatives and conjunctions within a step.** A step is one condition. Some workflows finish in either of two ways (a toast, or a redirect), and some succeed only when two things hold (a toast and a `200`). An `any:` or `all:` form can be added later without breaking the single-condition shape.
3. **Step-scoped failures.** Failures apply anywhere in an attempt. A condition that only counts after a given step (an error banner on the payment form, but not on the address form) may need an `after:` field.
4. **Undeclared failures.** A failure nobody declared reads as abandonment. A consumer could flag abandoned attempts that saw an uncaught exception, without the spec making every exception a failure.
5. **Concurrent attempts.** Two uploads in parallel are two attempts of one task, which "one open attempt at a time" cannot express. Correlating attempts to instances would need an identity rule this SEP does not propose.
6. **Status classes.** Examples list status codes because filters support only equality and membership. A `5xx`-style class for the reserved `status` name would remove the lists, and belongs with the comparison-operator question in SEP-0007.
7. **Cross-session tasks.** A workflow finished from an emailed link opens a new session. Tasks are scoped to one session; joining attempts across sessions needs an identity a consumer may not have.

## References

- [SEP-0005](0005-request-properties.md), [SEP-0006](0006-message-entity.md): the request and message entities, and the properties filters compare.
- [SEP-0007](0007-signals.md): reference-based composition, and the draft `filter:` shape tasks share.
- [SEP-0009](0009-component-privacy.md): privacy governs the properties a filter reads; a withheld value never matches.
- [SEP-0015](0015-component-watch.md): visibility reporting, implied for components a task waits on.
- Service level objectives as practiced in site reliability engineering: an objective over the ratio of good events to valid events, with abandoned attempts excluded from the valid set.
