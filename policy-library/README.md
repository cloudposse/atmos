# Atmos Policy Library

Reusable, individually documented CEL and Starlark policies. Browse the catalog at
[Policy Library](https://atmos.tools/policies/library).

Each directory contains one `policy.yaml`, a technical `README.md`, and
`tests/cases.json` with input snapshots and expected decisions. An alternate implementation lives in
`variants/<engine>/policy.yaml`; every entry provides CEL and Starlark versions. README frontmatter supplies
the catalog title, description, and category; no separate list of entries is maintained.

Every catalog manifest includes `metadata.description` with its plain-English purpose. This
literal text is separate from the `rule` and the denial `reason`.

## Use selected policies

From a reviewed, pinned checkout of this repository:

```shell
mkdir -p policy.d
cp policy-library/terraform-protect-stateful-resources/policy.yaml policy.d/protect-stateful-resources.yaml
cp policy-library/terraform-require-owner-tag/policy.yaml policy.d/require-owner-tag.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
atmos terraform plan vpc -s dev
```

Review parameters and hook prerequisites before enabling a policy. Only copy selected manifests;
the loader does not recurse through this catalog. On each library page, choose the CEL or Starlark tab
and copy the complete manifest; the website remembers your engine choice. To use an alternate from a
checkout, copy its `variants/<engine>/policy.yaml` instead of the entry's default `policy.yaml`.
Install only one implementation of each policy: variants intentionally share the same policy identity.
Preserve unique `metadata.name` values when
combining policies. Load governing definitions from a protected revision in CI, not unreviewed PR content.

## Tests

```shell
go test ./pkg/policy -run TestPolicyCatalog -v
```

The test runner discovers every directory and evaluates both actual manifests against the same cases.
It compares complete decisions (including approvals and hooks), and requires identical configuration,
selectors, parameters, hooks, and tool definitions; only the engine and expression may differ.
Entries must have one rule, matching/nonmatching cases, and exact expected hook requirements.
A combined-directory test checks each engine's catalog for policy and hook identity collisions.

`not_applicable` means the rule did not match. `allow` with hook requirements selects work, not
proof that work completed. Fixtures are evaluator tests; lifecycle execution is tested separately
in `pkg/hooks`.

End-to-end demonstration projects remain in `examples/policies/`.
