// Package policy provides experimental, side-effect-free policy evaluation.
//
// Callers supply already resolved, trusted rules and collected input snapshots.
// The explicit plan-directory loader and planfile subpackage support a narrow
// Terraform command gate. This package does not discover trusted tiers, resolve
// trust, collect CI facts, compute affected inventories, verify approvals or execute hooks.
// It must not be treated as an enforcement boundary on its own.
//
// Evaluator adapters receive materialized JSON, never lazy configuration values
// or an automation host. NewEngine registers CEL and restricted Starlark adapters.
// Existing settings.validation behavior is unchanged.
package policy
