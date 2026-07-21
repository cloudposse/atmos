package datafetcher

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file is the "you can't forget the schema" ratchet. Every stack-manifest section is
// declared in Atmos via a `*SectionName` constant in `pkg/config/const.go`. When a new section
// is added there (and wired into the stack processor) the JSON Schema for stack manifests must
// gain a matching property, or editors and `atmos validate stacks` silently reject — or fail to
// validate — the new capability. Historically that step was forgotten (e.g. `secrets`, and a
// partial `auth`). These tests fail the build when a new `*SectionName` constant is added
// without (a) classifying it and (b) modeling it in the manifest schema.
//
// The embedded schema (pkg/datafetcher/schema/atmos/manifest/1.0.json) is the single source of
// truth for the atmos-manifest JSON Schema. The website copy served at atmos.tools is generated
// from it at build time (see `atmos stack schema`) and is not checked separately here.

const (
	constsRelPath        = "../config/const.go"
	embeddedSchemaSource = "atmos://schema/atmos/manifest/1.0"
)

// sectionScope records where a stack-manifest section may legitimately appear, so the guard can
// assert the JSON schema models it at the right level(s).
type sectionScope struct {
	topLevel  bool // Appears at the root of a stack manifest (e.g. `vars:`, `secrets:`).
	component bool // Appears under `components.<type>.<name>` (e.g. `metadata:`, `secrets:`).
}

// manifestSections maps a section's string value to where it appears in stack manifests. Every
// entry MUST be modeled in the manifest JSON schema (see knownSchemaGaps for tracked exceptions).
// Keep this in sync with the extraction whitelist in internal/exec/stack_processor_process_stacks.go
// (top-level) and stack_processor_process_stacks_helpers_extraction.go (component-level).
var manifestSections = map[string]sectionScope{
	"import":                    {topLevel: true},
	"name":                      {topLevel: true},
	"terraform":                 {topLevel: true},
	"helmfile":                  {topLevel: true},
	"packer":                    {topLevel: true},
	"ansible":                   {topLevel: true},
	"kubernetes":                {topLevel: true},
	"helm":                      {topLevel: true},
	"components":                {topLevel: true},
	"overrides":                 {topLevel: true},
	"workflows":                 {topLevel: true},
	"vars":                      {topLevel: true, component: true},
	"env":                       {topLevel: true, component: true},
	"settings":                  {topLevel: true, component: true},
	"locals":                    {topLevel: true, component: true},
	"hooks":                     {topLevel: true, component: true},
	"test":                      {component: true},
	"mocks":                     {component: true},
	"generate":                  {topLevel: true, component: true},
	"dependencies":              {topLevel: true, component: true},
	"auth":                      {topLevel: true, component: true},
	"secrets":                   {topLevel: true, component: true},
	"metadata":                  {component: true},
	"component":                 {component: true},
	"command":                   {component: true},
	"providers":                 {component: true},
	"backend":                   {component: true},
	"backend_type":              {component: true},
	"remote_state_backend":      {component: true},
	"remote_state_backend_type": {component: true},
	"provision":                 {component: true},
	"source":                    {component: true},
	"version":                   {topLevel: true},
}

// nonManifestSections are `*SectionName` constants that are NOT authored stack-manifest sections
// (derived/describe output keys, atmos.yaml-only keys, or nested sub-fields). They are exempt
// from the schema-coverage requirement, but listing them here is mandatory: a new `*SectionName`
// constant absent from BOTH this set and manifestSections fails TestSchemaCoversAllSections.
var nonManifestSections = map[string]struct{}{
	"template":           {}, // Packer template sub-field.
	"playbook":           {}, // Ansible sub-field.
	"inventory":          {}, // Ansible sub-field.
	"provider":           {}, // Kubernetes component sub-field (manifest provider engine).
	"paths":              {}, // Kubernetes component sub-field (manifest paths).
	"manifests":          {}, // Kubernetes component sub-field (inline manifests).
	"render":             {}, // Kubernetes component sub-field (render output config).
	"chart":              {}, // Native Helm component sub-field (chart reference).
	"values":             {}, // Native Helm component sub-field (inline chart values).
	"values_files":       {}, // Native Helm component sub-field (chart values file paths).
	"repositories":       {}, // Native Helm component sub-field (chart repositories).
	"plugins":            {}, // Helm/Helmfile component sub-field (Helm CLI plugins list).
	"workspace":          {}, // Terraform workspace (derived/metadata).
	"required_version":   {}, // Introspected from Terraform, not authored.
	"required_providers": {}, // Introspected from Terraform, not authored.
	"inheritance":        {}, // Describe output.
	"integrations":       {}, // atmos.yaml / describe output.
	"github":             {}, // Sub-field of integrations.
	"process_env":        {}, // Describe output (resolved process env).
	"cli_args":           {}, // Describe output.
	"retry":              {}, // Workflow/source retry sub-field, not a manifest section.
	"tf_cli_vars":        {}, // Derived Terraform CLI vars.
	"env_tf_cli_args":    {}, // Derived Terraform CLI env.
	"env_tf_cli_vars":    {}, // Derived Terraform CLI env.
	"component_type":     {}, // Describe output.
	"outputs":            {}, // Describe output.
	"static":             {}, // Describe output (static remote state).
	"component_path":     {}, // Describe output.
	"inherits":           {}, // metadata sub-field, not a top-level section.
	"abstract":           {}, // metadata sub-field, not a top-level section.
	"container":          {}, // Component-type key; container components are authored under `components.container.<name>` and modeled via the components schema, not as a standalone top-level section.
	"emulator":           {}, // Component-type key; emulator components are authored under `components.emulator.<name>` and modeled via the components schema, not as a standalone top-level section.
}

// knownSchemaGaps tracks sections that SHOULD be in the manifest schema but currently are not.
// Each key is "topLevel:<section>" or "component:<section>". This is deliberate, reviewed
// technical debt — NOT an escape hatch for new sections. New sections must be modeled in the
// schema, not added here.
//
// TODO(schema-reconciliation): close these gaps and delete the entries.
//   - top-level `ansible` and global `auth` are not yet modeled (only component-level auth is).
//   - native Helm is not yet modeled: top-level `helm` (default config for helm components, peer
//     of `helmfile`/`kubernetes`) and the `helm_component_manifest` definition are missing.
//     Tracked until the native-Helm manifest schema lands.
var knownSchemaGaps = map[string]struct{}{
	"topLevel:ansible": {},
	"topLevel:auth":    {},
	"topLevel:helm":    {},
}

// componentManifestDefs are the per-component-type manifest definitions whose `properties` model
// component-level sections.
var componentManifestDefs = []string{
	"terraform_component_manifest",
	"helmfile_component_manifest",
	"packer_component_manifest",
}

// TestEverySectionConstantIsClassified fails when a `*SectionName` constant in pkg/config/const.go
// is in neither manifestSections nor nonManifestSections. This is the forcing function: a new
// section constant cannot pass CI until a human decides whether it belongs in the manifest schema.
func TestEverySectionConstantIsClassified(t *testing.T) {
	consts := parseSectionConstants(t)
	require.NotEmpty(t, consts, "expected to find *SectionName constants in %s", constsRelPath)

	for name, value := range consts {
		_, isManifest := manifestSections[value]
		_, isNonManifest := nonManifestSections[value]
		require.Falsef(t, isManifest && isNonManifest,
			"%s (%q) is in BOTH manifestSections and nonManifestSections — pick one", name, value)
		require.Truef(t, isManifest || isNonManifest,
			"new section constant %s (%q) is unclassified.\n"+
				"Classify it in schema_section_coverage_test.go:\n"+
				"  - if it is an authored stack-manifest section: add it to manifestSections AND model it in the manifest JSON schema, or\n"+
				"  - if it is a derived/describe/atmos.yaml-only key: add it to nonManifestSections.",
			name, value)
	}
}

// TestSchemaCoversAllSections asserts every manifest section is modeled in the embedded manifest
// schema (the single source of truth; the website copy is generated from it) at its declared
// level, except for tracked knownSchemaGaps.
func TestSchemaCoversAllSections(t *testing.T) {
	schema := loadEmbeddedSchema(t)
	topLevel := topLevelProps(t, schema)
	component := componentProps(t, schema)

	for value, scope := range manifestSections {
		if scope.topLevel {
			assertCovered(t, topLevel, "topLevel", value)
		}
		if scope.component {
			assertCovered(t, component, "component", value)
		}
	}
}

// assertCovered checks that a section property exists in the given property set, unless the
// (level, section) tuple is a tracked known gap. A section listed as a known gap that is actually
// present fails too — so closing a gap forces removing its allowlist entry.
func assertCovered(t *testing.T, props map[string]struct{}, level, section string) {
	t.Helper()
	_, present := props[section]
	gapKey := level + ":" + section
	_, isGap := knownSchemaGaps[gapKey]

	if isGap {
		require.Falsef(t, present,
			"%q is present in the manifest schema at %s level but is still listed in knownSchemaGaps — remove the %q entry",
			section, level, gapKey)
		return
	}
	require.Truef(t, present,
		"section %q is missing from the manifest schema at %s level.\n"+
			"Add a %q property to pkg/datafetcher/schema/atmos/manifest/1.0.json (see how `auth`/`secrets` are wired), or — only if this is reviewed, intentional debt — add %q to knownSchemaGaps.",
		section, level, section, gapKey)
}

// parseSectionConstants AST-parses pkg/config/const.go and returns a map of constant name to its
// string value for every constant whose name ends with "SectionName".
func parseSectionConstants(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, constsRelPath, nil, 0)
	require.NoErrorf(t, err, "failed to parse %s", constsRelPath)

	out := map[string]string{}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			collectSectionConst(spec, out)
		}
	}
	return out
}

// collectSectionConst extracts a single `Name = "value"` const into out when Name ends with
// "SectionName" and the value is a string literal.
func collectSectionConst(spec ast.Spec, out map[string]string) {
	valueSpec, ok := spec.(*ast.ValueSpec)
	if !ok {
		return
	}
	for i, name := range valueSpec.Names {
		if !strings.HasSuffix(name.Name, "SectionName") || i >= len(valueSpec.Values) {
			continue
		}
		lit, ok := valueSpec.Values[i].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		out[name.Name] = strings.Trim(lit.Value, "`\"")
	}
}

// loadEmbeddedSchema returns the parsed embedded manifest schema (used by `atmos validate stacks`
// and generated into the website copy served at atmos.tools).
func loadEmbeddedSchema(t *testing.T) map[string]any {
	t.Helper()
	data, err := (&atmosFetcher{}).FetchData(embeddedSchemaSource)
	require.NoError(t, err, "failed to fetch embedded manifest schema")
	var schema map[string]any
	require.NoError(t, json.Unmarshal(data, &schema), "failed to parse embedded manifest schema")
	return schema
}

// topLevelProps returns the set of top-level property names declared in the manifest schema.
func topLevelProps(t *testing.T, schema map[string]any) map[string]struct{} {
	t.Helper()
	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "schema should have top-level properties")
	return keySet(props)
}

// componentProps returns the union of property names across all component-manifest definitions.
func componentProps(t *testing.T, schema map[string]any) map[string]struct{} {
	t.Helper()
	defs, ok := schema["definitions"].(map[string]any)
	require.True(t, ok, "schema should have definitions")

	union := map[string]struct{}{}
	for _, defName := range componentManifestDefs {
		def, ok := defs[defName].(map[string]any)
		require.Truef(t, ok, "schema should define %s", defName)
		for k := range objectVariantProps(def) {
			union[k] = struct{}{}
		}
	}
	return union
}

// objectVariantProps returns the properties of the object variant of a definition, accounting for
// the common `oneOf: [ {string !include}, {object ...} ]` shape used throughout the schema.
func objectVariantProps(def map[string]any) map[string]struct{} {
	if props, ok := def["properties"].(map[string]any); ok {
		return keySet(props)
	}
	oneOf, ok := def["oneOf"].([]any)
	if !ok {
		return map[string]struct{}{}
	}
	for _, variant := range oneOf {
		v, ok := variant.(map[string]any)
		if !ok || v["type"] != "object" {
			continue
		}
		if props, ok := v["properties"].(map[string]any); ok {
			return keySet(props)
		}
	}
	return map[string]struct{}{}
}

// keySet returns the set of keys of a map as a set of strings.
func keySet(m map[string]any) map[string]struct{} {
	out := make(map[string]struct{}, len(m))
	for k := range m {
		out[k] = struct{}{}
	}
	return out
}
