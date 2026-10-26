// Package tfvars partitions Terraform variables into a disk-safe subset and a
// secret-bearing subset, and renders the secret subset as TF_VAR_* environment
// variables. This keeps resolved secret values out of the on-disk varfile
// (`*.terraform.tfvars.json`) — which would otherwise leak them in plaintext —
// by injecting them at runtime via the process environment instead.
//
// Whether a variable is "secret-bearing" is decided by an injected predicate
// (typically io.ContainsSecret), so any top-level variable whose value contains a
// registered secret — even as a substring composed via interpolation, or nested
// inside a map or list — is routed to the environment rather than the file.
package tfvars

import (
	"encoding/json"
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
)

// tfVarEnvPrefix is the prefix Terraform/OpenTofu uses for variable environment
// variables: TF_VAR_<name>. See
// https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_var_name.
const tfVarEnvPrefix = "TF_VAR_"

// Partition splits vars into two maps by top-level key:
//   - safe: variables whose value contains no registered secret (written to disk).
//   - secret: variables whose value contains a registered secret (injected via env).
//
// isSecret reports whether a single string contains a registered secret. A
// top-level variable is treated as secret-bearing if isSecret returns true for any
// string leaf reachable from its value (the value itself, nested map values, slice
// elements, or the string form of any other scalar). When isSecret is nil, every
// variable is considered safe (no partitioning).
//
// The returned maps are always non-nil. Input is not mutated.
func Partition(vars map[string]any, isSecret func(string) bool) (safe, secret map[string]any) {
	defer perf.Track(nil, "tfvars.Partition")()

	safe = make(map[string]any, len(vars))
	secret = make(map[string]any)

	for k, v := range vars {
		if isSecret != nil && valueContainsSecret(v, isSecret) {
			secret[k] = v
			continue
		}
		safe[k] = v
	}

	return safe, secret
}

// valueContainsSecret walks v depth-first and reports whether any string leaf
// satisfies isSecret. The recursion mirrors io.RegisterSecretValue so that every
// representation a secret could have been registered under is also checked here.
func valueContainsSecret(v any, isSecret func(string) bool) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return isSecret(t)
	case map[string]any:
		for _, child := range t {
			if valueContainsSecret(child, isSecret) {
				return true
			}
		}
		return false
	case []any:
		for _, child := range t {
			if valueContainsSecret(child, isSecret) {
				return true
			}
		}
		return false
	default:
		// Non-string scalars (numbers, bools) may themselves be secrets; check
		// their string form, matching how io.RegisterSecretValue registers them.
		return isSecret(fmt.Sprintf("%v", t))
	}
}

// SecretEnv renders the secret subset as TF_VAR_<name>=<value> environment entries,
// suitable for appending to a process environment (e.g. info.ComponentEnvList).
//
// String values pass through verbatim. All other types (numbers, bools, maps, lists)
// are JSON-encoded; Terraform parses TF_VAR_ values for complex-typed variables as
// HCL, and JSON object/list/primitive literals are valid HCL expressions.
//
// The returned slice is deterministic only up to Go map iteration order; callers that
// need stable ordering should sort. Returns an error if a value cannot be encoded.
func SecretEnv(secret map[string]any) ([]string, error) {
	defer perf.Track(nil, "tfvars.SecretEnv")()

	env := make([]string, 0, len(secret))
	for k, v := range secret {
		val, ok := v.(string)
		if !ok {
			encoded, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("encoding secret variable %q for TF_VAR_ injection: %w", k, err)
			}
			val = string(encoded)
		}
		env = append(env, fmt.Sprintf("%s%s=%s", tfVarEnvPrefix, k, val))
	}

	return env, nil
}
