// Package artifacttest provides test support for artifact repositories and the artifact service:
// a minimal in-memory [Repository] that implements the full [artifact.Repository] contract with
// failure injection, a fake signer and evidence format, and [RunRepositoryConformance], the shared
// conformance suite every backend (local directory, local archive, S3, OCI layout, OCI registry)
// runs against its own implementation.
//
// The in-memory repository is a harness, not a backend: it models the publication lifecycle
// (pending, completed, unavailable after interruption), conditional updates, digest-addressed
// reads that never serve replacement bytes, pagination, and ownership-checked locks, so service
// behavior such as reconciliation can be tested without a real store.
package artifacttest
