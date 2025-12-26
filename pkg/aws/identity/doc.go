// Package identity provides AWS caller identity retrieval and caching.
//
// This package consolidates AWS identity-related functionality used by Atmos functions
// (YAML, HCL, etc.) and provides a clean, reusable interface for identity operations.
//
// Key features:
//   - AWS config loading with support for auth context
//   - Caller identity retrieval via STS GetCallerIdentity
//   - Thread-safe caching of identity results per auth context
//   - Testable via Getter interface
package identity
