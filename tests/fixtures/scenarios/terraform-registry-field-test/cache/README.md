# Terraform registry field test: caches and smart init

Derived from `tests/fixtures/scenarios/plugin-cache` and the pinned label module in `complete/modules/label`. Local null resources and a registry naming module only; no cloud resources. The investigation harness copies this fixture into an isolated Git root, replaces the engine command with a transparent invocation recorder, and scopes every cache to that copy. Null 3.2.4 and label 0.25.0 are pinned for paired Terraform/OpenTofu execution. Runtime mutations and logs belong under `.context/regression-test/`.

The 2026-10-27 paired field run used Terraform 1.15.6/OpenTofu 1.12.5 on native macOS and disposable Linux containers. Harness and detailed evidence live in `.context/regression-test/terraform-registry-c041-20261027-01/cache/`. It exercised all four plugin/registry cache combinations, public module downloads, offline filesystem mirrors/corruption, RC overrides, smart-init recovery/opt-outs, concurrent fleets, and multi-platform provider locking. These components use only local `null_resource` state; they create no cloud resources.

The completeness addendum additionally exercises scoped provider/module cache inspection/deletion/pruning, offline registry-module reuse, isolated fingerprint changes, and failed/interrupted real-engine initialization recovery.
