# Fix: custom-command `type: container` steps now decode their `with:` block

**Date:** 2026-08-05

## Summary

A custom command's `type: container, action: build` step with a `with:`
block (`engine`, `driver`, `cache`, `tags`, `dockerfile`, `context`, ...)
silently dropped everything and ran a bare `docker build -f Dockerfile .`
when defined in `commands.yaml` and loaded as part of `atmos.yaml`. The same
`with:` block worked correctly in a standalone `workflows/*.yaml` file.
Fixes [cloudposse/atmos#2876](https://github.com/cloudposse/atmos/issues/2876).

## Context

`with:` is polymorphic: for `type: container` steps it must decode into the
typed `Build`/`Run`/`Push`/`Inspect` structs (selected by `action:`); for
other step types it decodes into the generic `With` map. That promotion is
implemented once, in `Task.UnmarshalYAML`/`WorkflowStep.UnmarshalYAML`
(`pkg/schema/task.go`, `pkg/schema/workflow.go`) — go-yaml's
`yaml.Unmarshaler` interface, invoked only when something calls
`yaml.Node.Decode` directly on a `Task`/`WorkflowStep` value. Standalone
workflow files take exactly that path via
`pkg/utils.UnmarshalYAMLFromFile[schema.WorkflowManifest]`.

Custom commands don't: `commands:` in `atmos.yaml` (and its `imports:`, the
usual home for a separate `commands.yaml`) is merged into Viper's config
tree and decoded via `mapstructure` (`pkg/config/load.go`'s
`atmosDecodeHook` → `schema.TasksDecodeHook()` →
`decodeTaskFromMap`). `mapstructure` has no notion of `yaml.Unmarshaler`, so
`Task.UnmarshalYAML` never ran for this path, and `decodeTaskFromMap` had no
equivalent `with:` promotion — the block only ever reached the raw,
untyped `With` map, leaving `Build`/`Run`/`Push`/`Inspect` `nil` regardless
of what `with:` contained.

## Changes

- `pkg/schema/task.go`: `decodeTaskFromMap` now pulls the `with:` key out of
  the map before the `mapstructure` decode runs, then (once `Type`/`Action`
  are known) replays the same polymorphic decode via a new
  `decodeStepWithFromMapValue` helper, which round-trips the value through
  YAML (`yaml.Marshal`/`yaml.Unmarshal` into a `*yaml.Node`) and calls the
  existing `decodeStepWith`/`decodeContainerWith` functions — the exact same
  code the workflow-file path uses. This guarantees the two loading paths
  can't drift apart, rather than re-implementing the promotion twice.

## Validation

- New regression tests, reproduced through real production paths per the
  bug report's explicit requirement (not by manually constructing
  `schema.Task`/`WorkflowStep`/`ContainerBuildStep` literals, which would
  bypass the actual decode bug):
  - `TestCustomCommandContainerBuildStepDecodesWithBlock`
    (`pkg/config/custom_command_container_with_test.go`) — writes a real
    `atmos.yaml`, loads it via `InitCliConfig`, asserts `Build` is populated.
  - `TestCustomCommandContainerBuildPassesWithBlockToDocker`
    (`cmd/custom_command_container_build_test.go`) — writes a real
    `atmos.yaml`, loads it via `InitCliConfig`, registers it via
    `processCustomCommands`, and executes the custom command through
    `RootCmd.Execute()` with a fake logging `docker` executable on `PATH`
    (`tests/testhelpers.InstallFakeContainerRuntime`), asserting the full
    `docker buildx build --builder ... --cache-from ... --cache-to ... -t
    ... -f ...` argv.
  - `TestContainerStepWithBlock_WorkflowAndCustomCommandDecodeIdentically`
    (`pkg/schema/task_test.go`) — the report's explicitly requested
    companion test: decodes the same `with:` block once via
    `yaml.Unmarshal` (the workflow-file path) and once via
    `mapstructure`+`TasksDecodeHook` (the custom-command path), asserting
    the resulting `Build` structs are equal.
- All three confirmed failing against the pre-fix code (`Build` nil / fake
  docker invoked with only `build -f Dockerfile .` / structs unequal) and
  passing post-fix.
- `go build ./...`, `gofumpt` — clean.
- `go test ./pkg/schema/... ./pkg/config/... ./cmd/...` (full packages, not
  just the new tests) — all pass. One transient failure
  (`TestCustomCommandContainerBuildPassesWithBlockToDocker` clashing with
  this repo's own real `.atmos.d/build.yaml` "build" custom command left
  registered on the shared global `RootCmd`) was fixed two ways: renaming
  the test's custom command away from `build`, and — since the underlying
  mechanism is any `cmd` test that loads real custom commands via
  `InitCliConfig` + `processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)`
  without removing them afterward — extending `cmd.NewTestKit(t)`'s
  `snapshotRootCmdState`/`restoreRootCmdState` (`cmd/testing_helpers_test.go`)
  to also snapshot and restore `RootCmd.Commands()`, so any command a test
  registers is removed again in cleanup regardless of which test runs next.
- `./custom-gcl run` via the repo's pre-commit hook — pass.

## Follow-ups

None.
