package schema

// StackManifestProcessingResult is the result of recursively processing a stack
// manifest file and deep-merging all its imports.
type StackManifestProcessingResult struct {
	// DeepMergedConfig is the final stack configuration after the manifest and
	// all its imports are deep-merged.
	DeepMergedConfig map[string]any
	// ImportsConfig maps each processed import path (without extension) to its
	// resolved configuration.
	ImportsConfig map[string]map[string]any
	// StackConfig is the raw configuration of the stack manifest itself, before
	// the imports are merged into it.
	StackConfig map[string]any
	// The four overrides maps carry the accumulated top-level `overrides`
	// sections through the recursive import chain, tracked separately for
	// Terraform and Helmfile and split into the manifest's own inline section
	// vs. the sections merged from its imports, so inline overrides take
	// precedence over imported ones.
	TerraformOverridesInline  map[string]any
	TerraformOverridesImports map[string]any
	HelmfileOverridesInline   map[string]any
	HelmfileOverridesImports  map[string]any
}
