// Package service implements [artifact.Service]: the repository-independent orchestration of
// publish, pull, copy, inspect, list, search, delete, document reads, sign, verify, and pin over
// [artifact.Repository] implementations.
//
// The service owns name and version selection, producer preparation, effective metadata, digest
// pinning, conflict policy, retries, and result aggregation; repositories own storage mechanics
// and capabilities. Three rules hold for every operation:
//
//   - A mutable reference is resolved exactly once; every later read, verification, and copy uses
//     the pinned subject, so a tag moved mid-operation can never substitute different bytes.
//   - Capabilities and lock guarantees are checked before the first mutation, locks are acquired in
//     canonical repository/resource order, and they are released on success, error, and cancellation.
//   - Required verification runs before destination publication or local materialization, and
//     force never bypasses it. Without any trust configuration the result is "unchecked", never
//     "verified".
//
// Every method returns its typed result alongside the error so reporting can describe completed work.
package service
