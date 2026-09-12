---
name: editions
description: "Decide whether a PR's new or changed default needs edition-journal handling (pkg/edition, docs/prd/editions.md), and do the mechanical work if so: journal entries, the four-layer default check, snapshot/invariant regeneration, and the post-editions KindBehavior candidate note for behavior changes the journal can't gate yet. Invoke whenever a PR changes what a config key defaults to, or changes what an existing stored value effectively means."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Editions (Date-Anchored Defaults)

Source of truth: `docs/prd/editions.md`. This skill is the contributor-facing decision tree for
"does my change need edition handling", not a restatement of the architecture — read
`docs/prd/editions.md` for the four-layer model, `pkg/edition/journal.go`'s `Entry` schema, and the
guardrail tests before touching anything. This skill exists because the deciding question
("does this default change supersede prior behavior?") is easy to answer wrong by only checking
the literal rule ("new keys are never journal-gated") without checking what the rule is *for*.

## The decision tree

Ask these in order. Stop at the first "yes."

**1. Does this change a previously-shipped key's literal default value?**
(e.g. flipping `settings.terminal.pager`'s struct default from `"true"` to `"false"`.)

→ **Yes: `KindValue` journal entry required.** Follow `docs/prd/editions.md`'s "Contributor
workflow" exactly:
1. Change the literal in `setDefaultConfiguration` (`pkg/config/load.go`), and in
   `defaultCliConfig` (`pkg/config/default.go`) if that struct also carries the field. Confirm the
   key isn't set in the embedded `atmos.yaml` (layer (b) must stay journaled-key-free).
2. Append a dated `Entry{Date, Key, Kind: KindValue, Old, New, Description, Ref}` to
   `pkg/edition/journal.go` — `Ref` is the PR URL, `Old`/`New` typed as the field is typed today.
3. Regenerate the snapshot: `ATMOS_REGENERATE_DEFAULTS_SNAPSHOT=true go test ./pkg/config -run TestDefaultConfigurationSnapshot`.

Skipping this is not optional — `pkg/config/default_snapshot_test.go` fails the build on an
unjournaled value change.

**2. Is this a brand-new key whose default doesn't change what Atmos already, unconditionally did?**
(e.g. a new opt-in flag for genuinely new functionality with no prior equivalent — nothing before
this key ever did what it controls, not even as hardcoded behavior.)

→ **No action needed.** `docs/prd/editions.md`: "new defaults are never journal-gated." Only edit
the layer that introduces the key and regenerate the snapshot (new-key diffs there don't require a
journal entry, just committing the updated golden file).

**3. Is this a brand-new key whose default *does* change what Atmos already, unconditionally did
— just via a key that didn't exist before?**

This is the case rule 2's literal wording ("new key, so exempt") lets through even though the
*effect* on an upgrading user is identical to a changed default: before the key existed, behavior
was fixed; after it ships, the key's default silently changes that fixed behavior for every
existing project with zero action from them. Two sub-questions decide the safe default:

- **Is the new automatic behavior something Atmos flat-out never did before** (a new class of
  automatic side effect, e.g. mutating a lock file, deleting something, calling out to a new
  service)? → **Default to the value that preserves the old, fixed behavior**, even though that
  costs you the "just works" framing in the announcement. Ship the new capability as opt-in.
  Precedent: `components.terraform.init.upgrade` defaults to `never` (Atmos never passed
  `-upgrade` automatically before this setting existed), not `auto`, even though `auto` was the
  natural symmetric choice next to `init.mode`/`init.reconfigure`.
- **Is the new key only making an already-unconditional action conditional/smarter** (same work,
  done less redundantly — no new class of side effect)? → Defaulting to the smarter behavior is
  more defensible, but you're still choosing to ship a real behavior change. Call it out
  explicitly and prominently in the PRD's Goals/Migration section and the blog post — don't let a
  reader assume "new key = no impact." Precedent: `components.terraform.init.mode`/
  `init.reconfigure` default to `auto` (init already ran unconditionally every time; `auto` only
  skips/conditions it), with the PRD's Goals section stating outright which legacy-default
  combination is *not* behavior-preserving.

Either way: proceed to step 4.

**4. Does this PR reinterpret what an existing, already-shipped config value effectively means,
without changing the stored value itself?**
(e.g. `init_run_reconfigure: true` keeps meaning "true" in the YAML, but what `true` *causes*
changed from "always add `-reconfigure`" to "add it only when the backend changed.")

→ **This is `KindBehavior`, not `KindValue` — and `KindBehavior` resolution isn't implemented yet**
(`pkg/edition/journal.go`'s `Kind` field has the enum value reserved, zero entries use it; see
`docs/prd/editions.md`'s Roadmap). You cannot mechanically gate this today. What you **can** and
**must** do:
1. State the reinterpretation explicitly in the PR's PRD (Goals or a dedicated Migration section)
   — don't let it hide inside a generic "behavior change" bullet. Say precisely what the old value
   used to cause and what it causes now.
2. Add a bullet to `docs/prd/editions.md`'s Roadmap → "Behavior gating" seed-entry list, dated to
   this PR's merge date, with a PR link and one sentence describing the reinterpretation — same
   format as the existing 9 seed entries. If this is the first such entry to ship *after* editions
   itself existed (check the entry dates against `docs/prd/editions.md`'s own PRD changelog date),
   say so explicitly, the way the "First post-editions candidate" paragraph does — it's a
   different situation from the pre-editions historical sweep and worth flagging as such.
3. Bump `docs/prd/editions.md`'s own Changelog table with a new dated row noting the addition (no
   version-number bump needed for a docs-only Roadmap addition; use the next minor, e.g. 1.0 → 1.1).

## What NOT to do

- Don't invent a `KindBehavior` journal entry with a fake or partial `BehaviorChanged` predicate to
  "make it gated" — the resolution engine doesn't call it, so it would silently do nothing while
  looking like real protection. Wait for the actual v2 implementation.
- Don't skip the Roadmap note because "it's just a doc change" — the whole point of the seed-entry
  list is to make ungated behavior changes discoverable later, not to pretend editions covers
  something it doesn't.
- Don't default a new automatic-side-effect key to the "smart" value just because the sibling keys
  in the same feature default that way — check step 3's two sub-questions per key, not per
  feature. A single PR can legitimately ship one key defaulting to `auto` and another to `never`.

## Verifying before you commit

```bash
go build ./...
go test ./pkg/edition/... ./pkg/config/...          # anchor/resolve/journal + load_edition_test.go + invariants
ATMOS_REGENERATE_DEFAULTS_SNAPSHOT=true go test ./pkg/config -run TestDefaultConfigurationSnapshot  # only if a default changed
```

`pkg/config/edition_invariants_test.go` and `pkg/edition/journal_invariants_test.go` fail the build
on: non-chronological or malformed journal dates, a chain where `entry[n+1].Old != entry[n].New`, a
journaled key present in the embedded `atmos.yaml`, or `defaultCliConfig` disagreeing with the
journal's current value. Don't hand-edit `pkg/config/testdata/default-config-snapshot.yaml` —
always regenerate it.

## Related

- `docs/prd/editions.md` — architecture, semantics, full journal contents, testing, roadmap.
- `docs/prd/terraform-auto-init.md` — worked example of both a `KindBehavior` candidate
  (`init_run_reconfigure`'s reinterpretation) and a step-3 new-automatic-behavior default choice
  (`init.upgrade: never`) in the same PRD.
- `pull-request` skill — the broader PR-readiness checklist this feeds into; run this skill's
  decision tree before that one's pre-push checklist when a PR touches any default.
