---
sep: 0016
title: Tags on messages and signals
author: Clint Ayres (@jurassix)
status: Review
created: 2026-09-29
updated: 2026-09-29
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Complete [SEP-0004](0004-component-tags.md)'s coverage. Add `tags` to `Message` entries, the one
matchable entity that cannot be classified today, and ratify `tags` on `Signal`, which shipped
through [SEP-0007](0007-signals.md) with a schema description pointing at a SEP that never mentions
signals. After this, every entity a consumer can match carries the same open-vocabulary
classification, resolved by the same union rule.

## Motivation

SEP-0004 gave `Component`, `View` and `Request` an author-declared classification. Its Open
questions deferred one entity:

> **Console/exception classification.** Console and exception events aren't a matchable entity in
> the spec today (no selector or route to attach a definition to). Tags there would need a
> different mechanism than this SEP proposes.

**That premise expired.** SEP-0004 was written in July 2026; [SEP-0006](0006-message-entity.md)
created the `messages` entity in August, matched by `level` and a `message` regex. A message is now
a matchable entity with a definition to attach a tag to, exactly as a request is, and it needs no
new mechanism at all.

The gap this leaves is conspicuous rather than theoretical. A corpus can tag the component that
*renders* an error, and a consumer will carry that tag onto the resulting signal. It cannot tag the
error itself:

```yaml
components:
  - name: CartErrorBanner
    selector: '.cart-error'
    tags: [defect]        # works: a signal for this element carries "defect"

messages:
  - name: CartVersionMismatch
    level: ERROR
    message: cart version mismatch
                          # no way to say the same thing about the error
```

The console error is the more direct evidence of the same defect, and it is the one the corpus
cannot classify. Anything consuming tags to route, filter or prioritise sees the banner and not the
exception behind it.

**Signals are a smaller, documentary problem.** `signal.tags` exists in the schema and in the Go
SDK today; it arrived with SEP-0007's Component/View subset. Its schema description reads *"See
SEP-0004"*, and SEP-0004 does not mention signals anywhere. The field works; the spec claims a
provenance it does not have, and nothing states how a signal's own tags relate to the tags of the
entity it references. This SEP ratifies the field and answers that question.

## Proposal

### Shape

```yaml
messages:
  - name: CartVersionMismatch
    level: ERROR
    message: cart version mismatch
    tags: [defect, checkout]

  - name: SlowNetworkWarning
    level: WARN
    message: 'request .* took over \d+ms'
    tags: [perf]

signals:
  - name: checkout.payment.declined
    ref: CheckoutPayment
    tags: [defect]
```

### JSON Schema

- `$defs.message`: add an optional `tags` property,
  `{ type: array, items: { type: string, minLength: 1 } }`, matching the shape already on
  `$defs.component`, `$defs.view`, `$defs.request` and `$defs.signal`. `$defs.message` keeps
  `additionalProperties: false`.
- `$defs.signal.properties.tags`: **no change** to the schema; its `description` is corrected to
  cite this SEP rather than SEP-0004.
- The Go SDK's `messageFields` unknown-field allowlist gains `"tags"`.
- No other `$defs` entry changes, and nothing changes type or requiredness.

### Semantics

**Message tags resolve as a union across every matching entry**, the rule SEP-0004 states for every
entity: tags deliberately do not follow whichever identity rule applies, so a broad tagged
definition is never shadowed by a narrower untagged one.

For messages that rule does more work than anywhere else, because message *identity* is not a
winner but a refusal. SEP-0006 requires a consumer to **surface an ambiguity** when a record matches
more than one entry rather than silently picking one. Tags are unaffected by that: even where a
consumer cannot say *which* message a record is, it can still say the record is tagged `defect`. The
classification survives an ambiguity that the identity does not.

```yaml
messages:
  - name: AnyCheckoutError
    level: ERROR
    tags: [checkout]
  - name: CartVersionMismatch
    level: ERROR
    message: cart version mismatch
    tags: [defect]
```

An `ERROR` record reading `cart version mismatch` matches both. Its identity is ambiguous and a
consumer must say so. Its tags are `[checkout, defect]` regardless.

**A signal's tags are the union of its own and those of the entity it references.** A signal is a
named classification *about* an entity, so the entity's classification applies to it: a signal
referencing a request tagged `payments` is itself about payments, and requiring the author to
restate that on every signal would be the shadowing problem SEP-0004 exists to avoid, one level up.

```yaml
requests:
  - name: CheckoutPayment
    route: /api/checkout/payment
    tags: [payments]
signals:
  - name: checkout.payment.declined
    ref: CheckoutPayment
    tags: [defect]        # resolves to [defect, payments]
```

Resolution is transitive only through `ref`, and `ref` resolves to exactly one entity, so there is
no chain to walk and no cycle to detect.

**The resolved set is deduplicated and lexicographically sorted**, as SEP-0004 already requires
everywhere else, so a consumer sees one stable order.

**Tags remain author-declared.** A consumer MUST NOT infer a tag from a message's `level`, from an
HTTP status, or from any other intrinsic field. Reconciling authored tags with a consumer's own
intrinsic classification stays entirely the consumer's concern, unchanged from SEP-0004.

### Conformance

A conforming SDK MUST:

- Accept an optional `tags: string[]` on any `messages:` entry.
- Resolve a matched record's effective message tags as the **union** of `tags` across every
  `messages:` entry the record matches, not only the entry that supplies identity, and surface that
  union even where identity is ambiguous.
- Resolve a signal's effective tags as the union of its own `tags` and the resolved tags of the
  entity named by its `ref`.
- Deduplicate and lexicographically sort every resolved tag set.
- Never infer a tag from any intrinsic field.

A conforming SDK MUST pass `spec/conformance/017-tags.fixture/`, extended by this SEP to cover
messages and signals.

## Alternatives considered

1. **Amend SEP-0004 in place rather than write a new SEP.** Tempting, since this is its coverage
   being completed. Ruled out: SEP-0004 is Accepted and implemented, and editing an accepted
   proposal to add a field erases the record of when the decision was made and on what grounds. The
   premise that changed, messages becoming matchable, is itself worth recording.

2. **A separate classification mechanism for messages**, as SEP-0004 anticipated. Ruled out because
   the reason for it is gone. SEP-0004 expected messages to need one because they had no definition
   to attach to; SEP-0006 gave them one. Inventing a second mechanism now would mean two vocabularies
   for one idea.

3. **Leave `signal.tags` undocumented.** It works, so nothing is broken today. Ruled out: a schema
   description citing a SEP that does not mention the field is the kind of small untruth that makes
   a spec untrustworthy, and the inheritance question genuinely is unanswered.

4. **Signals do not inherit their referenced entity's tags.** Each signal restates what it needs.
   Ruled out as the shadowing problem SEP-0004 was written against: a request tagged `payments`
   would be invisible on every signal about it unless every author remembered to repeat it. Noted
   as the main open question, since it is the one reviewable judgement here.

## Migration

No corpus migration is required. `message.tags` is optional and additive, and every existing corpus
is unchanged.

**The additive-field pin applies** (see `spec/VERSIONING.md`): an SDK older than the one shipping
this SEP rejects a corpus using `message.tags` with a `must NOT have additional properties` error
rather than ignoring it.

**Signal tag resolution changes observably.** A consumer that reads `signal.tags` today gets the
authored list; after this it gets the union with the referenced entity's tags. That is additive,
never removing a tag, but a consumer filtering on the *absence* of a tag could see different
results. Nothing in the reference implementation does that.

## Open questions

1. **Should a signal inherit its referenced entity's tags?** Proposed yes, per Semantics. The case
   against is that a signal is a distinct named classification and inheritance could surprise an
   author reading one line of YAML. This is the one substantive judgement in the SEP.
2. **Should a message tag be inferrable from `level`?** No, per Semantics, consistent with SEP-0004.
   Worth confirming, since `level: ERROR` is close enough to a classification that a consumer may be
   tempted.
3. **Does `View.tags` need anything here?** No. SEP-0004 already covers it and the JSON Schema
   already carries it; only the Go SDK lags, which is an implementation gap rather than a spec one.
   The PR that lands this SEP closes it, so that tags genuinely work on every entity afterwards.

## References

- [SEP-0004](0004-component-tags.md): the classification this completes, and the source of the
  union rule. Its Open question 2 is resolved by this SEP.
- [SEP-0006](0006-message-entity.md): made messages matchable, which is what makes this possible,
  and the source of the ambiguity rule tags survive.
- [SEP-0007](0007-signals.md): where `signal.tags` actually came from.
