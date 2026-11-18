// Package config decodes, validates, and interprets the `artifacts` section of atmos.yaml.
//
// It is separate from pkg/artifact so that the domain package stays free of configuration policy,
// while pkg/config (the loader) and the artifact service share one implementation of:
//
//   - strict decoding of the raw section into schema.ArtifactConfig, including rejecting
//     unquoted numeric permissions that YAML would silently turn into the wrong octal value;
//   - whole-configuration validation (kinds, per-kind options, packaging, permissions, catalogs,
//     identities, labels, verification policy, and delegated lock chains);
//   - the effective verification policy model, where a global requirement can be strengthened but
//     never weakened by a repository and both trust sets must be satisfied;
//   - anchoring relative local paths to the configuration file that declared them.
//
// Validation performs no network access, resolves no credentials, and creates no directories.
// An `identity` is only checked for shape (and, when the caller supplies a lookup, existence);
// it is resolved lazily by the operation that needs credentials.
//
// # Repository kinds
//
// Kind names are checked against artifact.RepositoryRegistry, which returns a registered backend's
// descriptor when one exists and otherwise the documented static set of V1 kinds (local/dir,
// local/archive, aws/s3, oci/registry, oci/layout). Validation therefore works before backend
// packages register, and a backend that registers a kind refines the descriptor rather than
// changing the accepted names.
package config
