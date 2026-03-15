package exec

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ---------------------------------------------------------------------------
// extractDescribeComponentSections
// ---------------------------------------------------------------------------

func TestExtractDescribeComponentSections_AllPresent(t *testing.T) {
	vars := map[string]any{"key": "val"}
	meta := map[string]any{"component": "base"}
	settings := map[string]any{"spacelift": true}
	env := map[string]any{"TF_VAR_x": "1"}
	authMap := map[string]any{"role": "arn:aws"}
	providers := map[string]any{"aws": map[string]any{}}
	hooks := map[string]any{"pre_plan": "script.sh"}
	overrides := map[string]any{"env": map[string]any{}}
	backend := map[string]any{"bucket": "my-bucket"}

	cs := map[string]any{
		cfg.VarsSectionName:        vars,
		cfg.MetadataSectionName:    meta,
		cfg.SettingsSectionName:    settings,
		cfg.EnvSectionName:         env,
		cfg.AuthSectionName:        authMap,
		cfg.ProvidersSectionName:   providers,
		cfg.HooksSectionName:       hooks,
		cfg.OverridesSectionName:   overrides,
		cfg.BackendSectionName:     backend,
		cfg.BackendTypeSectionName: "s3",
	}

	secs := extractDescribeComponentSections(cs)

	assert.Equal(t, vars, secs.vars)
	assert.Equal(t, meta, secs.metadata)
	assert.Equal(t, settings, secs.settings)
	assert.Equal(t, env, secs.env)
	assert.Equal(t, authMap, secs.auth)
	assert.Equal(t, providers, secs.providers)
	assert.Equal(t, hooks, secs.hooks)
	assert.Equal(t, overrides, secs.overrides)
	assert.Equal(t, backend, secs.backend)
	assert.Equal(t, "s3", secs.backendType)
}

func TestExtractDescribeComponentSections_Empty(t *testing.T) {
	secs := extractDescribeComponentSections(map[string]any{})

	assert.Equal(t, map[string]any{}, secs.vars)
	assert.Equal(t, map[string]any{}, secs.metadata)
	assert.Equal(t, map[string]any{}, secs.settings)
	assert.Equal(t, map[string]any{}, secs.env)
	assert.Equal(t, map[string]any{}, secs.auth)
	assert.Equal(t, map[string]any{}, secs.providers)
	assert.Equal(t, map[string]any{}, secs.hooks)
	assert.Equal(t, map[string]any{}, secs.overrides)
	assert.Equal(t, map[string]any{}, secs.backend)
	assert.Equal(t, "", secs.backendType)
}

func TestExtractDescribeComponentSections_WrongTypes(t *testing.T) {
	// When a section has the wrong type it should fall back to an empty map.
	cs := map[string]any{
		cfg.VarsSectionName:        "not-a-map",
		cfg.MetadataSectionName:    42,
		cfg.BackendTypeSectionName: 999, // wrong type → empty string
	}

	secs := extractDescribeComponentSections(cs)

	assert.Equal(t, map[string]any{}, secs.vars)
	assert.Equal(t, map[string]any{}, secs.metadata)
	assert.Equal(t, "", secs.backendType)
}

// ---------------------------------------------------------------------------
// buildConfigAndStacksInfo
// ---------------------------------------------------------------------------

func TestBuildConfigAndStacksInfo(t *testing.T) {
	secs := componentSections{
		vars:        map[string]any{"region": "us-east-1"},
		metadata:    map[string]any{"component": "base"},
		settings:    map[string]any{"spacelift": true},
		env:         map[string]any{"ENV": "prod"},
		auth:        map[string]any{},
		providers:   map[string]any{},
		hooks:       map[string]any{},
		overrides:   map[string]any{},
		backend:     map[string]any{"bucket": "tfstate"},
		backendType: "s3",
	}

	info := buildConfigAndStacksInfo("my-component", "stacks/prod.yaml", "prod", secs)

	assert.Equal(t, "my-component", info.ComponentFromArg)
	assert.Equal(t, "stacks/prod.yaml", info.Stack)
	assert.Equal(t, "prod", info.StackManifestName)
	assert.Equal(t, secs.vars, info.ComponentVarsSection)
	assert.Equal(t, secs.metadata, info.ComponentMetadataSection)
	assert.Equal(t, secs.settings, info.ComponentSettingsSection)
	assert.Equal(t, secs.env, info.ComponentEnvSection)
	assert.Equal(t, secs.auth, info.ComponentAuthSection)
	assert.Equal(t, secs.providers, info.ComponentProvidersSection)
	assert.Equal(t, secs.hooks, info.ComponentHooksSection)
	assert.Equal(t, secs.overrides, info.ComponentOverridesSection)
	assert.Equal(t, secs.backend, info.ComponentBackendSection)
	assert.Equal(t, "s3", info.ComponentBackendType)

	// ComponentSection mirror.
	assert.Equal(t, secs.vars, info.ComponentSection[cfg.VarsSectionName])
	assert.Equal(t, secs.metadata, info.ComponentSection[cfg.MetadataSectionName])
	assert.Equal(t, "s3", info.ComponentSection[cfg.BackendTypeSectionName])
}

// ---------------------------------------------------------------------------
// resolveStackName
// ---------------------------------------------------------------------------

func TestResolveStackName_ManifestName(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{}

	name, err := resolveStackName(ac, "stacks/prod.yaml", "my-manifest-name", info, nil)

	require.NoError(t, err)
	assert.Equal(t, "my-manifest-name", name)
}

func TestResolveStackName_NoNaming(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{}

	name, err := resolveStackName(ac, "stacks/prod.yaml", "", info, nil)

	require.NoError(t, err)
	assert.Equal(t, "stacks/prod.yaml", name)
}

func TestResolveStackName_Pattern(t *testing.T) {
	ac := &schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			NamePattern: "{tenant}-{environment}-{stage}",
		},
	}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}
	vars := map[string]any{
		"tenant":      "corp",
		"environment": "us-east-1",
		"stage":       "prod",
	}

	name, err := resolveStackName(ac, "stacks/corp-us-east-1-prod.yaml", "", info, vars)

	require.NoError(t, err)
	assert.Equal(t, "corp-us-east-1-prod", name)
}

func TestResolveStackName_NameTemplate(t *testing.T) {
	// Use a literal template that returns a fixed string.
	ac := &schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			NameTemplate: "hardcoded-stack-name",
		},
	}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}

	name, err := resolveStackName(ac, "stacks/prod.yaml", "", info, nil)

	require.NoError(t, err)
	assert.Equal(t, "hardcoded-stack-name", name)
}

func TestResolveStackName_NameTemplateError(t *testing.T) {
	// An invalid Go template should return an error.
	ac := &schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			NameTemplate: "{{.missing_open",
		},
	}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}

	_, err := resolveStackName(ac, "stacks/prod.yaml", "", info, nil)

	require.Error(t, err)
}

func TestResolveStackName_PatternValidationFallback(t *testing.T) {
	// When the pattern doesn't match the filename, it falls back to the filename.
	ac := &schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			NamePattern: "{tenant}-{environment}-{stage}",
		},
	}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}
	vars := map[string]any{
		"tenant": "corp",
		// missing environment and stage → pattern fails
	}

	name, err := resolveStackName(ac, "stacks/corp.yaml", "", info, vars)

	require.NoError(t, err)
	assert.Equal(t, "stacks/corp.yaml", name)
}

// ---------------------------------------------------------------------------
// shouldFilterByStack
// ---------------------------------------------------------------------------

func TestShouldFilterByStack(t *testing.T) {
	tests := []struct {
		name          string
		filterByStack string
		stackFileName string
		stackName     string
		wantSkip      bool
	}{
		{"no filter", "", "stacks/prod.yaml", "prod", false},
		{"matches filename", "stacks/prod.yaml", "stacks/prod.yaml", "prod", false},
		{"matches resolved name", "prod", "stacks/prod.yaml", "prod", false},
		{"no match", "dev", "stacks/prod.yaml", "prod", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantSkip, shouldFilterByStack(tc.filterByStack, tc.stackFileName, tc.stackName))
		})
	}
}

// ---------------------------------------------------------------------------
// ensureComponentEntryInMap
// ---------------------------------------------------------------------------

func TestEnsureComponentEntryInMap_CreatesAllLevels(t *testing.T) {
	finalMap := map[string]any{
		"prod": map[string]any{},
	}

	ensureComponentEntryInMap(finalMap, "prod", "terraform", "vpc")

	prodEntry := finalMap["prod"].(map[string]any)
	comps := prodEntry[cfg.ComponentsSectionName].(map[string]any)
	tf := comps["terraform"].(map[string]any)
	assert.NotNil(t, tf["vpc"])
}

func TestEnsureComponentEntryInMap_IdempotentForExistingEntry(t *testing.T) {
	finalMap := map[string]any{
		"prod": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"existing": true},
					},
				},
			},
		},
	}

	ensureComponentEntryInMap(finalMap, "prod", "terraform", "vpc")

	// Existing content should be preserved.
	comps := finalMap["prod"].(map[string]any)[cfg.ComponentsSectionName].(map[string]any)
	vpc := comps["terraform"].(map[string]any)["vpc"].(map[string]any)
	assert.Equal(t, map[string]any{"existing": true}, vpc["vars"])
}

// ---------------------------------------------------------------------------
// setAtmosComponentMetadata
// ---------------------------------------------------------------------------

func TestSetAtmosComponentMetadata(t *testing.T) {
	section := map[string]any{}
	setAtmosComponentMetadata(section, "my-comp", "prod", "stacks/prod.yaml")

	assert.Equal(t, "my-comp", section["atmos_component"])
	assert.Equal(t, "prod", section["atmos_stack"])
	assert.Equal(t, "prod", section["stack"])
	assert.Equal(t, "stacks/prod.yaml", section["atmos_stack_file"])
	assert.Equal(t, "stacks/prod.yaml", section["atmos_manifest"])
}

// ---------------------------------------------------------------------------
// resolveIncludeEmpty
// ---------------------------------------------------------------------------

func TestResolveIncludeEmpty_DefaultTrue(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	assert.True(t, resolveIncludeEmpty(ac, true))
}

func TestResolveIncludeEmpty_ConfigFalse(t *testing.T) {
	f := false
	ac := &schema.AtmosConfiguration{
		Describe: schema.Describe{
			Settings: schema.DescribeSettings{IncludeEmpty: &f},
		},
	}
	assert.False(t, resolveIncludeEmpty(ac, true))
}

func TestResolveIncludeEmpty_ConfigTrue(t *testing.T) {
	tr := true
	ac := &schema.AtmosConfiguration{
		Describe: schema.Describe{
			Settings: schema.DescribeSettings{IncludeEmpty: &tr},
		},
	}
	assert.True(t, resolveIncludeEmpty(ac, true))
}

func TestResolveIncludeEmpty_CheckFalseAlwaysTrue(t *testing.T) {
	// When checkIncludeEmpty is false (non-terraform types), always true regardless of config.
	f := false
	ac := &schema.AtmosConfiguration{
		Describe: schema.Describe{
			Settings: schema.DescribeSettings{IncludeEmpty: &f},
		},
	}
	assert.True(t, resolveIncludeEmpty(ac, false))
}

// ---------------------------------------------------------------------------
// addSectionsToComponentEntry
// ---------------------------------------------------------------------------

func TestAddSectionsToComponentEntry_AllSections(t *testing.T) {
	dest := map[string]any{}
	src := map[string]any{
		"vars":     map[string]any{"a": 1},
		"settings": map[string]any{},
	}

	addSectionsToComponentEntry(dest, src, nil, true)

	assert.Equal(t, map[string]any{"a": 1}, dest["vars"])
	assert.Equal(t, map[string]any{}, dest["settings"])
}

func TestAddSectionsToComponentEntry_FilterSections(t *testing.T) {
	dest := map[string]any{}
	src := map[string]any{
		"vars":     map[string]any{"a": 1},
		"settings": map[string]any{"s": 2},
		"env":      map[string]any{"E": "v"},
	}

	addSectionsToComponentEntry(dest, src, []string{"vars", "env"}, true)

	assert.Contains(t, dest, "vars")
	assert.Contains(t, dest, "env")
	assert.NotContains(t, dest, "settings")
}

func TestAddSectionsToComponentEntry_SkipsEmptyMapsWhenIncludeEmptyFalse(t *testing.T) {
	dest := map[string]any{}
	src := map[string]any{
		"vars":     map[string]any{},             // empty map → should be skipped
		"settings": map[string]any{"key": "val"}, // non-empty → should be included
		"label":    "not-a-map",                  // non-map → always included
	}

	addSectionsToComponentEntry(dest, src, nil, false)

	assert.NotContains(t, dest, "vars")
	assert.Contains(t, dest, "settings")
	assert.Contains(t, dest, "label")
}

func TestAddSectionsToComponentEntry_IncludesEmptyMapsWhenIncludeEmptyTrue(t *testing.T) {
	dest := map[string]any{}
	src := map[string]any{
		"vars": map[string]any{},
	}

	addSectionsToComponentEntry(dest, src, nil, true)

	assert.Contains(t, dest, "vars")
}

// ---------------------------------------------------------------------------
// hasStackExplicitComponents
// ---------------------------------------------------------------------------

func TestHasStackExplicitComponents_WithTerraform(t *testing.T) {
	stack := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.TerraformSectionName: map[string]any{
				"vpc": map[string]any{},
			},
		},
	}
	assert.True(t, hasStackExplicitComponents(stack))
}

func TestHasStackExplicitComponents_EmptyTerraform(t *testing.T) {
	stack := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.TerraformSectionName: map[string]any{},
		},
	}
	assert.False(t, hasStackExplicitComponents(stack))
}

func TestHasStackExplicitComponents_Helmfile(t *testing.T) {
	stack := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.HelmfileSectionName: map[string]any{
				"chart": map[string]any{},
			},
		},
	}
	assert.True(t, hasStackExplicitComponents(stack))
}

func TestHasStackExplicitComponents_Packer(t *testing.T) {
	stack := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.PackerSectionName: map[string]any{
				"ami": map[string]any{},
			},
		},
	}
	assert.True(t, hasStackExplicitComponents(stack))
}

func TestHasStackExplicitComponents_Ansible(t *testing.T) {
	stack := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.AnsibleSectionName: map[string]any{
				"playbook": map[string]any{},
			},
		},
	}
	assert.True(t, hasStackExplicitComponents(stack))
}

func TestHasStackExplicitComponents_NoComponents(t *testing.T) {
	assert.False(t, hasStackExplicitComponents(map[string]any{}))
}

func TestHasStackExplicitComponents_NilComponents(t *testing.T) {
	stack := map[string]any{
		cfg.ComponentsSectionName: nil,
	}
	assert.False(t, hasStackExplicitComponents(stack))
}

func TestHasStackExplicitComponents_WrongType(t *testing.T) {
	stack := map[string]any{
		cfg.ComponentsSectionName: "not-a-map",
	}
	assert.False(t, hasStackExplicitComponents(stack))
}

// ---------------------------------------------------------------------------
// hasStackImports
// ---------------------------------------------------------------------------

func TestHasStackImports_WithImports(t *testing.T) {
	stack := map[string]any{
		"import": []any{"stacks/catalog/vpc.yaml"},
	}
	assert.True(t, hasStackImports(stack))
}

func TestHasStackImports_EmptyImports(t *testing.T) {
	stack := map[string]any{
		"import": []any{},
	}
	assert.False(t, hasStackImports(stack))
}

func TestHasStackImports_NoImports(t *testing.T) {
	assert.False(t, hasStackImports(map[string]any{}))
}

// ---------------------------------------------------------------------------
// stackHasNonEmptyComponents
// ---------------------------------------------------------------------------

func TestStackHasNonEmptyComponents_WithVars(t *testing.T) {
	comps := map[string]any{
		"terraform": map[string]any{
			"vpc": map[string]any{
				"vars": map[string]any{"cidr": "10.0.0.0/8"},
			},
		},
	}
	assert.True(t, stackHasNonEmptyComponents(comps))
}

func TestStackHasNonEmptyComponents_WithWorkspace(t *testing.T) {
	comps := map[string]any{
		"terraform": map[string]any{
			"vpc": map[string]any{
				"workspace": "prod",
			},
		},
	}
	assert.True(t, stackHasNonEmptyComponents(comps))
}

func TestStackHasNonEmptyComponents_WithMetadata(t *testing.T) {
	comps := map[string]any{
		"terraform": map[string]any{
			"vpc": map[string]any{
				"metadata": map[string]any{"component": "base"},
			},
		},
	}
	assert.True(t, stackHasNonEmptyComponents(comps))
}

func TestStackHasNonEmptyComponents_WithSettings(t *testing.T) {
	comps := map[string]any{
		"terraform": map[string]any{
			"vpc": map[string]any{
				"settings": map[string]any{"spacelift": true},
			},
		},
	}
	assert.True(t, stackHasNonEmptyComponents(comps))
}

func TestStackHasNonEmptyComponents_WithEnv(t *testing.T) {
	comps := map[string]any{
		"helmfile": map[string]any{
			"chart": map[string]any{
				"env": map[string]any{"KEY": "val"},
			},
		},
	}
	assert.True(t, stackHasNonEmptyComponents(comps))
}

func TestStackHasNonEmptyComponents_NoRelevantSections(t *testing.T) {
	comps := map[string]any{
		"terraform": map[string]any{
			"vpc": map[string]any{
				"some_other_key": "value",
			},
		},
	}
	assert.False(t, stackHasNonEmptyComponents(comps))
}

func TestStackHasNonEmptyComponents_EmptyTypes(t *testing.T) {
	comps := map[string]any{
		"terraform": map[string]any{},
	}
	assert.False(t, stackHasNonEmptyComponents(comps))
}

func TestStackHasNonEmptyComponents_ComponentEntryNotMap(t *testing.T) {
	// When an individual component entry is not a map, it should be skipped.
	comps := map[string]any{
		"terraform": map[string]any{
			"vpc": "not-a-map", // non-map component entry
		},
	}
	assert.False(t, stackHasNonEmptyComponents(comps))
}

func TestStackHasNonEmptyComponents_WrongTypes(t *testing.T) {
	comps := map[string]any{
		"terraform": "not-a-map",
	}
	assert.False(t, stackHasNonEmptyComponents(comps))
}

// ---------------------------------------------------------------------------
// filterEmptyFinalStacks
// ---------------------------------------------------------------------------

func TestFilterEmptyFinalStacks_RemovesEmpty(t *testing.T) {
	finalMap := map[string]any{
		"prod": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"cidr": "10.0.0.0/8"},
					},
				},
			},
		},
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"some_key": "but no relevant sections",
					},
				},
			},
		},
	}

	err := filterEmptyFinalStacks(finalMap, false)

	require.NoError(t, err)
	assert.Contains(t, finalMap, "prod")
	assert.NotContains(t, finalMap, "dev")
}

func TestFilterEmptyFinalStacks_RemovesEmptyStackName(t *testing.T) {
	finalMap := map[string]any{
		"": map[string]any{},
	}

	err := filterEmptyFinalStacks(finalMap, false)

	require.NoError(t, err)
	assert.NotContains(t, finalMap, "")
}

func TestFilterEmptyFinalStacks_RemovesNoComponentsSection(t *testing.T) {
	finalMap := map[string]any{
		"prod": map[string]any{
			"other_key": "value",
		},
	}

	err := filterEmptyFinalStacks(finalMap, false)

	require.NoError(t, err)
	assert.NotContains(t, finalMap, "prod")
}

func TestFilterEmptyFinalStacks_IncludeEmptySkipsFiltering(t *testing.T) {
	finalMap := map[string]any{
		"prod": map[string]any{},
		"dev":  map[string]any{},
	}

	err := filterEmptyFinalStacks(finalMap, true)

	require.NoError(t, err)
	// Both stacks should remain because includeEmptyStacks=true.
	assert.Contains(t, finalMap, "prod")
	assert.Contains(t, finalMap, "dev")
}

func TestFilterEmptyFinalStacks_InvalidStackEntry(t *testing.T) {
	finalMap := map[string]any{
		"prod": "not-a-map",
	}

	err := filterEmptyFinalStacks(finalMap, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid stack entry type")
}

// ---------------------------------------------------------------------------
// applyTerraformMetadataInheritance – no inheritance (trivial path)
// ---------------------------------------------------------------------------

func TestApplyTerraformMetadataInheritance_NoInheritance(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	metadata := map[string]any{"component": "vpc"}

	result, err := applyTerraformMetadataInheritance(ac, map[string]any{}, "vpc", "stack.yaml", metadata)

	require.NoError(t, err)
	assert.Equal(t, metadata, result)
}

func TestApplyTerraformMetadataInheritance_EmptyInheritList(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	metadata := map[string]any{
		cfg.InheritsSectionName: []any{},
	}

	result, err := applyTerraformMetadataInheritance(ac, map[string]any{}, "vpc", "stack.yaml", metadata)

	require.NoError(t, err)
	assert.Equal(t, metadata, result)
}

func TestApplyTerraformMetadataInheritance_SkipsNonStringInherit(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	metadata := map[string]any{
		cfg.InheritsSectionName: []any{42, nil}, // non-string entries should be skipped
	}
	// allTerraformComponents has no "42" or nil component, so ProcessBaseComponentConfig would be called 0 times.
	allComponents := map[string]any{}

	result, err := applyTerraformMetadataInheritance(ac, allComponents, "vpc", "stack.yaml", metadata)

	require.NoError(t, err)
	// No merge because baseComponentMetadata is empty.
	assert.Equal(t, metadata, result)
}

// ---------------------------------------------------------------------------
// applyTerraformMetadataInheritance – with real base component
// ---------------------------------------------------------------------------

func TestApplyTerraformMetadataInheritance_WithBaseComponent(t *testing.T) {
	// Default AtmosConfiguration has metadata inheritance enabled (default=true).
	ac := &schema.AtmosConfiguration{}

	allComponents := map[string]any{
		"base-foo-comp": map[string]any{
			"metadata": map[string]any{
				"foo": "bar",
				"baz": "qux",
			},
		},
	}

	metadata := map[string]any{
		cfg.InheritsSectionName: []any{"base-foo-comp"},
		"own-key":               "own-value",
	}

	result, err := applyTerraformMetadataInheritance(
		ac, allComponents, "foo-comp", "foo-stack.yaml", metadata,
	)

	require.NoError(t, err)
	// The result should be the merged metadata (base overridden by component).
	assert.Equal(t, "bar", result["foo"])
	assert.Equal(t, "qux", result["baz"])
	assert.Equal(t, "own-value", result["own-key"])
}

func TestApplyTerraformMetadataInheritance_WorkspaceOverride(t *testing.T) {
	// When the component has an explicit terraform_workspace in merged metadata,
	// the workspace_pattern and workspace_template are deleted.
	ac := &schema.AtmosConfiguration{}

	allComponents := map[string]any{
		"base-ws-comp": map[string]any{
			"metadata": map[string]any{
				"terraform_workspace_pattern":  "{env}-{stage}",
				"terraform_workspace_template": "{{ .env }}-{{ .stage }}",
			},
		},
	}

	metadata := map[string]any{
		cfg.InheritsSectionName: []any{"base-ws-comp"},
		"terraform_workspace":   "my-custom-workspace",
	}

	result, err := applyTerraformMetadataInheritance(
		ac, allComponents, "ws-comp", "ws-stack.yaml", metadata,
	)

	require.NoError(t, err)
	assert.Equal(t, "my-custom-workspace", result["terraform_workspace"])
	// Pattern and template should be removed when explicit workspace is set.
	assert.NotContains(t, result, "terraform_workspace_pattern")
	assert.NotContains(t, result, "terraform_workspace_template")
}

// ---------------------------------------------------------------------------
// describeStacksProcessor method tests
// ---------------------------------------------------------------------------

// newMinimalProcessor creates a processor with default config and empty result map.
func newMinimalProcessor() *describeStacksProcessor {
	return newDescribeStacksProcessor(
		&schema.AtmosConfiguration{},
		"",    // filterByStack
		nil,   // components
		nil,   // componentTypes
		nil,   // sections
		false, // processTemplates
		false, // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
	)
}

func TestProcessStackFile_NonMapInput(t *testing.T) {
	// When stackSection is not a map, processStackFile should return nil.
	p := newMinimalProcessor()
	err := p.processStackFile("test.yaml", "not-a-map")
	require.NoError(t, err)
}

func TestProcessStackFile_NoComponentsSection(t *testing.T) {
	// When the stack has no "components" key, processStackFile should return nil.
	p := newMinimalProcessor()
	stackMap := map[string]any{"other_key": "value"}
	err := p.processStackFile("test.yaml", stackMap)
	require.NoError(t, err)
}

func TestProcessStackFile_ComponentTypeFilterSkipsNonMatching(t *testing.T) {
	// When componentTypes filter is set, types not in the filter should be skipped.
	p := newDescribeStacksProcessor(
		&schema.AtmosConfiguration{},
		"",
		nil,
		[]string{"helmfile"}, // only helmfile, not terraform
		nil,
		false, false, false, nil, nil,
	)

	stackMap := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.TerraformSectionName: map[string]any{
				"vpc": map[string]any{"vars": map[string]any{}},
			},
			// No helmfile components
		},
	}

	err := p.processStackFile("test.yaml", stackMap)
	require.NoError(t, err)
	// Terraform was filtered out, no components should be in finalStacksMap.
}

func TestProcessStackFile_TypeSectionNotAMap(t *testing.T) {
	// When a component type section is not a map, the type should be skipped (continue).
	p := newMinimalProcessor()
	stackMap := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.TerraformSectionName: "not-a-map", // invalid type → continue
		},
	}

	err := p.processStackFile("test.yaml", stackMap)
	require.NoError(t, err) // should not fail, just skip
}

func TestProcessStackFile_ProcessComponentTypeSectionError(t *testing.T) {
	// When a component type section contains a non-map component entry, processComponentTypeSection
	// should return an error which processStackFile propagates.
	p := newMinimalProcessor()
	stackMap := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.TerraformSectionName: map[string]any{
				"vpc": "not-a-map", // invalid component entry → error
			},
		},
	}

	err := p.processStackFile("test.yaml", stackMap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestProcessComponentTypeSection_ComponentSectionNotMap(t *testing.T) {
	// When a component entry in the type section is not a map, an error is returned.
	p := newMinimalProcessor()
	typeSection := map[string]any{
		"vpc": "not-a-map",
	}
	err := p.processComponentTypeSection(
		"test.yaml", "", cfg.TerraformSectionName, typeSection,
		processComponentTypeOpts{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestProcessComponentEntry_ComponentFilterExcluded(t *testing.T) {
	// When p.components filters out this component, processComponentEntry returns nil.
	p := newDescribeStacksProcessor(
		&schema.AtmosConfiguration{},
		"",
		[]string{"other-component"}, // filter: only "other-component"
		nil, nil, false, false, false, nil, nil,
	)

	componentSection := map[string]any{
		cfg.ComponentSectionName: "vpc",
		"vars":                   map[string]any{},
	}
	allTypeComponents := map[string]any{
		"vpc": componentSection,
	}

	err := p.processComponentEntry(
		"test.yaml", "", cfg.TerraformSectionName,
		"vpc", componentSection, allTypeComponents,
		processComponentTypeOpts{},
	)

	require.NoError(t, err)
	// "vpc" was filtered out, so nothing should appear in finalStacksMap.
	if entry, ok := p.finalStacksMap["test.yaml"]; ok {
		entryMap := entry.(map[string]any)
		if comps, ok := entryMap[cfg.ComponentsSectionName].(map[string]any); ok {
			if tf, ok := comps[cfg.TerraformSectionName].(map[string]any); ok {
				assert.NotContains(t, tf, "vpc", "vpc should have been filtered out")
			}
		}
	}
}

func TestProcessComponentEntry_EmptyStackName(t *testing.T) {
	// When stackFileName is "" and no name template/pattern, stackName falls back to "".
	// Line 223-225: stackName = stackFileName (both "").
	p := newMinimalProcessor()

	componentSection := map[string]any{
		cfg.ComponentSectionName: "vpc",
		"vars":                   map[string]any{"region": "us-east-1"},
	}
	allTypeComponents := map[string]any{
		"vpc": componentSection,
	}

	err := p.processComponentEntry(
		"", // empty stackFileName
		"", cfg.TerraformSectionName,
		"vpc", componentSection, allTypeComponents,
		processComponentTypeOpts{},
	)

	require.NoError(t, err)
	// Stack entry for "" should exist.
	assert.Contains(t, p.finalStacksMap, "")
}

func TestProcessComponentEntry_ResolveStackNameError(t *testing.T) {
	// When NameTemplate is invalid, resolveStackName returns an error.
	p := newDescribeStacksProcessor(
		&schema.AtmosConfiguration{
			Stacks: schema.Stacks{
				NameTemplate: "{{.invalid_open", // invalid Go template
			},
		},
		"", nil, nil, nil, false, false, false, nil, nil,
	)

	componentSection := map[string]any{
		cfg.ComponentSectionName: "vpc",
	}
	allTypeComponents := map[string]any{"vpc": componentSection}

	err := p.processComponentEntry(
		"test.yaml", "", cfg.TerraformSectionName,
		"vpc", componentSection, allTypeComponents,
		processComponentTypeOpts{},
	)

	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// processComponentTypeSection – additional coverage
// ---------------------------------------------------------------------------

func TestProcessComponentTypeSection_DefaultsComponentKey(t *testing.T) {
	// When a component section has no "component" key, it should be defaulted to the component name.
	// Lines 161-163 in processComponentTypeSection.
	p := newMinimalProcessor()

	typeSection := map[string]any{
		"vpc": map[string]any{
			"vars": map[string]any{"region": "us-east-1"},
			// No "component" key – should be set to "vpc" by processComponentTypeSection.
		},
	}

	err := p.processComponentTypeSection(
		"test.yaml", "", cfg.TerraformSectionName, typeSection,
		processComponentTypeOpts{},
	)

	require.NoError(t, err)
	// The component key should have been set to "vpc" in the section map.
	vpcSection := typeSection["vpc"].(map[string]any)
	assert.Equal(t, "vpc", vpcSection[cfg.ComponentSectionName])
}

func TestProcessComponentTypeSection_ProcessComponentEntryError(t *testing.T) {
	// When processComponentEntry returns an error (e.g., invalid name template),
	// processComponentTypeSection should propagate the error (lines 168-170).
	p := newDescribeStacksProcessor(
		&schema.AtmosConfiguration{
			Stacks: schema.Stacks{
				NameTemplate: "{{.bad", // invalid Go template → resolveStackName fails
			},
		},
		"", nil, nil, nil, false, false, false, nil, nil,
	)

	typeSection := map[string]any{
		"vpc": map[string]any{cfg.ComponentSectionName: "vpc"},
	}

	err := p.processComponentTypeSection(
		"test.yaml", "", cfg.TerraformSectionName, typeSection,
		processComponentTypeOpts{},
	)

	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// processComponentEntry – error path coverage
// ---------------------------------------------------------------------------

func TestProcessComponentEntry_FindComponentsDerivedError(t *testing.T) {
	// When allTypeComponents contains a non-map entry, FindComponentsDerivedFromBaseComponents
	// returns an error which processComponentEntry propagates (lines 187-189).
	p := newDescribeStacksProcessor(
		&schema.AtmosConfiguration{},
		"",
		[]string{"base-vpc"}, // non-empty components filter triggers FindComponentsDerivedFromBaseComponents
		nil, nil, false, false, false, nil, nil,
	)

	componentSection := map[string]any{
		cfg.ComponentSectionName: "vpc",
	}
	allTypeComponents := map[string]any{
		"vpc":      componentSection,
		"bad-comp": "not-a-map", // non-map entry causes FindComponentsDerivedFromBaseComponents to error
	}

	err := p.processComponentEntry(
		"test.yaml", "", cfg.TerraformSectionName,
		"vpc", componentSection, allTypeComponents,
		processComponentTypeOpts{},
	)

	require.Error(t, err)
}

func TestProcessComponentEntry_ApplyMetadataInheritanceError(t *testing.T) {
	// When the base component's metadata section is not a map, ProcessBaseComponentConfig errors,
	// which propagates through applyTerraformMetadataInheritance and processComponentEntry (lines 199-201).
	// Use unique names to avoid global BaseComponentConfig cache collisions.
	p := newDescribeStacksProcessor(
		&schema.AtmosConfiguration{}, // default: metadata inheritance enabled
		"", nil, nil, nil, false, false, false, nil, nil,
	)

	componentSection := map[string]any{
		cfg.ComponentSectionName: "inherit-error-vpc",
		cfg.MetadataSectionName: map[string]any{
			cfg.InheritsSectionName: []any{"base-inherit-error"},
		},
	}
	allTypeComponents := map[string]any{
		"inherit-error-vpc": componentSection,
		// base component has metadata but it's a string (invalid) → ProcessBaseComponentConfig errors
		"base-inherit-error": map[string]any{
			"metadata": "not-a-map",
		},
	}

	err := p.processComponentEntry(
		"inherit-error-stack.yaml", "", cfg.TerraformSectionName,
		"inherit-error-vpc", componentSection, allTypeComponents,
		processComponentTypeOpts{applyMetadataInheritance: true},
	)

	require.Error(t, err)
}

func TestProcessComponentEntry_BuildWorkspaceError(t *testing.T) {
	// When metadata has an invalid terraform_workspace_template, BuildTerraformWorkspace fails
	// and processComponentEntry propagates the error (lines 249-251).
	p := newMinimalProcessor()

	componentSection := map[string]any{
		cfg.ComponentSectionName: "vpc",
		cfg.MetadataSectionName: map[string]any{
			"terraform_workspace_template": "{{.bad", // invalid Go template
		},
	}
	allTypeComponents := map[string]any{
		"vpc": componentSection,
	}

	err := p.processComponentEntry(
		"test.yaml", "", cfg.TerraformSectionName,
		"vpc", componentSection, allTypeComponents,
		processComponentTypeOpts{buildWorkspace: true},
	)

	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// applyTerraformMetadataInheritance – ProcessBaseComponentConfig error
// ---------------------------------------------------------------------------

func TestApplyTerraformMetadataInheritance_BaseComponentConfigError(t *testing.T) {
	// When the base component has an invalid "metadata" section (not a map),
	// ProcessBaseComponentConfig returns an error (lines 609-611).
	// Use unique stack/component names to avoid global cache collision with other tests.
	ac := &schema.AtmosConfiguration{}

	allComponents := map[string]any{
		"base-error-comp": map[string]any{
			"metadata": "not-a-map", // invalid metadata type → ProcessBaseComponentConfig errors
		},
	}

	metadata := map[string]any{
		cfg.InheritsSectionName: []any{"base-error-comp"},
	}

	_, err := applyTerraformMetadataInheritance(
		ac, allComponents, "error-comp", "error-stack.yaml", metadata,
	)

	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// processComponentSectionTemplates – direct error path tests
// ---------------------------------------------------------------------------

// yamlMarshalError implements yaml.Marshaler and always returns an error,
// allowing us to trigger the ConvertToYAMLPreservingDelimiters error path in tests.
type yamlMarshalError struct{}

func (y yamlMarshalError) MarshalYAML() (interface{}, error) {
	return nil, errors.New("yaml marshal error for testing")
}

func TestProcessComponentSectionTemplates_ConvertToYAMLError(t *testing.T) {
	// A value whose MarshalYAML method returns an error triggers the
	// ConvertToYAMLPreservingDelimiters error path (lines 506-508).
	ac := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}

	componentSection := map[string]any{
		"key": yamlMarshalError{},
	}

	_, err := processComponentSectionTemplates(ac, info, componentSection, map[string]any{})
	require.Error(t, err)
}

func TestProcessComponentSectionTemplates_MapstructureDecodeError(t *testing.T) {
	// When settingsSection contains a string value for a nested struct field, mapstructure.Decode
	// returns an error (lines 511-513).
	ac := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}

	componentSection := map[string]any{"vars": map[string]any{}}
	// "templates" expects a nested struct but gets a string → mapstructure decoding fails.
	settingsSection := map[string]any{
		"templates": map[string]any{
			"settings": "not-a-struct",
		},
	}

	_, err := processComponentSectionTemplates(ac, info, componentSection, settingsSection)
	require.Error(t, err)
}

func TestProcessComponentSectionTemplates_ProcessTmplError(t *testing.T) {
	// When templates are enabled and the component section has an invalid Go template,
	// ProcessTmplWithDatasources fails (lines 529-531).
	ac := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true, // must enable templating for ProcessTmplWithDatasources to run
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}

	// An unclosed Go template directive causes ProcessTmplWithDatasources to return an error.
	componentSection := map[string]any{
		"vars": map[string]any{
			"bad_template": "{{ .bad_open",
		},
	}

	_, err := processComponentSectionTemplates(ac, info, componentSection, map[string]any{})
	require.Error(t, err)
}

func TestProcessComponentEntry_ProcessTemplatesError(t *testing.T) {
	// When processTemplates=true and the component section has an invalid Go template,
	// processComponentSectionTemplates returns an error which processComponentEntry propagates (lines 264-266).
	p := newDescribeStacksProcessor(
		&schema.AtmosConfiguration{
			Templates: schema.Templates{
				Settings: schema.TemplatesSettings{
					Enabled: true,
				},
			},
		},
		"", nil, nil, nil,
		true,  // processTemplates = true
		false, // processYamlFunctions
		false, nil, nil,
	)

	componentSection := map[string]any{
		cfg.ComponentSectionName: "vpc",
		"vars": map[string]any{
			"bad": "{{ .bad_open", // invalid Go template causes processComponentSectionTemplates to fail
		},
	}
	allTypeComponents := map[string]any{"vpc": componentSection}

	err := p.processComponentEntry(
		"test.yaml", "", cfg.TerraformSectionName,
		"vpc", componentSection, allTypeComponents,
		processComponentTypeOpts{},
	)

	require.Error(t, err)
}
