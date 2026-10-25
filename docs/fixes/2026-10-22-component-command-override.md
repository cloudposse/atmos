# Fix: Consistent component executable overrides

**Date:** 2026-10-22

## Summary

Terraform, Helmfile, Packer, and Ansible accept `--command` to select an executable for one invocation.

## Context

The Terraform and Helmfile skills advertised executable override flags that were not consistently registered or honored. Stack loading could replace an override with the component's configured command.

## Changes

Register `--command` and type-specific compatibility aliases with the standard flag parser. Reject empty values and conflicting flags. Preserve the override through merged stack processing, templates, version commands, and Ansible playbook execution. Native tool arguments remain separate. Document the same interface across all four families.

CLI fixtures reset Viper and identity environment variables so authentication selections left by earlier tests cannot change executable-override behavior under shuffled test order. A regression seeds both identity sources before invoking the Ansible fixture.

The Ansible playbook reference states that `--command` and `--ansible-command` are mutually exclusive, matching CLI validation.

## Validation

`go test ./cmd -run '^TestComponentCommand' -count=1` passes. CLI fixtures execute a local binary whose path contains spaces for all four families, covering version commands, aliases, and stack precedence. Invalid configured executables make ignored overrides fail decisively. Empty and conflicting selections fail. Eighteen live cases passed, including native `--command` forwarding after `--`. Raw-argument flag consistency regressions also pass for the compatibility aliases.

The identity/component subset passes with `-race -shuffle=1792894675144043280 -parallel=4`, reproducing the previously failing CI seed. The fixture-isolation regression fails before the fix and passes afterward. `go build ./...` and scoped patch lint pass.

## Follow-ups

None.
