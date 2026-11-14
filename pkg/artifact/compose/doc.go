// Package compose is the composition root of the artifact subsystem. It wires the five repository
// kinds (local/dir, local/archive, aws/s3, oci/registry, oci/layout), one shared signature
// verifier, the signer, the plugin-release immutability policy, the Atmos auth identity resolver,
// the OCI registry transport, and the lock-delegation repository lookup into an [artifact.Service]
// built from the `artifacts` section of the Atmos configuration.
//
// Nothing here runs at import time and nothing is eager: building a [Composition] validates the
// configuration (no network, no credentials, no directory creation) and returns. Repositories are
// constructed when an operation opens them, and identities are resolved only when a backend first
// needs credentials, so describing configuration, dry runs, and validation never authenticate.
//
// The package imports no CLI, runner, hook, plugin, or CI packages. Dependencies that live there,
// such as the managed Cosign signer and the plugin-aware registry authentication preparation, are
// injected by the caller with [WithBlobSigner] and [WithRegistryPreparation].
package compose
