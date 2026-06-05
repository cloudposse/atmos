// Package function provides a format-agnostic function registry for Atmos.
//
// This package implements a plugin-like architecture for YAML/HCL/JSON functions
// (e.g., !env, !exec, !terraform.output) that can be used across different
// configuration formats.
//
// The registry pattern allows functions to be registered, looked up by name or
// alias, and filtered by execution phase (PreMerge or PostMerge).
//
// Example usage:
//
//	// Register a function
//	fn := NewEnvFunction()
//	function.DefaultRegistry().Register(fn)
//
//	// Look up and execute
//	fn, err := function.DefaultRegistry().Get("env")
//	result, err := fn.Execute(ctx, "MY_VAR default_value", execCtx)
package function
