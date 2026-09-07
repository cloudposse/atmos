// Package proexec reports command-execution metadata (what ran, exit code,
// environment, resource usage, and optional command-specific structured data)
// to Atmos Pro whenever Atmos is running in a recognized CI environment and
// Atmos Pro is configured. Every command gets a fire-and-forget, best-effort
// asynchronous upload (CaptureAsync); a small allowlist of critical commands
// (terraform plan/apply, describe affected) additionally calls the blocking,
// configurable-timeout variant (CaptureSync) from their own execution path.
package proexec
