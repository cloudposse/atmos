# Quickstart: Full Component Name in Atmos Pro Uploads

**Feature**: `specs/003-fix-upload-component-name` | Bug-first workflow per Constitution III and CLAUDE.md.

## 1. Reproduce (tests first — they MUST fail before the fix)

### Site 1 — instance-status upload (`internal/exec/pro.go`)

Extend `internal/exec/pro_test.go` (or add a focused test) using the generated `pro.AtmosProAPIClientInterface` mock: call `uploadStatus` with `info := &schema.ConfigAndStacksInfo{ComponentFromArg: "foo/bar/baz", Component: "baz", Stack: "plat-use2-dev", SubCommand: "plan"}` and assert the captured `InstanceStatusUploadRequest.Component == "foo/bar/baz"`. Add a flat-name control case (`vpc`/`vpc`).

```bash
go test ./internal/exec/ -run 'TestUploadStatus' -v   # expect FAIL (got "baz")
```

### Site 2 — multi-component execution record (`cmd/terraform/utils.go`)

Extend `cmd/terraform/utils_exec_metadata_test.go`: invoke `nodeHooks.recordExecResult` with `info := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "baz", ComponentFromArg: "foo/bar/baz", ComponentType: "terraform"}` and assert the accumulated entry's `component` is `"foo/bar/baz"`. (Existing fixtures set both fields equal, so only the new case fails.)

```bash
go test ./cmd/terraform/ -run 'ExecMetadata' -v       # expect FAIL for the nested case
```

### Regression guard — single-component parser path (already correct)

Add/keep a case for `terraformExecMetadataParserFunc("foo/bar/baz", "dev")` asserting the entry's `component == "foo/bar/baz"` (passes before and after — guards FR-004).

## 2. Fix (two lines)

- `internal/exec/pro.go` ~254: `Component: info.Component,` → `Component: info.ComponentFromArg,`
- `cmd/terraform/utils.go` ~581: `..., info.Component, info.Stack, ...` → `..., info.ComponentFromArg, info.Stack, ...`

Do NOT touch `internal/exec/utils.go` splitting logic or any `Component`/`ComponentFolderPrefix` consumer.

## 3. Verify

```bash
go build ./...
go test ./internal/exec/ -run 'TestUploadStatus' -v      # now PASS
go test ./cmd/terraform/ -run 'ExecMetadata' -v          # now PASS
go test ./pkg/pro/ -run 'Pact' -v                        # unchanged, PASS
atmos test                                               # short suite
atmos lint --changed
atmos test --full                                        # before opening the PR
```

Manual smoke (optional, needs a Pro-enabled fixture): `atmos terraform plan "foo/bar/baz" -s <stack> --upload-status` and inspect the payload — `component` must be the full name.

## 4. Before merge (repo policy)

1. Open the FR-008 follow-up issue: single-component execution records report the raw filesystem path for path-style args (`execComponent = args[0]` captured before `resolveComponentPath` rewrite, `cmd/terraform/plan.go:91–95` + `apply.go`/`deploy.go`). Link it by number in the PR description.
2. Run the `/pull-request` skill: label `patch`, closes #3102, note the corrected two-site root cause in the PR body.
