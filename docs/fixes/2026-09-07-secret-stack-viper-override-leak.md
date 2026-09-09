# Fix: `secret` commands no longer leak the resolved `--stack` into viper's override layer

**Date:** 2026-09-07

## Summary

`[race] non-acceptance test suite` failed on a `main` push (run 34179504147, head `82f5413e85`) and on
#3069 (run 34181209571) in `cmd/secret`, each time in one of the two global-scope `secret set` tests:

```text
--- FAIL: TestRunSecretSet_GlobalScopeWithoutComponent (0.00s)
    Error: --component is required to set secret "SHARED_TOKEN": no global declaration was found in the stack
```

The tests themselves are correct. Under `-shuffle=on` they fail whenever any earlier test in the package
ran a `secret` command with a different `--stack`, because `parseScopeStack` (and `parseInitScope`) wrote
the resolved stack into the process-global viper with `viper.Set`. An override outranks every later flag
parse for the life of the process, so the next command's `--stack dev` resolved to the previous `prod`,
its stack filter matched nothing, and the global-declaration lookup failed.

## Context

- Replayed deterministically with the CI seeds: `go test -race ./cmd/secret/ -shuffle=1788834383053658425`
  (main's job) fails at `TestRunSecretSet_GlobalScopeWithoutComponent`; the PR job's seed
  `1788835970504508964` fails its sibling `..._PreservesType`. Unshuffled and 20× unshuffled runs pass.
- Bisected by pairing candidates in source order: `TestRunSecretImport_FromStoreMode` (runs
  `import … --stack prod --component api`) followed by the global-scope set test reproduces the failure;
  `list --stack prod` tests and the completion test that calls `viper.Reset()` do not.
- A throwaway probe confirmed the mechanism: after `resetSecretFlags` the `--stack` pflag reads `""` but
  `viper.GetString("stack")` still returns `"prod"`, and `viper.SetDefault("stack", "")` does not change
  it — so the value lives in the override layer, which only `viper.Set` populates. The only such writes
  are `cmd/secret/shared.go` (`parseScopeStack`) and `cmd/secret/init.go` (`parseInitScope`); `component`
  has no equivalent write and does not leak.
- The override existed so the interactive component prompt's completion (`componentCompletion`, which
  reads the selected stack through viper) sees a stack chosen at the stack prompt. Setting the command's
  own `--stack` flag achieves the same thing through viper's flag binding without a process-wide override.

## Changes

- `cmd/secret/shared.go`: `parseScopeStack` no longer calls `v.Set(cfg.StackStr, …)`. A new
  `adoptPromptedStack(cmd, chosen)` sets the command's `--stack` flag only when the prompt produced a
  value; an empty choice leaves the flag untouched so the required-flag error still fires.
- `cmd/secret/init.go`: `parseInitScope` uses the same helper instead of `viper.GetViper().Set("stack", …)`;
  the `viper` import is dropped.
- `cmd/secret/stack_override_test.go` (new):
  `TestRunSecretSet_StackFlagNotShadowedByEarlierCommand` runs the exact failing sequence
  (`import --stack prod`, then `set --stack dev`) and asserts the second command loads its service with
  `Stack == "dev"`; `TestAdoptPromptedStack_VisibleThroughFlagBinding` checks a prompted stack is visible
  through `viper.GetString("stack")` and gone after the flag reset; `TestAdoptPromptedStack_EmptyChoiceIsNoOp`
  and `TestRunSecretInit_MissingStackNonInteractive` cover the non-interactive path on both call sites, and
  `TestAdoptPromptedStack_MissingFlagErrors` the helper's only failure mode. The prompt branches fold the prompt
  error and the adopt error into the one pre-existing `if err != nil` return, so every changed line is
  exercised without a TTY.

## Validation

- `go test -race ./cmd/secret/ -shuffle=1788834383053658425` and `-shuffle=1788835970504508964` (the two
  failing CI seeds): pass.
- `go test -race ./cmd/secret/ -count=5 -shuffle=on`: 0 failures.
- The pair `TestRunSecretImport_FromStoreMode` → `TestRunSecretSet_GlobalScopeWithoutComponent` fails on
  `main` and passes with the fix (this is what the new regression test encodes).
- `go build ./cmd/... && go vet ./cmd/secret/`: clean; `custom-gcl run --new-from-rev=origin/main`
  (pinned to the go.mod toolchain): 0 issues.
- Not validated interactively: the stack prompt itself needs a TTY (`PromptForMissingRequired` returns
  `""` in tests); the flag-binding unit test covers the propagation the override used to provide.

## Follow-ups

None.
