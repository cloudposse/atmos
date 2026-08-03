# `hooks-tfmigrate-advanced`

Extends [`hooks-tfmigrate`](../hooks-tfmigrate/) to cover the rest of tfmigrate's
migration actions and both explicit hook modes, none of which are exercised by that
example or by any automated test in this repository:

| Component                                    | Action(s) covered                | Path exercised                                  |
| --------------------------------------------- | --------------------------------- | ------------------------------------------------ |
| `rm-demo`                                     | `rm`                              | `kind: tfmigrate` hook, `mode: apply` bound to `before.terraform.plan` |
| `import-demo`                                 | `import`                          | `kind: tfmigrate` hook, `mode: plan` bound to `before.terraform.apply` |
| `replace-provider-demo`                       | `replace-provider`                | one-off `atmos terraform migrate --migration <file>` CLI flag |
| `xmv-demo`                                    | `xmv` (count → `for_each`, quoted/bracketed addresses) | one-off `--migration` CLI flag |
| `multi-state-source` / `multi-state-target`   | `migration "multi_state"` (`from_dir`/`to_dir`) | `kind: tfmigrate` hook, `mode: dynamic` (standard) |

`rm-demo` and `import-demo` deliberately bind an explicit `mode:` to the *other*
lifecycle event than you'd expect, to make a real, easy-to-hit misunderstanding
concrete:

- **`rm-demo`**: `mode: apply` on `before.terraform.plan`. Running only `atmos
  terraform plan rm-demo -s test` — which looks like a safe, read-only preview —
  actually runs a real `tfmigrate apply` first and permanently mutates state.
- **`import-demo`**: `mode: plan` on `before.terraform.apply`. Running `atmos
  terraform apply import-demo -s test` looks like it performs the import, but the
  hook only previews it; the Terraform apply that follows never actually imports the
  pre-existing file, and instead creates/overwrites it fresh from the resource's
  `content` value.

All backends are local state (no cloud credentials needed), and each component uses
its **own** state file (unlike `hooks-tfmigrate`, which shares one file between two
components).

## Requirements

Nothing needs to be pre-installed — `opentofu` and `tfmigrate` are declared in
`dependencies.tools` and auto-installed by the Atmos toolchain on first use.

## `rm` — `rm-demo`

```bash
cd examples/hooks-tfmigrate-advanced

# Seed: temporarily swap in the "before" config and apply for real.
cp components/terraform/rm-demo/seed.main.tf.txt components/terraform/rm-demo/main.tf
atmos terraform apply rm-demo -s test -auto-approve
git checkout -- components/terraform/rm-demo/main.tf   # restore the real (post-rm) config

# This LOOKS like a read-only preview, but the mode: apply hook on
# before.terraform.plan means it actually runs `tfmigrate apply` for real.
atmos terraform plan rm-demo -s test
# Confirm: random_pet.temp is gone from state, random_pet.keep is untouched.
atmos terraform state list rm-demo -s test
```

## `import` — `import-demo`

`random_string.imported` simulates a value minted outside Terraform (e.g. a token
from another system) that needs to be brought under management without being
regenerated. The migration imports ID `prexist1` (8 chars, matching `length = 8`).

```bash
cd examples/hooks-tfmigrate-advanced

# This LOOKS like it will import the pre-existing value, but the mode: plan
# hook on before.terraform.apply only previews — it never actually imports.
# The terraform apply that follows creates a NEW random_string instead of
# adopting "prexist1".
atmos terraform apply import-demo -s test -auto-approve
atmos terraform state show random_string.imported import-demo -s test
# Compare the "result" attribute against "prexist1" — if the hook had actually
# imported, they'd match; instead a freshly generated value shows up.
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
