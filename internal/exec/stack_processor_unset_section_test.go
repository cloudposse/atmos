package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestExtractComponentSections_UnsetWholeSection verifies that a component section set to
// the `!unset` YAML tag (decoded to the plain string "!unset" before YAML functions run) is
// treated as absent instead of failing the map type assertion (issue #2994).
func TestExtractComponentSections_UnsetWholeSection(t *testing.T) {
	tests := []struct {
		section string
		get     func(r *ComponentProcessorResult) map[string]any
	}{
		{cfg.VarsSectionName, func(r *ComponentProcessorResult) map[string]any { return r.ComponentVars }},
		{cfg.SettingsSectionName, func(r *ComponentProcessorResult) map[string]any { return r.ComponentSettings }},
		{cfg.EnvSectionName, func(r *ComponentProcessorResult) map[string]any { return r.ComponentEnv }},
		{cfg.RetrySectionName, func(r *ComponentProcessorResult) map[string]any { return r.ComponentRetry }},
		{cfg.HooksSectionName, func(r *ComponentProcessorResult) map[string]any { return r.ComponentHooks }},
	}

	for _, tt := range tests {
		t.Run(tt.section, func(t *testing.T) {
			opts := ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				StackName:     "test-stack",
				ComponentMap:  map[string]any{tt.section: u.AtmosYamlFuncUnset},
				AtmosConfig:   &schema.AtmosConfiguration{},
			}
			result := &ComponentProcessorResult{}

			require.NoError(t, extractComponentSections(&opts, result))
			assert.Empty(t, tt.get(result))
			// The caller's component map must not be mutated.
			assert.Equal(t, u.AtmosYamlFuncUnset, opts.ComponentMap[tt.section])
		})
	}
}

// TestDescribeStacks_UnsetWholeSection runs the unset-section scenario end to end: a section set
// to `!unset` at one layer drops every lower layer (global and base component), while overrides
// still apply on top of a component-level unset.
func TestDescribeStacks_UnsetWholeSection(t *testing.T) {
	fixtureDir, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "unset-section"))
	require.NoError(t, err)
	t.Chdir(fixtureDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixtureDir)
	t.Setenv("ATMOS_BASE_PATH", fixtureDir)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// The embedded manifest schema accepts `!unset` for whole sections.
	require.NoError(t, ValidateStacks(&atmosConfig))

	stacks, err := ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, true, true, false, nil, nil)
	require.NoError(t, err)

	component := func(stack, name string) map[string]any {
		t.Helper()
		stackMap, ok := stacks[stack].(map[string]any)
		require.True(t, ok, "stack %s not found", stack)
		components, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
		require.True(t, ok)
		terraform, ok := components[cfg.TerraformComponentType].(map[string]any)
		require.True(t, ok)
		c, ok := terraform[name].(map[string]any)
		require.True(t, ok, "component %s not found in stack %s", name, stack)
		return c
	}

	t.Run("component unset drops the global layer", func(t *testing.T) {
		vpc := component("deploy/dev", "vpc")
		assert.Empty(t, vpc[cfg.VarsSectionName], "global vars, including stage, are dropped")
		assert.NotContains(t, vpc, cfg.RetrySectionName)
	})

	t.Run("component unset drops the inherited base component layer", func(t *testing.T) {
		child := component("deploy/dev", "vpc-child")
		assert.Empty(t, child[cfg.VarsSectionName])
		assert.NotContains(t, child, cfg.RetrySectionName)
		assert.Equal(t, []any{"vpc-base"}, child[cfg.InheritanceSectionName])
	})

	t.Run("base component unset drops its ancestors and the global layer", func(t *testing.T) {
		grandchild := component("deploy/dev", "vpc-grandchild")
		assert.Equal(t, map[string]any{"max_attempts": 5}, grandchild[cfg.RetrySectionName])
		assert.Equal(t, map[string]any{"cidr": "10.0.0.0/16", "stage": "dev"}, grandchild[cfg.VarsSectionName])
	})

	t.Run("overrides apply on top of a component unset", func(t *testing.T) {
		overridden := component("deploy/prod", "vpc-overrides")
		assert.Equal(t, map[string]any{"max_attempts": 7}, overridden[cfg.RetrySectionName])
	})

	t.Run("overrides unset drops every lower layer", func(t *testing.T) {
		overridden := component("deploy/prod", "vpc-overrides")
		assert.Empty(t, overridden[cfg.VarsSectionName])
	})

	t.Run("sections without unset still inherit", func(t *testing.T) {
		base := component("deploy/dev", "vpc-base")
		assert.Equal(t, map[string]any{"max_attempts": 3, "backoff_strategy": "constant"}, base[cfg.RetrySectionName])
		assert.Equal(t, map[string]any{"cidr": "10.0.0.0/16", "stage": "dev"}, base[cfg.VarsSectionName])
	})
}

// unsetSectionLayers maps each unsettable section to the fields that hold it at every layer.
// The table is written out by hand, so it also checks the lookup switches in the code under test.
type unsetSectionLayers struct {
	section   string
	global    func(o *ComponentProcessorOptions) map[string]any // nil when the section has no global layer.
	base      func(r *ComponentProcessorResult) map[string]any
	component func(r *ComponentProcessorResult) map[string]any
	baseField func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType
}

func unsetSectionLayersTable() []unsetSectionLayers {
	return []unsetSectionLayers{
		{
			cfg.VarsSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.GlobalVars },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentVars },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentVars },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentVars },
		},
		{
			cfg.SettingsSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.GlobalSettings },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentSettings },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentSettings },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentSettings },
		},
		{
			cfg.EnvSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.GlobalEnv },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentEnv },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentEnv },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentEnv },
		},
		{
			cfg.AuthSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.GlobalAuth },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentAuth },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentAuth },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentAuth },
		},
		{
			cfg.SecretsSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.GlobalSecrets },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentSecrets },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentSecrets },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentSecrets },
		},
		{
			cfg.ProvidersSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.TerraformProviders },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentProviders },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentProviders },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentProviders },
		},
		{
			cfg.RequiredProvidersSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.TerraformRequiredProviders },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentRequiredProviders },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentRequiredProviders },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType {
				return &c.BaseComponentRequiredProviders
			},
		},
		{
			cfg.HooksSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.GlobalAndTerraformHooks },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentHooks },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentHooks },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentHooks },
		},
		{
			cfg.TestSectionName,
			nil,
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentTest },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentTest },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentTest },
		},
		{
			cfg.MocksSectionName,
			nil,
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentMocks },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentMocks },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentMocks },
		},
		{
			cfg.GenerateSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.GlobalAndTerraformGenerate },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentGenerate },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentGenerate },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentGenerate },
		},
		{
			cfg.FlagsSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.GlobalAndTerraformFlags },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentFlags },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentFlags },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentFlags },
		},
		{
			cfg.DependenciesSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.GlobalDependencies },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentDependencies },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentDependencies },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentDependencies },
		},
		{
			cfg.LocalsSectionName,
			nil,
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentLocals },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentLocals },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentLocals },
		},
		{
			cfg.RetrySectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.GlobalComponentRetry },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentRetry },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentRetry },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType { return &c.BaseComponentRetry },
		},
		{
			cfg.ProvisionSectionName,
			func(o *ComponentProcessorOptions) map[string]any { return o.GlobalProvisionSection },
			func(r *ComponentProcessorResult) map[string]any { return r.BaseComponentProvisionSection },
			func(r *ComponentProcessorResult) map[string]any { return r.ComponentProvision },
			func(c *schema.BaseComponentConfig) *schema.AtmosSectionMapType {
				return &c.BaseComponentProvisionSection
			},
		},
	}
}

// TestUnsetSectionLayersTable_CoversEverySection guards the hand-written table above: a section
// added to unsettableComponentSections without a test row fails here.
func TestUnsetSectionLayersTable_CoversEverySection(t *testing.T) {
	table := unsetSectionLayersTable()
	require.Len(t, table, len(unsettableComponentSections))
	for i, row := range table {
		assert.Equal(t, unsettableComponentSections[i], row.section)
	}
}

func TestIsUnsetSection(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{"unset tag", u.AtmosYamlFuncUnset, true},
		{"unset tag with surrounding whitespace", "  " + u.AtmosYamlFuncUnset + "\n", true},
		{"other string", "unset", false},
		{"unset tag with a suffix", u.AtmosYamlFuncUnset + " now", false},
		{"empty string", "", false},
		{"map", map[string]any{"a": 1}, false},
		{"nil", nil, false},
		{"non-string scalar", 42, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isUnsetSection(tt.value))
		})
	}
}

func TestSplitUnsetSections(t *testing.T) {
	t.Run("returns the same map when nothing is unset", func(t *testing.T) {
		section := map[string]any{
			cfg.VarsSectionName: map[string]any{"a": 1},
			// Sections outside the unsettable list keep the raw tag for later YAML processing.
			cfg.MetadataSectionName: u.AtmosYamlFuncUnset,
		}
		got, unset := splitUnsetSections(section)
		assert.Nil(t, unset)
		// Same underlying map, no copy made.
		got["probe"] = true
		assert.Contains(t, section, "probe")
	})

	t.Run("nil map", func(t *testing.T) {
		got, unset := splitUnsetSections(nil)
		assert.Nil(t, got)
		assert.Nil(t, unset)
	})

	t.Run("strips unset sections from a copy and names them in list order", func(t *testing.T) {
		vars := map[string]any{"a": 1}
		section := map[string]any{
			cfg.RetrySectionName:    u.AtmosYamlFuncUnset,
			cfg.EnvSectionName:      " " + u.AtmosYamlFuncUnset,
			cfg.VarsSectionName:     vars,
			cfg.MetadataSectionName: u.AtmosYamlFuncUnset,
		}
		got, unset := splitUnsetSections(section)

		assert.Equal(t, []string{cfg.EnvSectionName, cfg.RetrySectionName}, unset)
		assert.Equal(t, map[string]any{
			cfg.VarsSectionName:     vars,
			cfg.MetadataSectionName: u.AtmosYamlFuncUnset,
		}, got)

		// The input map keeps every key.
		assert.Len(t, section, 4)
		assert.Equal(t, u.AtmosYamlFuncUnset, section[cfg.RetrySectionName])

		// result -> input isolation.
		got["added"] = true
		assert.NotContains(t, section, "added")
		// input -> result isolation.
		section["late"] = true
		assert.NotContains(t, got, "late")
	})

	t.Run("every unsettable section is recognized", func(t *testing.T) {
		section := map[string]any{}
		for _, name := range unsettableComponentSections {
			section[name] = u.AtmosYamlFuncUnset
		}
		got, unset := splitUnsetSections(section)
		assert.Empty(t, got)
		assert.Equal(t, unsettableComponentSections, unset)
	})
}

func TestClearUnsetBaseComponentSections(t *testing.T) {
	for _, row := range unsetSectionLayersTable() {
		t.Run(row.section, func(t *testing.T) {
			c := &schema.BaseComponentConfig{}
			for _, other := range unsetSectionLayersTable() {
				*other.baseField(c) = map[string]any{"from": other.section}
			}

			clearUnsetBaseComponentSections(c, []string{row.section})

			assert.Nil(t, *row.baseField(c), "the unset section is cleared")
			for _, other := range unsetSectionLayersTable() {
				if other.section != row.section {
					assert.Equal(t, map[string]any{"from": other.section}, *other.baseField(c),
						"%s must be left alone", other.section)
				}
			}
			assert.Equal(t, []string{row.section}, c.BaseComponentUnsetSections)
		})
	}

	t.Run("records each section once across inheritance levels", func(t *testing.T) {
		c := &schema.BaseComponentConfig{BaseComponentVars: map[string]any{"a": 1}}
		clearUnsetBaseComponentSections(c, []string{cfg.VarsSectionName, cfg.RetrySectionName})
		// A deeper base component unsets vars again after the chain re-added values.
		c.BaseComponentVars = map[string]any{"b": 2}
		clearUnsetBaseComponentSections(c, []string{cfg.VarsSectionName})

		assert.Nil(t, c.BaseComponentVars)
		assert.Equal(t, []string{cfg.VarsSectionName, cfg.RetrySectionName}, c.BaseComponentUnsetSections)
	})

	t.Run("no sections is a no-op", func(t *testing.T) {
		c := &schema.BaseComponentConfig{BaseComponentVars: map[string]any{"a": 1}}
		clearUnsetBaseComponentSections(c, nil)
		assert.Equal(t, map[string]any{"a": 1}, c.BaseComponentVars)
		assert.Empty(t, c.BaseComponentUnsetSections)
	})
}

func TestBaseComponentSectionField_UnknownSection(t *testing.T) {
	assert.Nil(t, baseComponentSectionField(&schema.BaseComponentConfig{}, cfg.MetadataSectionName))
}

func TestSectionLayerFields_UnknownSection(t *testing.T) {
	opts := &ComponentProcessorOptions{}
	result := &ComponentProcessorResult{}
	global, base, component := sectionLayerFields(opts, result, cfg.MetadataSectionName)
	assert.Nil(t, global)
	assert.Nil(t, base)
	assert.Nil(t, component)
}

// filledUnsetLayers returns options and a result with every layer of every unsettable section set
// to a marker map, so a test can see exactly which layers a drop cleared.
func filledUnsetLayers() (*ComponentProcessorOptions, *ComponentProcessorResult) {
	o := &ComponentProcessorOptions{}
	r := &ComponentProcessorResult{}
	for _, section := range unsettableComponentSections {
		global, base, component := sectionLayerFields(o, r, section)
		if global != nil {
			*global = map[string]any{"layer": "global"}
		}
		*base = map[string]any{"layer": "base"}
		*component = map[string]any{"layer": "component"}
	}
	return o, r
}

func TestDropUnsetSectionLayers(t *testing.T) {
	t.Run("returns the inputs unchanged when nothing is unset", func(t *testing.T) {
		o, r := filledUnsetLayers()
		gotO, gotR := dropUnsetSectionLayers(o, r)
		assert.Same(t, o, gotO)
		assert.Same(t, r, gotR)
	})

	cases := []struct {
		name          string
		mark          func(r *ComponentProcessorResult, section string)
		wantGlobal    bool
		wantBase      bool
		wantComponent bool
	}{
		{
			name:          "base component unset drops the global layer",
			mark:          func(r *ComponentProcessorResult, s string) { r.BaseComponentUnsetSections = []string{s} },
			wantBase:      true,
			wantComponent: true,
		},
		{
			name:          "component unset drops the global and base layers",
			mark:          func(r *ComponentProcessorResult, s string) { r.ComponentUnsetSections = []string{s} },
			wantComponent: true,
		},
		{
			name: "overrides unset drops every layer",
			mark: func(r *ComponentProcessorResult, s string) { r.ComponentOverridesUnsetSections = []string{s} },
		},
		{
			name: "overrides unset wins over a component unset",
			mark: func(r *ComponentProcessorResult, s string) {
				r.ComponentUnsetSections = []string{s}
				r.ComponentOverridesUnsetSections = []string{s}
			},
		},
		{
			name: "component unset wins over a base component unset",
			mark: func(r *ComponentProcessorResult, s string) {
				r.BaseComponentUnsetSections = []string{s}
				r.ComponentUnsetSections = []string{s}
			},
			wantComponent: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, row := range unsetSectionLayersTable() {
				t.Run(row.section, func(t *testing.T) {
					o, r := filledUnsetLayers()
					tc.mark(r, row.section)

					gotO, gotR := dropUnsetSectionLayers(o, r)

					// The section's own layers.
					if row.global != nil {
						assert.Equal(t, tc.wantGlobal, row.global(gotO) != nil, "global layer kept")
						assert.NotNil(t, row.global(o), "input options must not be mutated")
					}
					assert.Equal(t, tc.wantBase, row.base(gotR) != nil, "base layer kept")
					assert.Equal(t, tc.wantComponent, row.component(gotR) != nil, "component layer kept")
					assert.NotNil(t, row.base(r), "input result must not be mutated")
					assert.NotNil(t, row.component(r), "input result must not be mutated")

					// Every other section keeps all of its layers.
					for _, other := range unsetSectionLayersTable() {
						if other.section == row.section {
							continue
						}
						if other.global != nil {
							assert.Equal(t, map[string]any{"layer": "global"}, other.global(gotO), other.section)
						}
						assert.Equal(t, map[string]any{"layer": "base"}, other.base(gotR), other.section)
						assert.Equal(t, map[string]any{"layer": "component"}, other.component(gotR), other.section)
					}
				})
			}
		})
	}
}
