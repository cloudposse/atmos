// Package manifest provides a reusable model for Kubernetes-style Atmos
// configuration manifests (apiVersion/kind/metadata/spec).
//
// Kinds are registered once (typically in an init function) with a Go
// prototype for their spec. Registration automatically derives a JSON Schema
// from the prototype via reflection, and every manifest loaded through this
// package is validated against that schema before it is decoded into the
// typed envelope. This replaces per-kind hand-rolled envelope structs and
// string-equality kind checks with a single, schema-validated pipeline.
package manifest

// DefaultAPIVersion is the apiVersion used by all current Atmos manifests.
const DefaultAPIVersion = "atmos/v1"

// Metadata identifies a manifest, mirroring the Kubernetes object metadata
// convention. Name is required; the remaining fields are optional,
// human-oriented annotations.
type Metadata struct {
	Name        string `yaml:"name" json:"name" jsonschema:"description=Unique name of this manifest,minLength=1"`
	Description string `yaml:"description,omitempty" json:"description,omitempty" jsonschema:"description=Human-readable description"`
	Author      string `yaml:"author,omitempty" json:"author,omitempty" jsonschema:"description=Author or maintainer"`
	Version     string `yaml:"version,omitempty" json:"version,omitempty" jsonschema:"description=Version of this manifest"`
}

// Manifest is the generic Kubernetes-style envelope shared by all Atmos
// manifest kinds. S is the kind-specific spec type registered for the kind.
type Manifest[S any] struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Metadata   Metadata `yaml:"metadata" json:"metadata"`
	Spec       S        `yaml:"spec,omitempty" json:"spec,omitempty"`
}

// envelopeProbe is used to sniff the apiVersion and kind of a manifest
// without decoding the spec.
type envelopeProbe struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}
