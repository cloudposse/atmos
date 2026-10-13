# Developing the Policy Library

Contributor reference for the [Policy Library](https://atmos.tools/policies/library) and its executable fixtures.

## Tests

Reusable catalog entries live in `policy-library/<name>/`, each with `README.md`, `policy.yaml`,
and `tests/cases.json`. The website generates the [Policy Library](https://atmos.tools/policies/library) directly
from these directories. Frontmatter supplies `title`, `description`, `tags`, and `related_docs`.

```shell
go test ./pkg/policy -run TestPolicyCatalog -v
```

The runner loads each actual policy, requires matching and nonmatching fixtures, checks exact
hook requirements, and verifies that all entries can be combined without identity collisions.
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
