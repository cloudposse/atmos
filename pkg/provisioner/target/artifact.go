// Package target implements provision targets: reusable delivery destinations
// (kinds) that publish a rendered ProvisionArtifact to a place such as a Git
// repository, an OCI registry, or a Kubernetes cluster.
//
// A target is intentionally decoupled from the component type that produced the
// artifact: a Kubernetes component renders a "kubernetes.manifests" artifact and
// a future Docker component would render an "oci.image" artifact, yet both can be
// delivered by the same registered targets. The registry therefore lives in this
// neutral package, consumes a producer-agnostic ProvisionArtifact, and must not
// import any component package.
package target

// Artifact kinds produced by component types and consumed by target provisioners.
const (
	// ArtifactKindKubernetesManifests is a stream of rendered Kubernetes objects.
	ArtifactKindKubernetesManifests = "kubernetes.manifests"
)

// Artifact formats describe how ProvisionArtifact.Files content is encoded.
const (
	// FormatYAML indicates UTF-8 YAML file content.
	FormatYAML = "yaml"
	// FormatJSON indicates UTF-8 JSON file content.
	FormatJSON = "json"
	// FormatText indicates arbitrary UTF-8 text content.
	FormatText = "text"
	// FormatDirectory indicates an opaque set of files forming a directory tree.
	FormatDirectory = "directory"
)

// ArtifactMetadata carries provenance for a rendered artifact. Targets use it
// for commit messages, trailers, and idempotency checks.
type ArtifactMetadata struct {
	// Component is the Atmos component instance name.
	Component string
	// Stack is the Atmos stack the artifact was rendered for.
	Stack string
	// Target is the name of the selected provision target.
	Target string
	// SourcePaths are the component source paths the artifact was rendered from.
	SourcePaths []string
	// RenderHash is a deterministic digest of Files, used for change detection.
	RenderHash string
}

// ProvisionArtifact is the producer-agnostic unit a target provisioner delivers.
// A component type renders one of these; a target publishes it without knowing
// how it was produced.
type ProvisionArtifact struct {
	// Kind identifies what was rendered (e.g. ArtifactKindKubernetesManifests).
	Kind string
	// Format describes how Files content is encoded (e.g. FormatYAML).
	Format string
	// Files maps deterministic repo-relative paths to their content.
	Files map[string][]byte
	// Metadata carries provenance for commit messages, trailers, and change detection.
	Metadata ArtifactMetadata
}
