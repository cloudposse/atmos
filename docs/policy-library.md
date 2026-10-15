# Developing the Policy Library

Contributor reference for the [Policy Library](https://atmos.tools/policies/library) and its executable fixtures.

## Tests

Reusable catalog entries live in `policy-library/<name>/`, each with `README.md`, `policy.yaml`,
and `tests/cases.json`, plus an alternate `variants/<engine>/policy.yaml`. Every entry supplies both
CEL and Starlark implementations; the existing `policy.yaml` path remains its default implementation.
The website generates the [Policy Library](https://atmos.tools/policies/library) directly
from these directories. Frontmatter supplies `title`, `description`, `tags`, `related_docs`, and
`command` (the component command with explicit target placeholders).

The index is a compact searchable list inside the Reference sidebar. Each policy page displays the
actual manifests in CEL/Starlark tabs, with copying, followed by configuration, a command, and selectable tested
fixtures. Tabs share the `policy-engine` Docusaurus group so the engine preference persists across pages
and reloads. Install one variant per policy, not both. The website extracts Contract and Configuration sections from the README; manifests and
fixtures are not duplicated in website source. Existing file URLs remain available as secondary links.

```shell
go test ./pkg/policy -run TestPolicyCatalog -v
```

The runner loads both implementations, requires matching and nonmatching fixtures, checks exact
hook requirements, and compares full decisions including approval obligations. Only `engine` and `rule`
may differ between manifests: metadata, selectors, parameters, actions, hooks, and tools must agree.
It verifies each engine's entries can be combined without identity collisions. The website build rejects
missing, duplicate, unsupported, and mislabeled variants rather than displaying incomplete tabs.
Fixtures test evaluation, not hook execution or the trust of collected evidence.

The demonstration configurations live in `examples/policies/`. They are not the reusable policy
library. `TestSampleLibrary` loads these files directly and requires cases for every rule.

```shell
go test ./pkg/policy -run TestSampleLibrary -v
go test ./pkg/hooks -run 'TestPolicyHooksLifecycle|TestPolicyHookFailuresBlock'
go test ./pkg/ci/... -run TestPolicySnapshot
```

Cover matching and nonmatching inputs, threshold boundaries, replacement ordering, deletion
remediation, absent/null/unknown attributes, lifecycle outcomes, and missing required CI evidence.
Hook integration tests use local HTTP endpoints, not production services or installed scanners.

## Integration references

- [Policy configuration](https://atmos.tools/policies/configuration): supported manifest fields and loading behavior.
- [Inputs and expressions](https://atmos.tools/policies/inputs): user-visible input contracts.
- [Hooks](https://atmos.tools/policies/hooks): execution and failure semantics.
- [CI context](https://atmos.tools/policies/ci): supplied-event collector and permissions.
- [Policy capabilities](policy-capabilities.md): implemented contracts and enforcement paths.
- [Engine API](policy-engine.md): evaluator and collector extension points.
