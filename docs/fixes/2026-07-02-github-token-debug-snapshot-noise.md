# Fix: snapshot tests ignore GitHub token probe debug noise

**Date:** 2026-07-02

## Problem

Some CLI snapshot tests run with debug logging enabled while command startup
loads toolchain registry configuration. That path may probe GitHub credentials
through the GitHub CLI.

When the local environment does not have an authenticated `gh`, the command can
emit debug lines about the failed token lookup and anonymous GitHub access. Those
lines depend on developer and CI authentication state, so snapshots could fail
even though command behavior was unchanged.

## Fix

The affected snapshot test cases now ignore the environment-dependent GitHub
token probe debug lines.

This matches the existing treatment in other verbose snapshot tests and keeps
the snapshots focused on command behavior rather than whether `gh auth token`
works on the machine running the tests.

## Tests

```shell
go test ./tests -run 'TestCLICommands/(atmos_describe_config_imports|atmos_describe_configuration|atmos_auth_validate_--verbose)' -count=1
```
