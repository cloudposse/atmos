// Package oci implements the oci/registry and oci/layout artifact repositories on the pkg/oci
// transport boundary.
//
// Both kinds share one implementation core. A backend supplies the content store for an artifact
// name (an ORAS registry repository, or the single shared OCI layout), how versions are named,
// and how references are removed; everything else is common: classification of a resolved root,
// manifest assembly for new Atmos packages, metadata-only inspection, verified reads, guarded
// publication, name-scoped and collection listing, tag-safe deletion, signature attachments as
// OCI referrers, host and delegated write locks, and byte-identical graph transfer.
//
// A resolved root is exactly one of four things, and the package never confuses them:
//
//   - an Atmos package, whose config blob is the canonical portable Record;
//   - a legacy Atmos plugin release (immutable; never listed as a generic artifact, never
//     overwritten, even with force);
//   - a foreign image or artifact manifest;
//   - a foreign image index (for example a multi-platform image).
//
// Foreign graphs are inspected without pretending they have an Atmos config blob, are never
// repackaged, and are transferred with every original manifest, blob, and referrer unchanged, so
// their resolved digests and signatures survive registry, layout, and second-registry hops.
//
// Signature evidence is stored as Sigstore bundle referrers. A Record signature binds the Record
// to the addressed manifest's config descriptor; an image signature binds to the original
// manifest or index digest. Cryptography is delegated to an injected [artifact.Verifier]; this
// package neither imports Sigstore libraries nor implements cryptography.
//
// The package registers nothing by itself. A composition root calls [Register] explicitly, and the
// go-containerregistry and ORAS types never appear in this package's API.
package oci
