# Fix: Preserve devcontainer execution inputs

**Date:** 2026-10-22

## Summary

Devcontainers preserve environment and Docker build argument names, honor host/container port mappings and transport protocols, and pass command flags after `exec --` to the container.

## Context

Field tests found that Viper lowercased `containerEnv` and `build.args` keys, causing uppercase variables and Dockerfile arguments to disappear. A second port converter ignored the container side of `forwardPorts` mappings. Cobra removed the `--` separator before Atmos parsed the command again, so `sh -c` failed as an unknown Atmos flag.

## Changes

Protect case-sensitive dictionary keys before Viper merges and restore ordinary keys after configuration decoding. Imported dictionaries, includes, aliases, and spec lists retain their values; case-distinct keys remain distinct. This transformation does not evaluate functions. Container creation uses the existing mapping parser and normalizes HTTP/HTTPS application protocols to TCP. The exec parser receives the original argument boundary.

## Validation

Regression tests cover imported overrides, `!env`, includes and spec lists, aliases, distinct uppercase/lowercase keys, Docker port bindings, and Cobra execution with command flags. Live Docker tests built an image with case-distinct arguments, checked case-distinct environment variables with `sh -c`, inspected TCP/UDP bindings, and recreated the container successfully. Owned test containers were removed.

## Follow-ups

Other unsupported Dev Container fields remain outside this change.
