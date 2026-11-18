// Package discovery serves artifact List and Search from a source-scoped discovery cache and
// optional catalogs, without ever downloading a payload.
//
// Directory, archive, and OCI layout repositories enumerate their managed records locally and are
// always listed live. Remote repositories (S3 and OCI registries) are listed once per fifteen
// minutes: the entries are stored as a snapshot under the XDG cache directory
// (atmos/artifacts/discovery), scoped by the effective authentication configuration, realm, and
// host bindings, never by credential values, and replaced atomically only after a complete refresh.
// A fresh snapshot answers a request with zero repository requests. An explicit refresh
// revalidates the source and reuses the metadata of references whose resolved digest is unchanged.
// Offline mode serves only an existing valid snapshot and fails on a missing or corrupt one.
//
// A registry cannot be portably enumerated, so a collection listing of an OCI registry uses the
// registry's repository catalog scoped to the repository prefix when it provides one, or the
// catalog configured on the repository. Listing the versions of one explicit name needs neither.
// A collection that cannot be enumerated fails with an unsupported-capability error; it is never
// an empty successful result.
//
// Catalogs and snapshots are discoverable data, never trust grants. The current policy is
// re-evaluated on every read, including a read of a cached snapshot, and every catalog entry must
// be bound to the destination repository's path.
//
// The shared snapshot mechanics live in the neutral cache subpackage, which plugin discovery also
// uses; the artifact-specific schema and policy live here.
package discovery
