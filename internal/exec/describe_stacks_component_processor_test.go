package exec

import (
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
		"vars":     map[string]any{},                    // empty map → should be skipped
		"settings": map[string]any{"key": "val"},        // non-empty → should be included
		"label":    "not-a-map",                         // non-map → always included
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
