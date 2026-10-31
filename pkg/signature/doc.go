// Package signature holds the reusable Sigstore verification and signing primitives shared by
// every artifact repository and the plugin installer: bundle verification against trusted
// certificate identities, a separate explicitly configured public-key bundle path, the trusted
// root, and signer-output validation.
//
// It performs no registry, plugin, CI, or command-line work. Adapters fetch and store evidence and
// then call these verifiers, so attachment transport stays outside this package. No custom
// cryptography lives here; everything delegates to the Sigstore libraries.
package signature
