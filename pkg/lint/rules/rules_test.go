package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/lint"
	"github.com/cloudposse/atmos/pkg/lint/rules"
	"github.com/cloudposse/atmos/pkg/schema"
)

// makeContext creates a minimal LintContext for testing.
func makeContext(stacksMap map[string]any) lint.LintContext {
	return lint.LintContext{
		StacksMap:       stacksMap,
		RawStackConfigs: make(map[string]map[string]any),
		ImportGraph:     make(map[string][]string),
		LintConfig:      schema.LintConfig{},
	}
}

// componentsMap builds a components section map for use in stacks.
func componentsMap(compType string, comps map[string]any) map[string]any {
	return map[string]any{
		"components": map[string]any{
			compType: comps,
		},
	}
}

func TestL09CycleDetection(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l09 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-09" {
			l09 = r
			break
		}
	}
	require.NotNil(t, l09)
	assert.Equal(t, "L-09", l09.ID())
	assert.Equal(t, lint.SeverityError, l09.Severity())
	assert.False(t, l09.AutoFixable())

	tests := []struct {
		name          string
		stacksMap     map[string]any
		expectCycle   bool
		expectMessage string
	}{
		{
			name:        "no cycle",
			stacksMap:   map[string]any{},
			expectCycle: false,
		},
		{
			name: "simple cycle A->B->A",
			stacksMap: map[string]any{
				"stack1": componentsMap("terraform", map[string]any{
					"comp-a": map[string]any{
						"metadata": map[string]any{
							"inherits": []any{"comp-b"},
						},
					},
					"comp-b": map[string]any{
						"metadata": map[string]any{
							"inherits": []any{"comp-a"},
						},
					},
				}),
			},
			expectCycle: true,
		},
		{
			name: "no cycle with valid inheritance",
			stacksMap: map[string]any{
				"stack1": componentsMap("terraform", map[string]any{
					"comp-base": map[string]any{
						"metadata": map[string]any{
							"type": "abstract",
						},
					},
					"comp-child": map[string]any{
						"metadata": map[string]any{
							"inherits": []any{"comp-base"},
						},
					},
				}),
			},
			expectCycle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := makeContext(tt.stacksMap)
			findings, err := l09.Run(ctx)
			require.NoError(t, err)
			if tt.expectCycle {
				assert.NotEmpty(t, findings)
				for _, f := range findings {
					assert.Equal(t, "L-09", f.RuleID)
					assert.Equal(t, lint.SeverityError, f.Severity)
				}
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}

func TestL04AbstractLeak(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l04 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-04" {
			l04 = r
			break
		}
	}
	require.NotNil(t, l04)

	tests := []struct {
		name          string
		stacksMap     map[string]any
		expectLeak    bool
	}{
		{
			name:       "no abstract component",
			stacksMap:  map[string]any{},
			expectLeak: false,
		},
		{
			name: "abstract with concrete inheritor - no leak",
			stacksMap: map[string]any{
				"stack1": componentsMap("terraform", map[string]any{
					"base": map[string]any{
						"metadata": map[string]any{"type": "abstract"},
					},
					"concrete": map[string]any{
						"metadata": map[string]any{
							"inherits": []any{"base"},
						},
					},
				}),
			},
			expectLeak: false,
		},
		{
			name: "abstract with no inheritor - leak",
			stacksMap: map[string]any{
				"stack1": componentsMap("terraform", map[string]any{
					"base": map[string]any{
						"metadata": map[string]any{"type": "abstract"},
					},
				}),
			},
			expectLeak: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := makeContext(tt.stacksMap)
			findings, err := l04.Run(ctx)
			require.NoError(t, err)
			if tt.expectLeak {
				assert.NotEmpty(t, findings)
				for _, f := range findings {
					assert.Equal(t, "L-04", f.RuleID)
					assert.Equal(t, lint.SeverityError, f.Severity)
				}
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}

func TestL02RedundantOverride(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l02 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-02" {
			l02 = r
			break
		}
	}
	require.NotNil(t, l02)
	assert.True(t, l02.AutoFixable())

	tests := []struct {
		name           string
		stacksMap      map[string]any
		expectRedundant bool
	}{
		{
			name:            "no redundant overrides",
			stacksMap:       map[string]any{},
			expectRedundant: false,
		},
		{
			name: "redundant override detected",
			stacksMap: map[string]any{
				"stack1": componentsMap("terraform", map[string]any{
					"base": map[string]any{
						"metadata": map[string]any{"type": "abstract"},
						"vars":     map[string]any{"region": "us-east-1"},
					},
					"child": map[string]any{
						"metadata": map[string]any{
							"inherits": []any{"base"},
						},
						// Redundant: same value as parent
						"vars": map[string]any{"region": "us-east-1"},
					},
				}),
			},
			expectRedundant: true,
		},
		{
			name: "override with different value - not redundant",
			stacksMap: map[string]any{
				"stack1": componentsMap("terraform", map[string]any{
					"base": map[string]any{
						"metadata": map[string]any{"type": "abstract"},
						"vars":     map[string]any{"region": "us-east-1"},
					},
					"child": map[string]any{
						"metadata": map[string]any{
							"inherits": []any{"base"},
						},
						"vars": map[string]any{"region": "us-west-2"},
					},
				}),
			},
			expectRedundant: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := makeContext(tt.stacksMap)
			findings, err := l02.Run(ctx)
			require.NoError(t, err)
			if tt.expectRedundant {
				assert.NotEmpty(t, findings)
				for _, f := range findings {
					assert.Equal(t, "L-02", f.RuleID)
					assert.Equal(t, lint.SeverityWarning, f.Severity)
				}
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}

func TestL01DeadVar(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l01 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-01" {
			l01 = r
			break
		}
	}
	require.NotNil(t, l01)

	tests := []struct {
		name        string
		stacksMap   map[string]any
		expectDead  bool
	}{
		{
			name:       "no dead vars",
			stacksMap:  map[string]any{},
			expectDead: false,
		},
		{
			name: "dead global var",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"vars": map[string]any{
						"unused_var": "value",
					},
					"components": map[string]any{
						"terraform": map[string]any{
							"mycomp": map[string]any{
								"vars": map[string]any{
									"other_var": "other",
								},
							},
						},
					},
				},
			},
			expectDead: true,
		},
		{
			name: "global var used by component",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"vars": map[string]any{
						"region": "us-east-1",
					},
					"components": map[string]any{
						"terraform": map[string]any{
							"mycomp": map[string]any{
								"vars": map[string]any{
									"region": "us-east-1",
								},
							},
						},
					},
				},
			},
			expectDead: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := makeContext(tt.stacksMap)
			findings, err := l01.Run(ctx)
			require.NoError(t, err)
			if tt.expectDead {
				assert.NotEmpty(t, findings)
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}

func TestL03ImportDepth(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l03 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-03" {
			l03 = r
			break
		}
	}
	require.NotNil(t, l03)

	tests := []struct {
		name           string
		importGraph    map[string][]string
		threshold      int
		expectTooDeep  bool
	}{
		{
			name:          "no imports",
			importGraph:   map[string][]string{},
			threshold:     3,
			expectTooDeep: false,
		},
		{
			name: "depth within threshold",
			importGraph: map[string][]string{
				"root":  {"level1"},
				"level1": {"level2"},
			},
			threshold:     3,
			expectTooDeep: false,
		},
		{
			name: "depth exceeds threshold",
			importGraph: map[string][]string{
				"root":   {"level1"},
				"level1": {"level2"},
				"level2": {"level3"},
				"level3": {"level4"},
			},
			threshold:     3,
			expectTooDeep: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := lint.LintContext{
				StacksMap:   make(map[string]any),
				ImportGraph: tt.importGraph,
				LintConfig: schema.LintConfig{
					MaxImportDepth: tt.threshold,
				},
			}
			findings, err := l03.Run(ctx)
			require.NoError(t, err)
			if tt.expectTooDeep {
				assert.NotEmpty(t, findings)
				for _, f := range findings {
					assert.Equal(t, "L-03", f.RuleID)
				}
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}

func TestL08SensitiveVar(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l08 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-08" {
			l08 = r
			break
		}
	}
	require.NotNil(t, l08)

	tests := []struct {
		name            string
		stacksMap       map[string]any
		expectSensitive bool
	}{
		{
			name: "sensitive var at global scope",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"vars": map[string]any{
						"my_secret_key": "supersecret",
					},
				},
			},
			expectSensitive: true,
		},
		{
			name: "non-sensitive var at global scope",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"vars": map[string]any{
						"region": "us-east-1",
					},
				},
			},
			expectSensitive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := makeContext(tt.stacksMap)
			findings, err := l08.Run(ctx)
			require.NoError(t, err)
			if tt.expectSensitive {
				assert.NotEmpty(t, findings)
				for _, f := range findings {
					assert.Equal(t, "L-08", f.RuleID)
				}
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}

func TestL10EnvShadowing(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l10 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-10" {
			l10 = r
			break
		}
	}
	require.NotNil(t, l10)

	tests := []struct {
		name           string
		stacksMap      map[string]any
		expectShadowed bool
	}{
		{
			name: "env shadowing detected",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"env": map[string]any{
						"MY_VAR": "stack-value",
					},
					"components": map[string]any{
						"terraform": map[string]any{
							"mycomp": map[string]any{
								"env": map[string]any{
									"MY_VAR": "component-value",
								},
							},
						},
					},
				},
			},
			expectShadowed: true,
		},
		{
			name: "same env value - no shadowing",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"env": map[string]any{
						"MY_VAR": "same-value",
					},
					"components": map[string]any{
						"terraform": map[string]any{
							"mycomp": map[string]any{
								"env": map[string]any{
									"MY_VAR": "same-value",
								},
							},
						},
					},
				},
			},
			expectShadowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := makeContext(tt.stacksMap)
			findings, err := l10.Run(ctx)
			require.NoError(t, err)
			if tt.expectShadowed {
				assert.NotEmpty(t, findings)
				for _, f := range findings {
					assert.Equal(t, "L-10", f.RuleID)
				}
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}

func TestAllRulesArePresent(t *testing.T) {
	t.Parallel()

	all := rules.All()
	expectedIDs := []string{"L-01", "L-02", "L-03", "L-04", "L-05", "L-06", "L-07", "L-08", "L-09", "L-10"}

	foundIDs := make(map[string]bool)
	for _, r := range all {
		foundIDs[r.ID()] = true
		assert.NotEmpty(t, r.Name())
		assert.NotEmpty(t, r.Description())
	}

	for _, id := range expectedIDs {
		assert.True(t, foundIDs[id], "Expected rule %s to be registered", id)
	}
}

func TestEngineRunWithFilter(t *testing.T) {
	t.Parallel()

	ctx := lint.LintContext{
		StacksMap: map[string]any{
			"stack1": map[string]any{
				"vars": map[string]any{"my_password": "secret"},
			},
		},
		ImportGraph: map[string][]string{},
		LintConfig:  schema.LintConfig{},
	}

	engine := lint.NewEngine(rules.All())

	// Run only L-08.
	result, err := engine.Run(ctx, []string{"L-08"}, lint.SeverityInfo)
	require.NoError(t, err)
	for _, f := range result.Findings {
		assert.Equal(t, "L-08", f.RuleID, "only L-08 should run")
	}
}
