# `hooks-tfmigrate-advanced`

Extends [`hooks-tfmigrate`](../hooks-tfmigrate/) to cover the rest of tfmigrate's
migration actions and both explicit hook modes. That example, and this repository's
automated tests, do not exercise any of these:

| Component                                    | Action(s) covered                | Path exercised                                  |
| --------------------------------------------- | --------------------------------- | ------------------------------------------------ |
| `rm-demo`                                     | `rm`                              | `kind: tfmigrate` hook, `mode: dynamic` |
| `import-demo`                                 | `import`                          | `kind: tfmigrate` hook, `mode: dynamic` |
| `replace-provider-demo`                       | `replace-provider`                | one-off `atmos terraform migrate --migration <file>` CLI flag |
| `xmv-demo`                                    | `xmv` (count → `for_each`, quoted/bracketed addresses) | one-off `--migration` CLI flag |
| `multi-state-source` / `multi-state-target`   | `migration "multi_state"` (`from_dir`/`to_dir`) | `kind: tfmigrate` hook, `mode: dynamic` (standard) |
| `multi-state-diffvars-source` / `-target`     | `multi_state` with mismatched required variables between `from_dir`/`to_dir`, no `from_skip_plan`/`to_skip_plan` | `kind: tfmigrate` hook - demonstrates the documented residual limitation |
| `mode-apply-mismatch-demo`                    | `mode: apply` bound to `before.terraform.plan` (mismatched) | `kind: tfmigrate` hook - must hard-error |
| `mode-plan-mismatch-demo`                     | `mode: plan` bound to `before.terraform.apply` (mismatched) | `kind: tfmigrate` hook - must warn and preview-only |
| `history-nodir-demo`                          | zero-config history mode with no `migrations/` directory yet | `kind: tfmigrate` hook - must report "nothing to do", not crash |
| `onfailure-ignore-demo`                       | a genuinely-failing migration with `on_failure: ignore` | `kind: tfmigrate` hook - underlying apply must still succeed |

> [!NOTE]
> Atmos validates an explicit `mode: apply`/`mode: plan` when it's bound to a
> mismatched lifecycle event, for example `mode: apply` on `before.terraform.plan`.
> Atmos rejects `mode: apply` outright unless it's bound to `before.terraform.apply`
> or `before.terraform.deploy`. Otherwise, `mode: apply` could mutate state during
> what looks like a read-only `plan`. Atmos warns instead when `mode: plan` is bound
> to an apply/deploy event, because the migration would then only ever preview, and
> never apply. `mode-apply-mismatch-demo` and `mode-plan-mismatch-demo` exercise this
> guardrail end-to-end, through the real hook path. Previously, only a unit test
> covered this directly, against `pkg/terraform/tfmigrate/tfmigrate_test.go`'s
> `TestActionForMode_RejectsModeApplyOnNonApplyEvent`. Every other component in this
> example uses `mode: dynamic`, so each action demonstrates success.

All backends use local state, so no cloud credentials are needed. Each component
uses its **own** state file. This differs from `hooks-tfmigrate`, which shares one
state file between two components.

## Requirements

Nothing needs to be pre-installed. The components declare `opentofu` and
`tfmigrate` in `dependencies.tools`. The Atmos toolchain installs both
automatically on first use.

## `rm` — `rm-demo`

```bash
cd examples/hooks-tfmigrate-advanced

# Seed: temporarily swap in the "before" config and apply for real. Skip
# hooks for this bootstrap apply - the migration removes random_pet.temp,
# which doesn't exist yet until this very apply creates it.
cp components/terraform/rm-demo/seed.main.tf.txt components/terraform/rm-demo/main.tf
atmos terraform apply rm-demo -s test -auto-approve --skip-hooks
git checkout -- components/terraform/rm-demo/main.tf   # restore the real (post-rm) config

# before.terraform.plan previews the rm; apply commits it.
atmos terraform plan rm-demo -s test
atmos terraform apply rm-demo -s test -auto-approve
# Confirm: random_pet.temp is gone from state, random_pet.keep is untouched.
atmos terraform state list rm-demo -s test
```

## `import` — `import-demo`

`random_string.imported` simulates a value minted outside Terraform, for example
a token from another system. It needs Terraform management, but must keep its
original value instead of being regenerated. The migration imports ID
`prexist1`, 8 characters long, matching `length = 8`.

```bash
cd examples/hooks-tfmigrate-advanced

atmos terraform apply import-demo -s test -auto-approve
atmos terraform state show random_string.imported import-demo -s test
# "result" matches "prexist1" — the pre-existing value was adopted, not regenerated.
```

## `replace-provider` — `replace-provider-demo`

```bash
cd examples/hooks-tfmigrate-advanced
atmos terraform apply replace-provider-demo -s test -auto-approve

# Simulate a legacy/unqualified provider address in state (what tfmigrate's
# own docs use as the canonical replace-provider example).
python3 -c "
import json
p = 'state/replace-provider-demo.tfstate'
with open(p) as f: s = json.load(f)
for r in s['resources']:
    r['provider'] = 'provider[\"registry.terraform.io/-/null\"]'
with open(p, 'w') as f: json.dump(s, f, indent=2)
"

atmos terraform migrate plan replace-provider-demo -s test \
  --migration migrations/20260802100000_replace_provider.hcl
atmos terraform migrate apply replace-provider-demo -s test \
  --migration migrations/20260802100000_replace_provider.hcl
atmos terraform plan replace-provider-demo -s test   # should show no changes
```

## `xmv` — `xmv-demo`

```bash
cd examples/hooks-tfmigrate-advanced

cp components/terraform/xmv-demo/seed.main.tf.txt components/terraform/xmv-demo/main.tf
atmos terraform apply xmv-demo -s test -auto-approve
git checkout -- components/terraform/xmv-demo/main.tf   # restore the for_each config

atmos terraform migrate plan xmv-demo -s test \
  --migration migrations/20260802100000_xmv_to_for_each.hcl
atmos terraform migrate apply xmv-demo -s test \
  --migration migrations/20260802100000_xmv_to_for_each.hcl
atmos terraform plan xmv-demo -s test   # should show no changes
```

## `multi_state` — `multi-state-source` / `multi-state-target`

```bash
cd examples/hooks-tfmigrate-advanced

cp components/terraform/multi-state-source/seed.main.tf.txt components/terraform/multi-state-source/main.tf
atmos terraform apply multi-state-source -s test -auto-approve
git checkout -- components/terraform/multi-state-source/main.tf   # restore (resource moved out)

# The before.terraform.apply hook on multi-state-target runs the multi_state
# migration, moving random_pet.shared's state entry from multi-state-source
# into multi-state-target.
atmos terraform apply multi-state-target -s test -auto-approve

atmos terraform state list multi-state-source -s test   # empty
atmos terraform state list multi-state-target -s test   # random_pet.shared
atmos terraform plan multi-state-source -s test          # no changes
```
