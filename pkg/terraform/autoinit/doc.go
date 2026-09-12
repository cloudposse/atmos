// Package autoinit implements Atmos's "smart init" policy for Terraform/OpenTofu: skip the
// `terraform init` subprocess when nothing that init cares about has changed since the last
// successful init, add `-reconfigure` only when the backend configuration itself changed, add
// `-upgrade` only when explicitly requested or required, and recover automatically when
// terraform/tofu reports that init is required after all.
//
// # The three moving parts
//
//   - [Compute] derives a [Fingerprint] from an [Inputs] value: a digest over every file and
//     environment setting that can change what `terraform init` would do (root configuration
//     files, the dependency lock file, var files, CLI config, the resolved binary, and relevant
//     environment variables), plus a narrower [Fingerprint.BackendHash] over only the
//     backend-relevant subset of those files.
//   - [Marker] is the small JSON record ([WriteMarker] / [ReadMarker] / [Record]) Atmos writes
//     into the Terraform data directory after a successful init, capturing the fingerprint that
//     was true at that moment.
//   - [Decide] compares a fresh [Compute] against the last [Marker] (and a handful of filesystem
//     preconditions -- were providers actually installed, are modules present, does local state
//     exist) to produce a [Decision]: whether to run init at all, and with which flags.
//
// # Recovering from a stale skip
//
// Skipping init is a bet that nothing relevant changed; [Classify] and [ShouldRecover] are the
// fallback when that bet is wrong. [Classify] inspects terraform/tofu's own diagnostic output for
// known "you need to run init" signatures, and [ShouldRecover] turns that diagnosis into a
// concrete recovery action -- respecting the caller's configured init policy, including erroring
// out (rather than silently re-running init) when the user explicitly disabled it.
//
// All file paths that contribute to a fingerprint are recorded by name relative to
// [Inputs.ComponentPath], never by absolute path, so the same component checked out at two
// different locations on disk produces identical fingerprints, and files written to a
// process-unique temporary path (e.g. Atmos's generated `TF_CLI_CONFIG_FILE`) are hashed by
// content rather than by their throwaway path.
package autoinit
