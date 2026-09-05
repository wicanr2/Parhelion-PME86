# From reverse-engineering evidence to code: the spec gate

**English** ｜ [日本語](spec-workflow.ja.md) ｜ [繁體中文](spec-workflow.md)

This repo holds both the knowledge base and the implementation. Between them there has
to be a gate, or "the impression I had while reading the code" turns straight into code,
and in three months nobody remembers what that impression rested on.

## Three states

```
RE evidence  ──►  DRAFT spec  ──►  evidence review  ──►  READY spec  ──►  implement  ──►  same-state verification  ──►  CONFORMED spec
```

| State | Meaning | What it permits |
|---|---|---|
| `DRAFT` | a spec written from disassembly or the manual, not yet backed by evidence line by line | experimental code only, not on the main line |
| `READY` | every assertion has a source (file offset, manual page, or explicitly marked as remake-defined) | **authorises writing the real implementation** |
| `CONFORMED` | implemented, and verified in the same state as the original | may be cited by other specs as a premise |

**Only `READY` authorises real implementation.** That is the point of the whole process:
it separates "I think" from "I checked".

## What a spec contains

Each spec lives in `docs/30-remake/specs/`, opens with its state and scope, then states
assertions one by one, each followed by its source. There are only three kinds of source:

- **original evidence**: a file offset plus bytes, or a manual page.
- **inference**: which pieces of original evidence it was inferred from, with the steps
  written out.
- **remake-defined**: the original did not do it this way; this is the remake's own
  decision.

The third kind has to be spelled out, and split in two:

- **deliberate non-conformance** (for instance, no segment swapping) — the original has
  this behaviour and we chose not to.
- **the remake's own things** (for instance, a debug output format) — the original has
  no counterpart.

Mixed together, they make "this differs from the original — is it a bug or a design
decision?" unanswerable.

## Verification has to be "in the same state"

The bar for `CONFORMED` is not "tested" but **compared in the same state as the
original**: the same codefile, the same inputs, the same starting stack, both sides
producing the same thing.

Comparing with approximate inputs produces a result that looks like a pass and proves
nothing. For details see the oracle loop in
[the feasibility assessment](feasibility.en.md).

## Labels expire

A label like `remake-defined` is **a decision made at a moment**, not a permanent fact.
Once the original evidence is decoded, something originally marked "defined by us" may
turn out to have a counterpart after all. So every spec carries a date, and when an
existing assertion is overturned:

- the body is rewritten to the correct present state, without narrating how it was wrong;
- the record of the overturning goes in one place, the errata table in `PLAN.md`;
- the lesson is written as a rule, not as an event — a present-tense lesson becomes a
  false assertion the moment the thing is fixed.
