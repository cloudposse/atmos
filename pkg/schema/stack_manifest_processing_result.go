package schema

// StackManifestProcessingResult is the result of recursively processing a stack
// manifest file and deep-merging all its imports.
type StackManifestProcessingResult struct {
	// ImportMetadata preserves literal metadata separately from rendered configuration.
	ImportMetadata StackImportMetadata
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

// StackImportLabels holds literal stack labels used before template/function evaluation.
// Reset masks lower-precedence labels when an entire label map is null or dynamic.
// Nil values mask individual labels whose value cannot be determined at this stage.
type StackImportLabels struct {
	Values map[string]any
	Reset  bool
}

// StackImportMetadata preserves literal selector inputs independently of rendered configuration.
type StackImportMetadata struct {
	Labels StackImportLabels
	// Tags contains a tags entry only when the manifest declares it. Null masks inherited tags.
	Tags map[string]any
}
