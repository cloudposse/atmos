# Field test: configure and select components with native metadata

**Date:** 2026-10-25

## Scope and method

Test native labels and tags as conditional-import inputs, then use the same classification to list and describe component instances. The target is the feature in PR #3473, against `origin/main` after merging its explicit YAML-tag changes.

Research covered the import implementations, early authentication scanner, existing tests, CLI flag registrations, and related documentation. Live commands used a freshly built CLI in a real PTY, without piping or redirecting commands under test. A fixture hash check verified the expected source state before each invocation. No cloud resources were created or modified.

The durable boundary fixture is `tests/fixtures/scenarios/conditional-import-selectors`, copied from `examples/conditional-imports`. Run the commands below from that fixture with `FIELD_TEST_PCI=true` and `FIELD_TEST_TAG=other`. The example itself contains only the simple classification/configuration/selection flow.

## Findings addressed before publication

### P2: unavailable tags enabled a negative condition

```shell
atmos describe component app -s unknown-negative --query .vars
atmos describe component app -s unknown-negative --query .metadata.tags
```

The stack obtains its tag list through `!include`, which is unavailable during import resolution. Its condition is `!('pci' in tags)`. Before remediation, the unavailable list became an empty CEL list, enabling the mixin and reporting `audit_logging_enabled: true`, although the final tags contained `pci`. This reproduced twice.

Unknown lists now remain distinct from absent or explicitly null lists. A condition that needs unknown tags fails evaluation and skips the import. Both live repetitions now report `audit_logging_enabled: false`. Regression tests also cover unknown values under append and merge strategies, and known replacement clearing an unknown value.

### P2: quoted literal tags beginning with `!` did not match

```shell
atmos describe component app -s literal-bang --query .vars
atmos list instances --stack literal-bang --tags '!env PCI' --format json
```

Under the current explicit-tag rules, the quoted string `"!env PCI"` is literal metadata. The selector found the component, but the equivalent import condition did not enable its mixin. The literal reader incorrectly treated every exclamation-prefixed string as a function. This reproduced twice.

Literal strings now retain their YAML type; actual custom tags remain unavailable. Both live repetitions now enable the mixin, agreeing with component selection.

### P2: draft list commands used invalid column syntax

```shell
atmos list instances --labels pci:true --columns component,stack
```

This failed twice because custom columns require `Name=Template` specifications. The draft examples now omit `--columns`: the default output already shows Component and Stack. The corrected commands were verified live and are used by the regenerated cast.

## Verified behavior

- Label, tag, and combined selectors select `payments/app`; describing it reports audit logging enabled. The standard stack remains disabled.
- A tag-only stack enables the same mixin through `when: !cel "'pci' in tags"` and is selected by `--tags pci`.
- `--labels pci:false` selects the standard stack. A case-mismatched tag and disjoint label/tag filters return an empty array.
- Replacement yields `[payments]` and leaves the PCI mixin disabled. Global append yields `[pci, payments]` and enables it. Global merge preserves the earlier scalar prefix `[pci]` and enables it.
- Structural `!append` yields exactly `[pci, payments]`; a condition requiring `size(tags) == 2` passes.
- A skipped missing import does not fail stack resolution.
- Repeated selection and description commands are deterministic.
- Import/authentication parity is additionally covered by the shared unit scenarios and race-detector runs; this pass made no cloud authentication calls.

## Intended boundaries confirmed live

| Fixture | Import decision | Final selector behavior |
|---|---|---|
| `quoted-boolean` | String `"true"` does not satisfy `labels.pci == true` | `--labels pci:true` matches because selectors stringify values |
| `component-override` | Stack-wide `pci: true` enables the mixin | Component-local `pci: false` excludes it from the true selector |
| `later-label` | Earlier condition skips the mixin | A later import adds the label, so final selection matches |
| `computed-label` | `!env` value is unavailable during imports | It resolves for final selection |
| `mixed-tags` | A computed member makes the import-time list unavailable | The literal `pci` member is present in final selection |

The blog and reference explain these boundaries. For the simplest connection between configuration and selection, declare literal metadata in the top-level stack and let components inherit it unchanged. Direct `describe component` takes a component and stack; selectors belong on `list instances` and other commands that explicitly support them.

## Follow-ups

No unaddressed runtime defects found in the exercised scope. The broader agent-skills summaries do not yet cover conditional imports; this pass did not expand that separate documentation surface.
