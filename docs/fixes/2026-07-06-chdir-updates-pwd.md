# Fix: chdir updates PWD for templates and subprocesses

**Date:** 2026-07-06

## Problem

Changing the process working directory did not necessarily update the `PWD`
environment variable. Code that reads `PWD`, including subprocesses and
templates such as `{{ env "PWD" }}`, could still see the original directory
after `--chdir`.

## Fix

The chdir helper now updates both the process working directory and `PWD`,
matching shell `cd` behavior. If changing directory fails, `PWD` is left
unchanged.

## Tests

```shell
go test ./pkg/env -run 'TestChdir|TestChdirNonexistentDirectory' -count=1
```
