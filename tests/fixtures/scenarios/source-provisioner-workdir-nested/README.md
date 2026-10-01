# Nested Component Name + Workdir + Local Backend Test Fixture

Regression fixture for `docs/fixes/2026-08-05-workdir-nested-component-path-depth.md`
(`workdir.BuildPath`, `pkg/provisioner/workdir/types.go`). Unlike
`../source-provisioner-workdir` (flat component names only, no backend
config) and `../terraform-state-jit-workdir` / `../terraform-output-jit-workdir`
(flat names only), this fixture combines everything needed to actually
observe the bug's real-world symptom:

- `source:`-based JIT provisioning, using a **local, network-free** source
  module (`./source-modules/mock`) instead of a remote GitHub URL.
- `provision.workdir.enabled: true`.
- `backend_type: local` with a **relative** `backend.local.path` containing
  `../` segments, so a workdir-depth regression silently changes *where
  state lands* instead of raising an error.
- `components.terraform.workspaces_enabled: false` -- required for the
  relative `backend.local.path` to actually govern where Terraform writes
  state at all. With Atmos's default (workspaces enabled, workspace ==
  stack name), Terraform's local backend ignores the configured `path` for
  non-default workspaces and uses `terraform.tfstate.d/<workspace>/` under
  the CWD instead -- a footgun independent of this fix, worth knowing if you
  reuse this pattern.
- Sibling components with flat (`vpc`), one-level-nested (`ecs/cluster`),
  and two-level-nested (`ecs/cluster/subnet`) names at the **same stack**,
  to prove workdir depth -- and therefore where a relative backend path
  resolves -- is independent of component name nesting depth.
- `app/local-nested`: a **local (non-source)** nested component with
  workdir enabled, exercising `pkg/provisioner/workdir/workdir.go`'s
  `Service.Provision` -> `createWorkdirDirectory`, a *different* code path
  from `workdir.BuildPath` that used to reimplement the same unsanitized
  `fmt.Sprintf("%s-%s", stack, component)` + `filepath.Join` formula
  (`createWorkdirDirectory` now delegates to `BuildPath` -- see
  docs/fixes/2026-08-07-createworkdirdirectory-duplicate-unsanitized-formula.md).
- `../escape-test` and `../escape-test-nowd`: path-traversal probes for
  component names containing `..` segments.
- `consumer-*`: components reading the flat and nested producers via
  `!terraform.output` / `!terraform.state`, exercising
  `pkg/terraform/output/config.go`'s `extractComponentPath` resolver.

## Manual testing

```bash
cd tests/fixtures/scenarios/source-provisioner-workdir-nested

# Flat vs. nested vs. two-level-nested workdirs should be SIBLINGS at the
# same depth under .workdir/terraform/, and state should land at the SAME
# .context/tfstate/dev/<name>/ ancestor for all three:
atmos terraform apply vpc --stack dev
atmos terraform apply "ecs/cluster" --stack dev
atmos terraform apply "ecs/cluster/subnet" --stack dev
find .workdir/terraform -maxdepth 1 -mindepth 1 -type d
find .context -type f

# !terraform.output / !terraform.state against the nested producer:
atmos describe component consumer-nested-output --stack dev
atmos describe component consumer-nested-state --stack dev

# FIXED: local, non-source nested component now sanitizes identically to
# workdir.BuildPath (createWorkdirDirectory delegates to it). Expect the
# sanitized sibling .workdir/terraform/dev-app-slocal-hnested/ (NOT a real
# nested .workdir/terraform/dev-app/local-nested/ directory -- BuildPath
# injectively encodes "/" as "-s" and a literal "-" as "-h", escaping the
# literal hyphen before the separator so it can never collide with a
# differently-named component that uses "/", "-", or "--" in the same
# spot), and state landing at .context/tfstate/dev/app-local-nested/
# at the fixture root (a separate, hand-configured backend.local.path in this
# fixture's stack config, unrelated to BuildPath's own encoding) (NOT nested
# one level short at .workdir/.context/tfstate/...):
atmos terraform apply "app/local-nested" --stack dev
find .workdir/terraform -maxdepth 1 -mindepth 1 -type d
find .workdir -name terraform.tfstate -not -path "*/.terraform/*"

# Path-traversal probes: ../escape-test is sanitized safely, while
# ../escape-test-nowd must fail with ErrPathTraversal ("path traversal not allowed")
# rather than vendoring outside the intended component base path. Neither should create
# anything under components/ -- confirm with the `test -d` checks below.
atmos terraform source pull "../escape-test" --stack dev        # sanitized, safe
atmos terraform source pull "../escape-test-nowd" --stack dev   # must fail with ErrPathTraversal
test -d components/escape-test && echo "UNEXPECTED: escape-test dir exists" || echo "ok: no escape"
test -d components/escape-test-nowd && echo "UNEXPECTED: escape-test-nowd dir exists" || echo "ok: no escape"
```

## Cleanup

```bash
rm -rf .workdir/ .context/ components/escape-test-nowd/
```
