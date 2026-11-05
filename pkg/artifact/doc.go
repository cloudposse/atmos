// Package artifact defines the generic, repository-independent domain contracts for Atmos
// artifacts: references, portable records, canonical serialization and digests, the repository,
// producer, signer, verifier, and lock-provider interfaces, and the typed service requests and
// results.
//
// The package deliberately imports no CLI, runner, hook, plugin, or CI packages. Backends and
// producers register through composition roots and implement the interfaces defined here.
// Backend SDK types (AWS clients, go-containerregistry images, ORAS remote repositories) never
// appear in this API; OCI seams use opencontainers image-spec descriptors and go-digest values.
//
// # Canonical records
//
// A [Record] serializes to one canonical byte sequence (see [Record.Canonical]): typed JSON with
// a fixed field order, sorted inventory and document arrays, sorted map keys, UTF-8 strings,
// integer sizes and modes, UTC RFC3339 timestamps at second precision, no insignificant
// whitespace, no HTML escaping, and no trailing newline. A Record never contains its own hash.
// The content, intent, and record digests are SHA-256 digests over canonical byte sequences
// defined in this package and frozen by test vectors.
package artifact
