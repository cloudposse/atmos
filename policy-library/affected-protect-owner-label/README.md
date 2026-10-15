---
title: Preserve instance ownership
description: Allow new owner labels but reject changing or removing an existing owner.
command: atmos describe affected --base origin/main
tags:
  - Affected Changes
related_docs:
  - label: Affected changes
    url: /policies/affected
  - label: Policy configuration
    url: /policies/configuration
---

# Preserve instance ownership

Allow new owner labels but reject changing or removing an existing owner, including deletion
of the entire instance. Inherited label changes are evaluated on merged instance configuration.

## Contract

| Field | Value |
|---|---|
| Input | `atmos.affected` |
| Engine | `cel` |
| Action | `deny` |
| Selects when | An existing `metadata.labels.owner` changes or disappears. |
| Required data | Complete static change inventory under protected discovery settings. |

## Configuration

None. Change `owner` in the expression to protect a different label.

Run affected analysis against clean, committed worktrees. The policy runs before stack, tag,
label, and output filtering. It does not automatically run on commands without affected analysis.
Protect discovery settings and load this policy from a governing revision in CI; a policy removed
from the candidate directory cannot enforce its own preservation.

## Policy

[View policy.yaml](https://atmos.tools/policies/library/affected-protect-owner-label/policy.yaml).

## Use

```shell
mkdir -p policy.d
cp policy-library/affected-protect-owner-label/policy.yaml policy.d/protect-owner.yaml
atmos describe affected --base origin/main
```

Commit local changes before comparison. In CI, use `ATMOS_POLICY_DIR` to select a protected directory.
See [enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Tests

Fixtures cover changed and removed owners, instance deletion, null parent metadata, allowed label
additions, unchanged owners, and incomplete inventories.

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/affected-protect-owner-label$' -v
```

[View fixtures](https://atmos.tools/policies/library/affected-protect-owner-label/tests/cases.json).
