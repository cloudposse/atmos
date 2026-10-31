// Package sigstore provides the ready-to-use [artifact.Signer] and [artifact.Verifier] for Sigstore
// bundle evidence. It adapts the reusable primitives in pkg/signature to the artifact contracts, so
// composition roots can pass one verifier to every repository (for example
// local.Register(local.WithVerifier(v)) or an OCI repository) without pkg/artifact itself gaining a
// Sigstore dependency.
//
// # Subjects
//
// A signature subject is an immutable digest: the canonical portable Record of an Atmos file
// package, or the original manifest or index of a foreign OCI graph. Verification always derives
// the digest it checks from the subject the caller asked about, never from the evidence.
//
// # Statement form
//
// The Signer is digest-only, because the artifact.Signer contract does not carry subject bytes. It
// signs [SubjectStatement], a deterministic canonical JSON document naming the subject kind,
// digest, media type, and size, with the injected [signature.BlobSigner]. The resulting bundle is
// a message signature over the statement, so it works unchanged on every backend.
//
// # Raw-digest compatibility form
//
// The Verifier also accepts bundles whose signed artifact digest is the subject digest itself:
// a message signature over the subject bytes (as plugin signing and cosign sign-blob produce) or a
// DSSE in-toto statement naming the digest (as cosign OCI signatures produce). The raw form is
// tried first, then the statement form.
//
// # Key path separation
//
// Certificate (keyless) bundles are verified by [signature.Verifier] against trusted identity and
// issuer pairs. Bundles signed with a configured public key are verified by the separate
// [signature.KeyVerifier]. A public-key bundle never satisfies an identity requirement and a
// certificate bundle never satisfies a key requirement; the bundle's verification material and its
// declared evidence format must agree, or the evidence is ignored and reported.
//
// Trust roots come from the injected loader (by default the TUF-authenticated Sigstore public-good
// root); roots supplied by an artifact are never used. In offline mode the loader may use only
// cached trust material, and missing or expired trust fails verification without network access.
package sigstore
