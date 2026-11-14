# Terraform registry field test: caches and smart init

Derived from `tests/fixtures/scenarios/plugin-cache` and the pinned label module in `complete/modules/label`. Local null resources and a registry naming module only; no cloud resources. The investigation harness copies this fixture into an isolated Git root, replaces the engine command with a transparent invocation recorder, and scopes every cache to that copy. Null 3.2.4 and label 0.25.0 are pinned for paired Terraform/OpenTofu execution. Runtime mutations and logs belong under `.context/regression-test/`.

The 2026-10-27 paired field run used Terraform 1.15.6/OpenTofu 1.12.5 on native macOS and disposable Linux containers. Harness and detailed evidence live in `.context/regression-test/terraform-registry-c041-20261027-01/cache/`. It exercised all four plugin/registry cache combinations, public module downloads, offline filesystem mirrors/corruption, RC overrides, smart-init recovery/opt-outs, concurrent fleets, and multi-platform provider locking. These components use only local `null_resource` state; they create no cloud resources.

The completeness addendum additionally exercises scoped provider/module cache inspection/deletion/pruning, offline registry-module reuse, isolated fingerprint changes, and failed/interrupted real-engine initialization recovery.

The retained `.context` harness is not part of the repository. Reproduce each scenario from a fresh checkout with the steps below.

## Prepare a disposable copy

Copy this directory into a new temporary directory, run `git init` there, and point every cache at that copy so no shared cache is read or written:

```shell
export HOME="$PWD/home" XDG_CACHE_HOME="$PWD/xdg-cache"
export ATMOS_XDG_CACHE_HOME="$PWD/xdg-cache"
export ATMOS_COMPONENTS_TERRAFORM_CACHE_LOCATION="$PWD/xdg-cache/atmos/terraform/registry"
export ATMOS_COMPONENTS_TERRAFORM_PLUGIN_CACHE_DIR="$PWD/plugin-cache"
export ATMOS_COMPONENTS_TERRAFORM_COMMAND=terraform   # Repeat every scenario with tofu.
```

On macOS and Windows, enable the registry cache and run a Terraform command first to generate its certificate. The command may fail the trust check; run `atmos terraform cache trust`, then retry it. Linux needs no trust step.

## Cache combinations

Run `atmos terraform apply provider-a -s dev`, `provider-b`, and `modules` for each combination of `ATMOS_COMPONENTS_TERRAFORM_PLUGIN_CACHE` and `ATMOS_COMPONENTS_TERRAFORM_CACHE_ENABLED` (`true`/`false` each). Use a fresh copy per combination, then repeat the applies in the same copy. When the registry cache is enabled, remove each component's `.terraform/` directory before its repeated apply.

- Every apply exits 0 and `provider-a` and `provider-b` report outputs `alpha` and `beta`.
- With the plugin cache enabled, `plugin-cache/registry.terraform.io/hashicorp/null/3.2.4/` exists once and serves both provider components.
- With the registry cache enabled, `atmos terraform cache list` shows the `hashicorp/null` provider and the `cloudposse/label/null` 0.25.0 module. A repeated run that serves cached artifacts prints a savings report.
- With both disabled, nothing is written under `plugin-cache/` or `xdg-cache/atmos/terraform/registry`.

## Offline reuse and corruption

After a warm registry-cache run, block outbound network access (for example, run the copy in a container with `--network none`), delete each component's `.terraform/` directory, and apply again. The applies must succeed from the cache alone. Next, overwrite one cached provider archive under `xdg-cache/atmos/terraform/registry/providers/` with garbage bytes. Disable the plugin cache and remove the affected provider component's `.terraform/` directory before applying it again. The apply must fail with an integrity error, never install the corrupted archive, and recover after `atmos terraform cache delete <key>` with network access restored.

## Smart init

With `init.mode: auto` (the fixture default), a second `atmos terraform plan provider-a -s dev` must not run `terraform init`. Then check each case:

- Changing `components/terraform/provider-a/main.tf` (for example, the provider version constraint) must cause one init for `provider-a` only.
- `--init-mode=always` initializes on every command.
- `--init-mode=never` and `--skip-init` skip initialization.
- After interrupting `atmos terraform init provider-a -s dev` with Ctrl+C, the next ordinary `plan` must reinitialize rather than reuse a partial `.terraform/`.

## Cache maintenance and platforms

- `atmos terraform cache list --format=json` and `atmos terraform cache stats` report the cached provider and module.
- `atmos terraform cache delete <key>` removes only that key.
- `atmos terraform cache prune --older-than=0s --dry-run` lists entries without deleting them.
- With `components.terraform.platforms: [linux_amd64, darwin_arm64]`, `atmos terraform cache mirror --all` stores both platforms of `hashicorp/null` 3.2.4.

For concurrency, start the three applies in parallel against one cold registry cache. All must succeed, and each provider and module must be downloaded once.
