# Fix: Playwright Node preseed rejects glibc Node.js on Linux musl

**Date:** 2026-07-03

## Problem

The Playwright driver preseed fallback downloaded Node.js archives from
`nodejs.org` whenever `PLAYWRIGHT_NODEJS_PATH` was not set. On Linux, those
archives are glibc-linked. Alpine and other musl-based environments could
therefore receive a Node.js binary that downloaded successfully but could not
run.

## Fix

The Node.js preseed platform resolver now detects Linux/musl hosts and refuses
to download the `nodejs.org` Linux archive. In that case, Atmos returns the
Playwright driver seed error with instructions to set `PLAYWRIGHT_NODEJS_PATH`
to a preinstalled musl-compatible Node.js binary.

glibc Linux, macOS, and Windows still use the verified `nodejs.org` archive
path.

## Tests

```shell
go test ./pkg/auth/providers/aws -count=1
```
