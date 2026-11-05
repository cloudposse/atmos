# Fix: Align policy references with supported admission controls

**Date:** 2026-10-15

## Summary

Correct the policy references for Go-template admission, source inputs, local path
confinement, and component command selectors. Move library navigation into the
contextual Reference sidebar.

## Context

PR review identified four contradictions or omissions in the policy guides and
library Reference view. The implementation already supports the documented
controls; the remaining proposed source features concern trusted distribution
and trust beyond provenance attribution.

## Changes

- Describe YAML and Go-template function admission as implemented in the PRD and
  admission overview.
- Include source policies in the input and approval references, and link Starlark
  readers to the complete supported input list.
- Document `within_allowed_roots` for rules with a nonempty `allowed_roots` list,
  including unresolved-path behavior.
- Include component command policies in the library's selector guidance and use
  function/source labels that cover their full input contracts.
- Add function documentation for catalog validation, fixture exploration,
  navigation fallbacks, and manifest metadata helpers identified by the review's
  docstring coverage warning.
- Use the existing desktop sidebar and mobile drawer to navigate library entries,
  with a return link to policy guides. Remove the duplicate article navigation and
  preserve engine/view selections when switching policies.

## Validation

- Verified the descriptions against manifest validation, source request
  preparation, and Go-template admission code.
- Website prebuild checks passed (13 tests).
- The documentation production build passed before the sidebar follow-up.
- Sidebar and file-browser checks passed (52 tests).
- The final production build passed; the generated guides contain the corrected
  source and admission references.
- Desktop and mobile browser checks passed for all 17 sidebar links, active
  selection, filtering, guide/catalog transitions, preserved engine/view, cleared
  scenario, history/reload, and narrow layouts without overflow or hydration errors.
- Patch-scoped lint, fix-document validation, and `git diff --check` passed.

## Follow-ups

None.
