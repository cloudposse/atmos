// Package report renders the results of artifact operations as CI job summaries and CI output
// variables.
//
// The package has two halves. [Collector] is the seam execution boundaries record into: the
// artifact step handler, the CLI commands, and (through the step handler) workflows, custom
// commands, and hooks. [Session] is the collector that renders. It buffers every recorded
// attempt, keeps the final attempt of each logical operation, orders the operations by their
// scheduled position, and on [Session.Flush] writes exactly one summary through the existing
// ci.Reporter, so the ci.* gates, provider routing, masking, and template loader apply unchanged.
//
// The renderer works over typed results only. It never mutates a repository and never asks a
// repository for more metadata or evidence: a disabled summary costs nothing, and an enabled one
// reports exactly what the operation already returned. Collectors are passed explicitly through
// runner services and options, never through context values.
//
// This is the only package under pkg/artifact that depends on CI reporting. The domain,
// services, and wiring packages stay free of it, which a boundary test enforces.
package report
