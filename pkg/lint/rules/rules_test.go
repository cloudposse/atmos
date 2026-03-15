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
		name       string
		stacksMap  map[string]any
		expectLeak bool
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
		name            string
		stacksMap       map[string]any
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
		name       string
		stacksMap  map[string]any
		expectDead bool
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
		name          string
		importGraph   map[string][]string
		threshold     int
		expectTooDeep bool
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
				"root":   {"level1"},
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
		{
			name: "default threshold used when zero",
			importGraph: map[string][]string{
				"root":   {"l1"},
				"l1":     {"l2"},
				"l2":     {"l3"},
				"l3":     {"l4"},
			},
			threshold:     0, // triggers default of 3
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

func TestL05CohesionRule(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l05 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-05" {
			l05 = r
			break
		}
	}
	require.NotNil(t, l05)
	assert.Equal(t, "L-05", l05.ID())
	assert.NotEmpty(t, l05.Name())
	assert.NotEmpty(t, l05.Description())
	assert.Equal(t, lint.SeverityInfo, l05.Severity())
	assert.False(t, l05.AutoFixable())

	tests := []struct {
		name            string
		rawStackConfigs map[string]map[string]any
		expectFindings  bool
	}{
		{
			name:            "empty configs",
			rawStackConfigs: map[string]map[string]any{},
			expectFindings:  false,
		},
		{
			name: "few concern groups - no finding",
			rawStackConfigs: map[string]map[string]any{
				"/stacks/catalog/network.yaml": {
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc":   map[string]any{},
							"vpc-2": map[string]any{},
						},
					},
				},
			},
			expectFindings: false,
		},
		{
			name: "many concern groups - finding",
			rawStackConfigs: map[string]map[string]any{
				"/stacks/catalog/mixed.yaml": {
					"components": map[string]any{
						"terraform": map[string]any{
							// 4 different concern groups (prefixes): vpc, ecs, rds, eks
							"vpc-prod":      map[string]any{},
							"ecs-api":       map[string]any{},
							"rds-postgres":  map[string]any{},
							"eks-cluster":   map[string]any{},
						},
					},
				},
			},
			expectFindings: true,
		},
		{
			name: "many groups with helmfile components",
			rawStackConfigs: map[string]map[string]any{
				"/stacks/catalog/mixed-hf.yaml": {
					"components": map[string]any{
						"helmfile": map[string]any{
							"cert-manager":  map[string]any{},
							"ingress-nginx": map[string]any{},
							"argocd":        map[string]any{},
							"vault":         map[string]any{},
						},
					},
				},
			},
			expectFindings: true,
		},
		{
			name: "with StacksBasePath - shorter display path",
			rawStackConfigs: map[string]map[string]any{
				"/project/stacks/catalog/mixed.yaml": {
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc-prod":     map[string]any{},
							"ecs-api":      map[string]any{},
							"rds-postgres": map[string]any{},
							"eks-cluster":  map[string]any{},
						},
					},
				},
			},
			expectFindings: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := lint.LintContext{
				StacksMap:       make(map[string]any),
				RawStackConfigs: tt.rawStackConfigs,
				ImportGraph:     make(map[string][]string),
				LintConfig:      schema.LintConfig{},
				StacksBasePath:  "/project/stacks",
			}
			findings, err := l05.Run(ctx)
			require.NoError(t, err)
			if tt.expectFindings {
				assert.NotEmpty(t, findings)
				for _, f := range findings {
					assert.Equal(t, "L-05", f.RuleID)
					assert.Equal(t, lint.SeverityInfo, f.Severity)
					assert.NotEmpty(t, f.Message)
					assert.NotEmpty(t, f.FixHint)
				}
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}

func TestL06DRYExtractionOpportunity(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l06 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-06" {
			l06 = r
			break
		}
	}
	require.NotNil(t, l06)
	assert.Equal(t, "L-06", l06.ID())
	assert.NotEmpty(t, l06.Name())
	assert.NotEmpty(t, l06.Description())
	assert.Equal(t, lint.SeverityInfo, l06.Severity())
	assert.False(t, l06.AutoFixable())

	tests := []struct {
		name           string
		stacksMap      map[string]any
		thresholdPct   int
		expectFindings bool
	}{
		{
			name:           "empty stacks",
			stacksMap:      map[string]any{},
			thresholdPct:   80,
			expectFindings: false,
		},
		{
			name: "var in single stack - not a DRY issue",
			stacksMap: map[string]any{
				"stack1": componentsMap("terraform", map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{
							"cidr": "10.0.0.0/16",
						},
					},
				}),
			},
			thresholdPct:   80,
			expectFindings: false,
		},
		{
			name: "same var value in 100% of stacks - DRY opportunity",
			stacksMap: map[string]any{
				"stack1": componentsMap("terraform", map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"region": "us-east-1"},
					},
				}),
				"stack2": componentsMap("terraform", map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"region": "us-east-1"},
					},
				}),
				"stack3": componentsMap("terraform", map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"region": "us-east-1"},
					},
				}),
			},
			thresholdPct:   80,
			expectFindings: true,
		},
		{
			name: "different values in stacks - not a DRY issue",
			stacksMap: map[string]any{
				"stack1": componentsMap("terraform", map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"cidr": "10.0.0.0/16"},
					},
				}),
				"stack2": componentsMap("terraform", map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"cidr": "10.1.0.0/16"},
					},
				}),
			},
			thresholdPct:   80,
			expectFindings: false,
		},
		{
			name: "default threshold used when zero",
			stacksMap: map[string]any{
				"stack1": componentsMap("terraform", map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"region": "us-east-1"},
					},
				}),
				"stack2": componentsMap("terraform", map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"region": "us-east-1"},
					},
				}),
			},
			thresholdPct:   0, // triggers default 80
			expectFindings: true,
		},
		{
			name: "helmfile components also detected",
			stacksMap: map[string]any{
				"stack1": componentsMap("helmfile", map[string]any{
					"nginx": map[string]any{
						"vars": map[string]any{"replicas": "3"},
					},
				}),
				"stack2": componentsMap("helmfile", map[string]any{
					"nginx": map[string]any{
						"vars": map[string]any{"replicas": "3"},
					},
				}),
			},
			thresholdPct:   80,
			expectFindings: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := lint.LintContext{
				StacksMap:       tt.stacksMap,
				RawStackConfigs: make(map[string]map[string]any),
				ImportGraph:     make(map[string][]string),
				LintConfig: schema.LintConfig{
					DRYThresholdPct: tt.thresholdPct,
				},
			}
			findings, err := l06.Run(ctx)
			require.NoError(t, err)
			if tt.expectFindings {
				assert.NotEmpty(t, findings)
				for _, f := range findings {
					assert.Equal(t, "L-06", f.RuleID)
					assert.Equal(t, lint.SeverityInfo, f.Severity)
					assert.NotEmpty(t, f.Message)
					assert.NotEmpty(t, f.FixHint)
				}
			} else {
				assert.Empty(t, findings)
			}
		})
	}
}

func TestL07OrphanedFile(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l07 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-07" {
			l07 = r
			break
		}
	}
	require.NotNil(t, l07)
	assert.Equal(t, "L-07", l07.ID())
	assert.NotEmpty(t, l07.Name())
	assert.NotEmpty(t, l07.Description())
	assert.Equal(t, lint.SeverityWarning, l07.Severity())
	assert.False(t, l07.AutoFixable())

	tests := []struct {
		name           string
		allStackFiles  []string
		importGraph    map[string][]string
		stacksMap      map[string]any
		rawStackConfig map[string]map[string]any
		basePath       string
		expectFindings bool
	}{
		{
			name:           "no stack files",
			allStackFiles:  []string{},
			importGraph:    map[string][]string{},
			expectFindings: false,
		},
		{
			name:          "all files referenced",
			allStackFiles: []string{"/stacks/deploy/prod.yaml"},
			importGraph: map[string][]string{
				"/stacks/deploy/prod.yaml": {"/stacks/catalog/base.yaml"},
			},
			expectFindings: false,
		},
		{
			name:          "orphaned file detected",
			allStackFiles: []string{"/stacks/catalog/unused.yaml"},
			importGraph:   map[string][]string{},
			stacksMap:     map[string]any{},
			rawStackConfig: map[string]map[string]any{},
			basePath:      "/stacks",
			expectFindings: true,
		},
		{
			name:          "file in stacksMap - not orphaned",
			allStackFiles: []string{"/stacks/deploy/prod.yaml"},
			importGraph:   map[string][]string{},
			stacksMap: map[string]any{
				"/stacks/deploy/prod": map[string]any{},
			},
			expectFindings: false,
		},
		{
			name:          "file in rawStackConfigs - not orphaned",
			allStackFiles: []string{"/stacks/catalog/base.yaml"},
			importGraph:   map[string][]string{},
			rawStackConfig: map[string]map[string]any{
				"/stacks/catalog/base": map[string]any{},
			},
			expectFindings: false,
		},
		{
			name:          "orphaned with no basePath - uses full path",
			allStackFiles: []string{"/stacks/catalog/unused.yaml"},
			importGraph:   map[string][]string{},
			stacksMap:     map[string]any{},
			rawStackConfig: map[string]map[string]any{},
			basePath:      "",
			expectFindings: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rawCfg := tt.rawStackConfig
			if rawCfg == nil {
				rawCfg = make(map[string]map[string]any)
			}
			stacksMap := tt.stacksMap
			if stacksMap == nil {
				stacksMap = make(map[string]any)
			}
			ctx := lint.LintContext{
				StacksMap:       stacksMap,
				RawStackConfigs: rawCfg,
				ImportGraph:     tt.importGraph,
				AllStackFiles:   tt.allStackFiles,
				StacksBasePath:  tt.basePath,
				LintConfig:      schema.LintConfig{},
			}
			findings, err := l07.Run(ctx)
			require.NoError(t, err)
			if tt.expectFindings {
				assert.NotEmpty(t, findings)
				for _, f := range findings {
					assert.Equal(t, "L-07", f.RuleID)
					assert.Equal(t, lint.SeverityWarning, f.Severity)
					assert.NotEmpty(t, f.Message)
					assert.NotEmpty(t, f.FixHint)
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
		extraPatterns   []string
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
		{
			name: "custom pattern matches",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"vars": map[string]any{
						"my_api_key": "abc123",
					},
				},
			},
			extraPatterns:   []string{"*api_key*"},
			expectSensitive: true,
		},
		{
			name: "stack name with path segment gets empty file",
			stacksMap: map[string]any{
				"stacks/deploy/prod": map[string]any{
					"vars": map[string]any{
						"db_password": "secret123",
					},
				},
			},
			expectSensitive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := lint.LintContext{
				StacksMap:       tt.stacksMap,
				RawStackConfigs: make(map[string]map[string]any),
				ImportGraph:     make(map[string][]string),
				LintConfig: schema.LintConfig{
					SensitiveVarPatterns: tt.extraPatterns,
				},
			}
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
		{
			name: "helmfile env shadowing detected",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"env": map[string]any{
						"APP_ENV": "staging",
					},
					"components": map[string]any{
						"helmfile": map[string]any{
							"nginx": map[string]any{
								"env": map[string]any{
									"APP_ENV": "production",
								},
							},
						},
					},
				},
			},
			expectShadowed: true,
		},
		{
			name: "component env not in stack env - no finding",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"env": map[string]any{},
					"components": map[string]any{
						"terraform": map[string]any{
							"mycomp": map[string]any{
								"env": map[string]any{
									"COMPONENT_ONLY_VAR": "value",
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

func TestEngineRunWithSeverityOverride(t *testing.T) {
	t.Parallel()

	// Set up a context that will produce L-08 (sensitive var) findings.
	ctx := lint.LintContext{
		StacksMap: map[string]any{
			"stack1": map[string]any{
				"vars": map[string]any{"my_password": "secret"},
			},
		},
		ImportGraph: map[string][]string{},
		LintConfig: schema.LintConfig{
			Rules: map[string]string{
				"L-08": "error", // override to error
			},
		},
	}

	engine := lint.NewEngine(rules.All())

	// Run with L-08, check severity override applied.
	result, err := engine.Run(ctx, []string{"L-08"}, lint.SeverityInfo)
	require.NoError(t, err)
	require.NotEmpty(t, result.Findings)

	for _, f := range result.Findings {
		if f.RuleID == "L-08" {
			assert.Equal(t, lint.SeverityError, f.Severity, "L-08 should be overridden to error")
		}
	}
	assert.Greater(t, result.Summary.Errors, 0)
}

func TestEngineRunMinSeverityFilter(t *testing.T) {
	t.Parallel()

	// L-08 produces warning findings; filtering to error should hide them.
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

	result, err := engine.Run(ctx, []string{"L-08"}, lint.SeverityError)
	require.NoError(t, err)
	// L-08 default severity is warning; filtering to error should produce no findings.
	assert.Empty(t, result.Findings)
}

func TestEngineRunNoRulesFilter(t *testing.T) {
	t.Parallel()

	// Running engine with no rule filter should run all rules.
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

	result, err := engine.Run(ctx, nil, lint.SeverityInfo)
	require.NoError(t, err)
	// Should have at least L-08 findings.
	assert.NotEmpty(t, result.Findings)
}

func TestEngineRunSortedFindings(t *testing.T) {
	t.Parallel()

	// Create a context with both warnings (L-08) and ensure findings are sorted.
	ctx := lint.LintContext{
		StacksMap: map[string]any{
			"stack-a": map[string]any{
				"vars": map[string]any{"password": "secret1"},
			},
			"stack-b": map[string]any{
				"vars": map[string]any{"api_key": "secret2"},
			},
		},
		ImportGraph: map[string][]string{},
		LintConfig:  schema.LintConfig{},
	}

	engine := lint.NewEngine(rules.All())
	result, err := engine.Run(ctx, []string{"L-08"}, lint.SeverityInfo)
	require.NoError(t, err)

	// Verify findings are sorted by severity (descending), then file, then rule ID.
	for i := 1; i < len(result.Findings); i++ {
		prev := result.Findings[i-1]
		curr := result.Findings[i]
		// Higher severity level should come first.
		if prev.Severity.Level() < curr.Severity.Level() {
			t.Errorf("findings not sorted by severity: %v before %v", prev.Severity, curr.Severity)
		}
	}
}

func TestLintResultHasErrors(t *testing.T) {
	t.Parallel()

	// Test HasErrors method.
	resultNoErrors := &lint.LintResult{
		Summary: lint.LintSummary{Errors: 0},
	}
	assert.False(t, resultNoErrors.HasErrors())

	resultWithErrors := &lint.LintResult{
		Summary: lint.LintSummary{Errors: 1},
	}
	assert.True(t, resultWithErrors.HasErrors())
}

func TestSeverityLevel(t *testing.T) {
	t.Parallel()

	assert.Greater(t, lint.SeverityError.Level(), lint.SeverityWarning.Level())
	assert.Greater(t, lint.SeverityWarning.Level(), lint.SeverityInfo.Level())

	// Unknown severity returns -1.
	unknown := lint.Severity("unknown")
	assert.Equal(t, -1, unknown.Level())
}

func TestDefaultRulesIsNil(t *testing.T) {
	t.Parallel()

	// DefaultRules intentionally returns nil to avoid circular imports.
	defaultRules := lint.DefaultRules()
	assert.Nil(t, defaultRules)
}

// TestAllRulesAutoFixable verifies AutoFixable() is called on every rule,
// ensuring those trivial methods are covered and only L-02 returns true.
func TestAllRulesAutoFixable(t *testing.T) {
	t.Parallel()

	all := rules.All()
	autoFixableRules := make([]string, 0)
	for _, r := range all {
		if r.AutoFixable() {
			autoFixableRules = append(autoFixableRules, r.ID())
		}
	}
	// Only L-02 (Redundant No-Op Override) is auto-fixable.
	assert.Equal(t, []string{"L-02"}, autoFixableRules)
}

// TestHelpersExtractInheritsStringSlice verifies the []string fast-path in extractInherits.
// We use L-09 which internally calls extractInherits; a []string inherits list
// should be handled the same as []any.
func TestHelpersExtractInheritsStringSlice(t *testing.T) {
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

	// Use a []string directly instead of []any to exercise the []string branch.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-a": map[string]any{
				"metadata": map[string]any{
					// Using []string (not []any) to cover the []string branch.
					"inherits": []string{"comp-b"},
				},
			},
			"comp-b": map[string]any{
				"metadata": map[string]any{
					"inherits": []string{"comp-a"},
				},
			},
		}),
	})
	findings, err := l09.Run(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, findings, "should detect cycle even with []string inherits")
}

// TestHelpersAppendIfMissingNoDuplicate ensures appendIfMissing skips duplicates.
// We exercise this via L-09 which calls appendIfMissing for each parent.
// If both stacks declare the same parent for a component, only one edge is recorded.
func TestHelpersAppendIfMissingNoDuplicate(t *testing.T) {
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

	// Two stacks both declare comp-a inheriting comp-base.
	// appendIfMissing should only record the edge once — no cycle should be detected.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-a": map[string]any{
				"metadata": map[string]any{
					"inherits": []any{"comp-base"},
				},
			},
		}),
		"stack2": componentsMap("terraform", map[string]any{
			"comp-a": map[string]any{
				"metadata": map[string]any{
					// Same parent declared again in a second stack — should not create duplicate edge.
					"inherits": []any{"comp-base"},
				},
			},
		}),
	})
	findings, err := l09.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings, "duplicate inherits entries should not create false cycles")
}

// TestL05ConcernGroupSingleSegment covers the "no hyphen" branch in concernGroup.
func TestL05ConcernGroupSingleSegment(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l05 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-05" {
			l05 = r
			break
		}
	}
	require.NotNil(t, l05)

	// Components with no hyphen — each becomes its own concern group.
	// 4 single-word component names exceed the default threshold of 3.
	ctx := lint.LintContext{
		StacksMap: make(map[string]any),
		RawStackConfigs: map[string]map[string]any{
			"/stacks/catalog/mixed.yaml": {
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc":     map[string]any{},
						"ecs":     map[string]any{},
						"rds":     map[string]any{},
						"eks":     map[string]any{},
					},
				},
			},
		},
		ImportGraph: make(map[string][]string),
		LintConfig:  schema.LintConfig{},
	}
	findings, err := l05.Run(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, findings, "four single-segment concern groups should exceed threshold of 3")
}

// TestL08SensitiveVarBasePathResolution covers stackNameToFile with basePath set.
func TestL08SensitiveVarBasePathResolution(t *testing.T) {
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

	ctx := lint.LintContext{
		StacksMap: map[string]any{
			"simple-stack-name": map[string]any{
				"vars": map[string]any{
					"my_token": "abc123",
				},
			},
		},
		RawStackConfigs: make(map[string]map[string]any),
		ImportGraph:     make(map[string][]string),
		LintConfig:      schema.LintConfig{},
		StacksBasePath:  "/stacks",
	}
	findings, err := l08.Run(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)
	for _, f := range findings {
		assert.Equal(t, "L-08", f.RuleID)
	}
}


