package rules_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/lint"
	"github.com/cloudposse/atmos/pkg/lint/rules"
	"github.com/cloudposse/atmos/pkg/schema"
)

// defaultSensitiveVarPatterns mirrors the defaults applied by mergedLintConfig
// in internal/exec/lint_stacks.go.  Tests that exercise L-08 with the default
// patterns must set these explicitly because the rule no longer hard-codes them.
// Note: "*arn*", "*account_id*", and "*role*" were removed from defaults in v4
// because they match ubiquitous infrastructure vars and create noise. They can
// be added as opt-in via lint.stacks.sensitive_var_patterns in atmos.yaml.
var defaultSensitiveVarPatterns = []string{
	"*password*", "*secret*", "*token*", "*key*",
}

// makeContext creates a minimal LintContext for testing.
func makeContext(stacksMap map[string]any) lint.LintContext {
	return lint.LintContext{
		StacksMap:       stacksMap,
		RawStackConfigs: make(map[string]map[string]any),
		ImportGraph:     make(map[string][]string),
		LintConfig:      schema.LintStacksConfig{},
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

// TestL04CatalogDeploySplit verifies that L-04 does not flag abstract components
// when their concrete inheritors live in a different stack (catalog/deploy pattern).
// Before the cross-stack fix, L-04 evaluated per-stack and always flagged abstract
// components in catalog files — making the rule permanently red in standard Atmos repos.
func TestL04CatalogDeploySplit(t *testing.T) {
	t.Parallel()

	l04 := findRuleByID(t, "L-04")

	// Catalog stack: defines the abstract base component.
	// Deploy stack: defines the concrete component that inherits from the catalog base.
	// The two stacks are separate entries in StacksMap (simulating catalog/ vs deploy/ files).
	stacksMap := map[string]any{
		"catalog/network": componentsMap("terraform", map[string]any{
			"vpc": map[string]any{
				"metadata": map[string]any{"type": "abstract"},
			},
		}),
		"deploy/prod": componentsMap("terraform", map[string]any{
			"vpc-prod": map[string]any{
				"metadata": map[string]any{
					"inherits": []any{"vpc"},
				},
			},
		}),
	}

	ctx := makeContext(stacksMap)
	findings, err := l04.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings, "abstract component with concrete inheritor in a different stack must not be flagged")
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

// TestL01DeadVarCrossStack verifies that L-01 uses a cross-stack check.
// A global var in one stack must not be flagged as dead if any component in any
// other stack uses that var key — simulating how Atmos deep-merge works in practice
// (global vars in a catalog/base stack flow to components in deploy stacks).
func TestL01DeadVarCrossStack(t *testing.T) {
	t.Parallel()

	l01 := findRuleByID(t, "L-01")

	// Stack A defines a global var "region".
	// Stack B has a component that explicitly includes "region" in its vars
	// (as the merged StacksMap would after deep-merge delivers the global var).
	stacksMap := map[string]any{
		"catalog/globals": map[string]any{
			"vars": map[string]any{
				"region": "us-east-1",
			},
		},
		"deploy/prod": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{
							"region": "us-east-1", // present in merged component vars
						},
					},
				},
			},
		},
	}

	ctx := makeContext(stacksMap)
	findings, err := l01.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings, "global var consumed in a different stack must not be flagged")
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
				"root": {"l1"},
				"l1":   {"l2"},
				"l2":   {"l3"},
				"l3":   {"l4"},
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
				LintConfig: schema.LintStacksConfig{
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
				LintConfig:      schema.LintStacksConfig{},
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
				LintConfig: schema.LintStacksConfig{
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
			name:           "orphaned file detected",
			allStackFiles:  []string{"/stacks/catalog/unused.yaml"},
			importGraph:    map[string][]string{},
			stacksMap:      map[string]any{},
			rawStackConfig: map[string]map[string]any{},
			basePath:       "/stacks",
			expectFindings: true,
		},
		{
			// StacksMap keys are logical stack names (e.g. "plat-ue2-prod"), not file
			// paths. L-07 now only uses RawStackConfigs for root-file protection.
			// This test confirms that a file only in StacksMap (not in RawStackConfigs
			// or ImportGraph) IS reported as orphaned — the StacksMap key provides no
			// protection because relNorm("plat-ue2-prod") never matches a physical path.
			name:          "file only in stacksMap logical name - still orphaned",
			allStackFiles: []string{"/stacks/deploy/prod.yaml"},
			importGraph:   map[string][]string{},
			stacksMap: map[string]any{
				"plat-ue2-prod": map[string]any{}, // logical name, not a file path
			},
			expectFindings: true,
		},
		{
			// RawStackConfigs keys are absolute file paths and correctly protect root stacks.
			name:          "file in rawStackConfigs - not orphaned",
			allStackFiles: []string{"/stacks/catalog/base.yaml"},
			importGraph:   map[string][]string{},
			rawStackConfig: map[string]map[string]any{
				"/stacks/catalog/base.yaml": {},
			},
			expectFindings: false,
		},
		{
			name:          "absolute import graph key matches absolute allStackFile with basePath",
			allStackFiles: []string{"/stacks/catalog/base.yaml"},
			importGraph: map[string][]string{
				"/stacks/deploy/prod.yaml": {"catalog/base"},
			},
			basePath:       "/stacks",
			expectFindings: false,
		},
		{
			name:          "relative import value without extension matches absolute file",
			allStackFiles: []string{"/stacks/catalog/networking.yaml"},
			importGraph: map[string][]string{
				"/stacks/deploy/prod.yaml": {"catalog/networking"},
			},
			basePath:       "/stacks",
			expectFindings: false,
		},
		{
			name:           "orphaned with no basePath - uses full path",
			allStackFiles:  []string{"/stacks/catalog/unused.yaml"},
			importGraph:    map[string][]string{},
			stacksMap:      map[string]any{},
			rawStackConfig: map[string]map[string]any{},
			basePath:       "",
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
				LintConfig:      schema.LintStacksConfig{},
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
		patterns        []string
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
			patterns:        defaultSensitiveVarPatterns,
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
			patterns:        defaultSensitiveVarPatterns,
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
			patterns:        []string{"*api_key*"},
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
			patterns:        defaultSensitiveVarPatterns,
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
				LintConfig: schema.LintStacksConfig{
					SensitiveVarPatterns: tt.patterns,
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
					cfg.EnvSectionName: map[string]any{
						"MY_VAR": "stack-value",
					},
					"components": map[string]any{
						"terraform": map[string]any{
							"mycomp": map[string]any{
								cfg.EnvSectionName: map[string]any{
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
					cfg.EnvSectionName: map[string]any{
						"MY_VAR": "same-value",
					},
					"components": map[string]any{
						"terraform": map[string]any{
							"mycomp": map[string]any{
								cfg.EnvSectionName: map[string]any{
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
					cfg.EnvSectionName: map[string]any{
						"APP_ENV": "staging",
					},
					"components": map[string]any{
						"helmfile": map[string]any{
							"nginx": map[string]any{
								cfg.EnvSectionName: map[string]any{
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
					cfg.EnvSectionName: map[string]any{},
					"components": map[string]any{
						"terraform": map[string]any{
							"mycomp": map[string]any{
								cfg.EnvSectionName: map[string]any{
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

// TestCfgSectionNameConstants locks the values of cfg constants that test fixtures
// depend on.  If a constant changes, tests that embed its value as a literal would
// silently pass while production behavior would be wrong.
func TestCfgSectionNameConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "env", cfg.EnvSectionName,
		"cfg.EnvSectionName changed — update all env-section test data if intentional")
	assert.Equal(t, "vars", cfg.VarsSectionName,
		"cfg.VarsSectionName changed — update all vars-section test data if intentional")
	assert.Equal(t, "components", cfg.ComponentsSectionName,
		"cfg.ComponentsSectionName changed — update all components-section test data if intentional")
	assert.Equal(t, "metadata", cfg.MetadataSectionName,
		"cfg.MetadataSectionName changed — update all metadata-section test data if intentional")
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
		LintConfig: schema.LintStacksConfig{
			SensitiveVarPatterns: defaultSensitiveVarPatterns,
		},
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
		LintConfig: schema.LintStacksConfig{
			SensitiveVarPatterns: defaultSensitiveVarPatterns,
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
		LintConfig: schema.LintStacksConfig{
			SensitiveVarPatterns: defaultSensitiveVarPatterns,
		},
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
		LintConfig: schema.LintStacksConfig{
			SensitiveVarPatterns: defaultSensitiveVarPatterns,
		},
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
		LintConfig: schema.LintStacksConfig{
			SensitiveVarPatterns: defaultSensitiveVarPatterns,
		},
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

// TestGetNestedMap exercises the getNestedMap helper directly.
func TestGetNestedMap(t *testing.T) {
	t.Parallel()

	t.Run("happy path single key", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{
			cfg.ComponentsSectionName: map[string]any{cfg.TerraformSectionName: map[string]any{}},
		}
		result, ok := rules.ExportedGetNestedMap(m, cfg.ComponentsSectionName)
		require.True(t, ok)
		assert.NotNil(t, result)
	})

	t.Run("happy path nested keys", func(t *testing.T) {
		t.Parallel()
		inner := map[string]any{"vpc": "config"}
		m := map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: inner,
			},
		}
		result, ok := rules.ExportedGetNestedMap(m, cfg.ComponentsSectionName, cfg.TerraformSectionName)
		require.True(t, ok)
		assert.Equal(t, inner, result)
	})

	t.Run("missing key returns false", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{}
		result, ok := rules.ExportedGetNestedMap(m, "missing")
		assert.False(t, ok)
		assert.Nil(t, result)
	})

	t.Run("non-map value at intermediate key returns false", func(t *testing.T) {
		t.Parallel()
		m := map[string]any{
			cfg.ComponentsSectionName: "not-a-map",
		}
		result, ok := rules.ExportedGetNestedMap(m, cfg.ComponentsSectionName, cfg.TerraformSectionName)
		assert.False(t, ok)
		assert.Nil(t, result)
	})

	t.Run("empty key path returns input map", func(t *testing.T) {
		t.Parallel()
		inner := map[string]any{"a": "b"}
		result, ok := rules.ExportedGetNestedMap(inner)
		require.True(t, ok)
		assert.Equal(t, inner, result)
	})
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
						"vpc": map[string]any{},
						"ecs": map[string]any{},
						"rds": map[string]any{},
						"eks": map[string]any{},
					},
				},
			},
		},
		ImportGraph: make(map[string][]string),
		LintConfig:  schema.LintStacksConfig{},
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
		LintConfig: schema.LintStacksConfig{
			SensitiveVarPatterns: defaultSensitiveVarPatterns,
		},
		StacksBasePath: "/stacks",
	}
	findings, err := l08.Run(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)
	for _, f := range findings {
		assert.Equal(t, "L-08", f.RuleID)
	}
}

// -- White-box helper tests (using exported aliases from export_test.go) --

func TestFormatCyclePath(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "A", rules.ExportedFormatCyclePath([]string{"A"}))
	assert.Equal(t, "A → B → A", rules.ExportedFormatCyclePath([]string{"A", "B", "A"}))
}

func TestConcernGroupLeadingHyphen(t *testing.T) {
	t.Parallel()

	// A component name starting with "-" yields an empty first segment
	// so the function returns the full name unchanged.
	result := rules.ExportedConcernGroup("-foo")
	assert.Equal(t, "-foo", result)

	// A normal name uses the first segment.
	assert.Equal(t, "vpc", rules.ExportedConcernGroup("vpc-prod"))
	assert.Equal(t, "vpc", rules.ExportedConcernGroup("vpc"))
}

func TestNormalizeForComparison(t *testing.T) {
	t.Parallel()

	// .yaml extension stripped.
	assert.Equal(t, "stacks/catalog/vpc", rules.ExportedNormalizeForComparison("stacks/catalog/vpc.yaml"))
	// .yml extension stripped.
	assert.Equal(t, "stacks/catalog/ecs", rules.ExportedNormalizeForComparison("stacks/catalog/ecs.yml"))
	// Trailing slash removed (filepath.Clean handles this).
	assert.Equal(t, "stacks/catalog", rules.ExportedNormalizeForComparison("stacks/catalog/"))
	// No extension remains unchanged.
	assert.Equal(t, "stacks/catalog/rds", rules.ExportedNormalizeForComparison("stacks/catalog/rds"))
}

func TestRelNorm(t *testing.T) {
	t.Parallel()

	// Absolute path made relative to basePath, extension stripped.
	assert.Equal(t, "catalog/vpc", rules.ExportedRelNorm("/stacks/catalog/vpc.yaml", "/stacks"))
	// Relative path used as-is after extension strip.
	assert.Equal(t, "catalog/vpc", rules.ExportedRelNorm("catalog/vpc.yaml", "/stacks"))
	// No basePath — absolute path kept, extension stripped.
	assert.Equal(t, "/stacks/catalog/vpc", rules.ExportedRelNorm("/stacks/catalog/vpc.yaml", ""))
	// Relative path without extension unchanged.
	assert.Equal(t, "catalog/base", rules.ExportedRelNorm("catalog/base", "/stacks"))
}

func TestStackNameToFile(t *testing.T) {
	t.Parallel()

	// Empty basePath returns the name as-is.
	assert.Equal(t, "my-stack", rules.ExportedStackNameToFile("my-stack", ""))

	// basePath set but name has no slash/extension → returns "".
	assert.Equal(t, "", rules.ExportedStackNameToFile("my-stack", "/stacks"))

	// Name with slash returns name unchanged (already a path).
	assert.Equal(t, "us-east-1/prod", rules.ExportedStackNameToFile("us-east-1/prod", "/stacks"))

	// Name with .yaml suffix returns name unchanged.
	assert.Equal(t, "prod.yaml", rules.ExportedStackNameToFile("prod.yaml", "/stacks"))

	// Name with .yml suffix returns name unchanged.
	assert.Equal(t, "prod.yml", rules.ExportedStackNameToFile("prod.yml", "/stacks"))
}

// -- Non-map section guards (defensive branch coverage) --

func TestL01DeadVarNonMapStackSection(t *testing.T) {
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

	// A stack section that is not a map[string]any must be skipped gracefully.
	ctx := makeContext(map[string]any{
		"bad-stack": "not-a-map",
	})
	findings, err := l01.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestL01DeadVarNonMapComponentData(t *testing.T) {
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

	// Component data that is not a map[string]any should be skipped gracefully.
	ctx := makeContext(map[string]any{
		"stack1": map[string]any{
			"vars": map[string]any{"unused_var": "value"},
			"components": map[string]any{
				"terraform": map[string]any{
					"comp-a": "not-a-map", // non-map component data
				},
			},
		},
	})
	findings, err := l01.Run(ctx)
	require.NoError(t, err)
	// The global var is unused because component data couldn't be read.
	assert.NotEmpty(t, findings)
}

func TestL02RedundantOverrideNonMapStackSection(t *testing.T) {
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

	ctx := makeContext(map[string]any{
		"bad-stack": "not-a-map",
	})
	findings, err := l02.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestL02RedundantOverrideNoInherits(t *testing.T) {
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

	// Concrete component with no inherits must produce no findings.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-a": map[string]any{
				"vars": map[string]any{"region": "us-east-1"},
			},
		}),
	})
	findings, err := l02.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestL02RedundantOverrideUnknownParent(t *testing.T) {
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

	// Concrete component inheriting from a parent not in the stacks map must produce no findings.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-concrete": map[string]any{
				"metadata": map[string]any{
					"inherits": []any{"comp-unknown-parent"},
				},
				"vars": map[string]any{"region": "us-east-1"},
			},
		}),
	})
	findings, err := l02.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestL02RedundantOverrideAbstractWithNoVars(t *testing.T) {
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

	// Abstract component with no vars section: concrete inheritor cannot be redundant.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-base": map[string]any{
				"metadata": map[string]any{"type": "abstract"},
				// no vars section
			},
			"comp-concrete": map[string]any{
				"metadata": map[string]any{
					"inherits": []any{"comp-base"},
				},
				"vars": map[string]any{"region": "us-east-1"},
			},
		}),
	})
	findings, err := l02.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestL03ImportDepthEmptyImportsEntry(t *testing.T) {
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

	// An entry in ImportGraph with an empty imports slice should be skipped.
	ctx := lint.LintContext{
		StacksMap:       make(map[string]any),
		RawStackConfigs: make(map[string]map[string]any),
		ImportGraph: map[string][]string{
			"stacks/root.yaml": {}, // empty — should hit the continue branch
		},
		LintConfig: schema.LintStacksConfig{MaxImportDepth: 3},
	}
	findings, err := l03.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings, "empty import list should not trigger import depth finding")
}

func TestL03DfsDepthWithDiamondPattern(t *testing.T) {
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

	// Diamond pattern: root imports A and B, both import C.
	// The visited cache in dfsDepth prevents double-counting C.
	ctx := lint.LintContext{
		StacksMap:       make(map[string]any),
		RawStackConfigs: make(map[string]map[string]any),
		ImportGraph: map[string][]string{
			"root.yaml": {"a.yaml", "b.yaml"},
			"a.yaml":    {"c.yaml"},
			"b.yaml":    {"c.yaml"},
		},
		LintConfig: schema.LintStacksConfig{MaxImportDepth: 1},
	}
	// root → a → c gives depth 3 which exceeds threshold 1.
	findings, err := l03.Run(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, findings, "diamond with depth 3 should exceed threshold of 1")
}

func TestL04AbstractLeakNonMapChildData(t *testing.T) {
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

	// Child component data that is not a map must be skipped gracefully.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-abstract": map[string]any{
				"metadata": map[string]any{"type": "abstract"},
			},
			// Non-map "inheritor" that cannot be processed — the abstract component should still leak.
			"comp-bad-child": "not-a-map",
		}),
	})
	findings, err := l04.Run(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, findings, "abstract component with non-map child should still be reported as leaking")
}

func TestL08NonMapStackSection(t *testing.T) {
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

	ctx := makeContext(map[string]any{
		"bad-stack": "not-a-map",
	})
	findings, err := l08.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestL08StackNameWithSlash(t *testing.T) {
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

	// Stack name containing a "/" is treated as a path and returned verbatim by stackNameToFile.
	ctx := lint.LintContext{
		StacksMap: map[string]any{
			"us-east-1/prod": map[string]any{
				"vars": map[string]any{"api_key": "secret"},
			},
		},
		RawStackConfigs: make(map[string]map[string]any),
		ImportGraph:     make(map[string][]string),
		LintConfig: schema.LintStacksConfig{
			SensitiveVarPatterns: defaultSensitiveVarPatterns,
		},
		StacksBasePath: "/stacks",
	}
	findings, err := l08.Run(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, findings)
	assert.Equal(t, "us-east-1/prod", findings[0].File)
}

func TestL09CycleWithHelmfileComponents(t *testing.T) {
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

	// Cycle detection should work for helmfile components as well.
	ctx := makeContext(map[string]any{
		"stack1": map[string]any{
			"components": map[string]any{
				"helmfile": map[string]any{
					"svc-a": map[string]any{
						"metadata": map[string]any{
							"inherits": []any{"svc-b"},
						},
					},
					"svc-b": map[string]any{
						"metadata": map[string]any{
							"inherits": []any{"svc-a"},
						},
					},
				},
			},
		},
	})
	findings, err := l09.Run(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)
}

func TestL09CycleNonMapCompData(t *testing.T) {
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

	// Non-map component data must be skipped gracefully.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-bad": "not-a-map",
		}),
	})
	findings, err := l09.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestL10EnvShadowingNonMapStackSection(t *testing.T) {
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

	ctx := makeContext(map[string]any{
		"bad-stack": "not-a-map",
	})
	findings, err := l10.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestL10EnvShadowingNoComponentsSection(t *testing.T) {
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

	// Stack with env vars but no components section must produce no findings.
	ctx := makeContext(map[string]any{
		"stack1": map[string]any{
			cfg.EnvSectionName: map[string]any{"MY_VAR": "global-value"},
			// no components section
		},
	})
	findings, err := l10.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestExtractInheritsNonListType(t *testing.T) {
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

	// An "inherits" value that is neither []string nor []any should produce no edges
	// (graceful handling of unexpected types).
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-a": map[string]any{
				"metadata": map[string]any{
					"inherits": 42, // unexpected non-slice type
				},
			},
		}),
	})
	findings, err := l09.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings, "non-list inherits value should be ignored without error")
}

// TestL01DeadVarGlobalVarsNoComponents covers the path where a stack has global vars
// but no components section at all.
func TestL01DeadVarGlobalVarsNoComponents(t *testing.T) {
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

	// Stack has a global var but no components section — the var is trivially dead.
	ctx := makeContext(map[string]any{
		"stack1": map[string]any{
			"vars": map[string]any{"orphan_var": "value"},
			// no "components" key
		},
	})
	findings, err := l01.Run(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)
}

// TestL02RedundantOverridePhase2NonMapCompData covers the !ok branch for compData
// in phase 2 of the redundant override rule.
func TestL02RedundantOverridePhase2NonMapCompData(t *testing.T) {
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

	// Components section has one valid abstract comp and one non-map entry.
	// Phase 2 should skip the non-map entry gracefully.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-base": map[string]any{
				"metadata": map[string]any{"type": "abstract"},
				"vars":     map[string]any{"region": "us-east-1"},
			},
			"comp-bad": "not-a-map",
		}),
	})
	findings, err := l02.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestL02RedundantOverrideConcreteNoVars covers the case where a concrete component
// has inherits but no vars section.
func TestL02RedundantOverrideConcreteNoVars(t *testing.T) {
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

	// Concrete component inherits from base but has no vars section.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-base": map[string]any{
				"metadata": map[string]any{"type": "abstract"},
				"vars":     map[string]any{"region": "us-east-1"},
			},
			"comp-concrete": map[string]any{
				"metadata": map[string]any{
					"inherits": []any{"comp-base"},
				},
				// no vars section
			},
		}),
	})
	findings, err := l02.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings, "concrete component with no vars cannot have redundant overrides")
}

// TestL03DfsDepthWithCycle covers the visited-node guard in dfsDepth by providing
// a cyclic import graph.
func TestL03DfsDepthWithCycle(t *testing.T) {
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

	// A↔B cycle in the import graph.
	// dfsDepth uses a visited map to prevent infinite recursion; the visited check
	// fires when A tries to re-enter B's DFS path (or vice versa).
	ctx := lint.LintContext{
		StacksMap:       make(map[string]any),
		RawStackConfigs: make(map[string]map[string]any),
		ImportGraph: map[string][]string{
			"a.yaml": {"b.yaml"},
			"b.yaml": {"a.yaml"}, // cycle
		},
		LintConfig: schema.LintStacksConfig{MaxImportDepth: 1},
	}
	// Must not panic and should report findings since depth exceeds threshold.
	findings, err := l03.Run(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, findings, "cyclic import should exceed depth threshold and not cause infinite recursion")
}

// TestL06DRYNonMapStackSection covers the !ok guard for non-map stack sections.
func TestL06DRYNonMapStackSection(t *testing.T) {
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

	ctx := makeContext(map[string]any{
		"bad-stack": "not-a-map",
	})
	findings, err := l06.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestL06DRYNonMapCompData covers the !ok guard for non-map component data.
func TestL06DRYNonMapCompData(t *testing.T) {
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

	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-bad": "not-a-map",
		}),
	})
	findings, err := l06.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestL10EnvShadowingNonMapCompData covers the !ok guard for non-map component data.
func TestL10EnvShadowingNonMapCompData(t *testing.T) {
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

	ctx := makeContext(map[string]any{
		"stack1": map[string]any{
			cfg.EnvSectionName: map[string]any{"MY_VAR": "global-value"},
			"components":       map[string]any{"terraform": map[string]any{"comp-bad": "not-a-map"}},
		},
	})
	findings, err := l10.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestL09CycleNonMapStackSection covers the !ok guard for non-map stack sections in L-09.
func TestL09CycleNonMapStackSection(t *testing.T) {
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

	ctx := makeContext(map[string]any{
		"bad-stack": "not-a-map",
	})
	findings, err := l09.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestL04AbstractLeakNonMapStackSection covers the !ok guard for non-map stack sections in L-04.
func TestL04AbstractLeakNonMapStackSection(t *testing.T) {
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

	ctx := makeContext(map[string]any{
		"bad-stack": "not-a-map",
	})
	findings, err := l04.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestL06DRYNonMapComponentsSection covers the !ok guard for non-map components section in L-06.
func TestL06DRYNonMapComponentsSection(t *testing.T) {
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

	// Stack with no components section must produce no findings.
	ctx := makeContext(map[string]any{
		"stack1": map[string]any{
			// no "components" key
		},
	})
	findings, err := l06.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestL06DRYComponentWithNoVars covers the case where a component has no vars section.
func TestL06DRYComponentWithNoVars(t *testing.T) {
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

	// Component with no vars section must produce no DRY findings.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-a": map[string]any{
				// no vars section
			},
		}),
	})
	findings, err := l06.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestL08SensitiveVarEmptyVarsMap covers the case where globalVars is an empty map.
func TestL08SensitiveVarEmptyVarsMap(t *testing.T) {
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

	// An empty vars map must not produce findings.
	ctx := makeContext(map[string]any{
		"stack1": map[string]any{
			"vars": map[string]any{}, // empty map
		},
	})
	findings, err := l08.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestL09CycleComponentWithNoMetadata covers the !ok guard for components
// that have no metadata section in L-09.
func TestL09CycleComponentWithNoMetadata(t *testing.T) {
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

	// Component without a metadata section must be skipped gracefully.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-no-meta": map[string]any{
				"vars": map[string]any{"region": "us-east-1"},
				// no "metadata" key
			},
		}),
	})
	findings, err := l09.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestL02RedundantOverrideVarNotInParent covers the parentHas=false branch
// where a concrete component has a var the parent does not define.
func TestL02RedundantOverrideVarNotInParent(t *testing.T) {
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

	// Concrete component has an extra var not present in the parent — no redundancy.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-base": map[string]any{
				"metadata": map[string]any{"type": "abstract"},
				"vars":     map[string]any{"region": "us-east-1"},
			},
			"comp-concrete": map[string]any{
				"metadata": map[string]any{"inherits": []any{"comp-base"}},
				"vars": map[string]any{
					"region": "us-east-1", // matches parent (redundant)
					"extra":  "unique",    // not in parent: parentHas=false → no finding
				},
			},
		}),
	})
	findings, err := l02.Run(ctx)
	require.NoError(t, err)
	// Only "region" is redundant; "extra" is not.
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "region")
}

// TestL06DRYSingleStackNoFinding covers the total<2 guard where a component
// appears in only one stack and therefore cannot be DRY-extracted.
func TestL06DRYSingleStackNoFinding(t *testing.T) {
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

	// A single stack with a component: total=1 < 2, so no DRY finding.
	ctx := makeContext(map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-a": map[string]any{
				"vars": map[string]any{"region": "us-east-1"},
			},
		}),
	})
	findings, err := l06.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings, "single-occurrence var cannot be DRY extracted")
}

// TestL10EnvShadowingEmptyCompEnv covers the len(compEnv)==0 guard.
func TestL10EnvShadowingEmptyCompEnv(t *testing.T) {
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

	// Component with an empty env map must produce no findings.
	ctx := makeContext(map[string]any{
		"stack1": map[string]any{
			cfg.EnvSectionName: map[string]any{"MY_VAR": "global-value"},
			"components": map[string]any{
				"terraform": map[string]any{
					"comp-a": map[string]any{
						cfg.EnvSectionName: map[string]any{}, // empty env map
					},
				},
			},
		},
	})
	findings, err := l10.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// TestL01DeadVarEmptyGlobalVars covers the len(globalVars)==0 guard.
func TestL01DeadVarEmptyGlobalVars(t *testing.T) {
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

	// An empty vars map has zero global vars, so no dead-var findings.
	ctx := makeContext(map[string]any{
		"stack1": map[string]any{
			"vars": map[string]any{}, // present but empty
		},
	})
	findings, err := l01.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// findRuleByID looks up a lint rule by its ID from the full rule set.
// It fails the test immediately if the rule is not found.
func findRuleByID(t *testing.T, ruleID string) lint.LintRule {
	t.Helper()
	for _, r := range rules.All() {
		if r.ID() == ruleID {
			return r
		}
	}
	t.Fatalf("rule %q not found in rules.All()", ruleID)
	return nil
}

// TestL09DFSPathAliasing verifies that cycle path strings are not corrupted when a
// component inherits from multiple parents (branching DFS) and the path slice has excess
// capacity. Before the fix, append(path, node) for sibling iterations shared the same
// backing array, causing the second sibling to overwrite the first sibling's path entries.
func TestL09DFSPathAliasing(t *testing.T) {
	t.Parallel()

	l09 := findRuleByID(t, "L-09")

	// Graph: comp-diamond inherits from both comp-left and comp-right.
	// comp-left inherits comp-base. comp-right inherits comp-base.
	// No cycle — this is a valid diamond inheritance. The fix must not corrupt
	// the path tracking for either branch.
	diamondStacksMap := map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-base": map[string]any{
				"metadata": map[string]any{"type": "abstract"},
			},
			"comp-left": map[string]any{
				"metadata": map[string]any{
					"inherits": []any{"comp-base"},
				},
			},
			"comp-right": map[string]any{
				"metadata": map[string]any{
					"inherits": []any{"comp-base"},
				},
			},
			"comp-diamond": map[string]any{
				"metadata": map[string]any{
					"inherits": []any{"comp-left", "comp-right"},
				},
			},
		}),
	}
	ctx := makeContext(diamondStacksMap)
	findings, err := l09.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings, "diamond inheritance should not be reported as a cycle")

	// Graph with a cycle through a branching node:
	// comp-a inherits comp-b and comp-c.
	// comp-c inherits comp-a (creates cycle: a->c->a).
	// comp-b has no cycle.
	// The cycle path reported must include comp-c, not a corrupted path.
	cycleWithBranchStacksMap := map[string]any{
		"stack1": componentsMap("terraform", map[string]any{
			"comp-b": map[string]any{
				"metadata": map[string]any{"type": "abstract"},
			},
			"comp-c": map[string]any{
				"metadata": map[string]any{
					"inherits": []any{"comp-a"},
				},
			},
			"comp-a": map[string]any{
				"metadata": map[string]any{
					"inherits": []any{"comp-b", "comp-c"},
				},
			},
		}),
	}
	ctx2 := makeContext(cycleWithBranchStacksMap)
	findings2, err := l09.Run(ctx2)
	require.NoError(t, err)
	require.NotEmpty(t, findings2, "cycle through comp-c should be detected")
	// The cycle message must mention comp-a and comp-c.
	found := false
	for _, f := range findings2 {
		if strings.Contains(f.Message, "comp-a") && strings.Contains(f.Message, "comp-c") {
			found = true
			break
		}
	}
	assert.True(t, found, "cycle message must correctly include comp-a and comp-c, not a corrupted path")
}

// TestL08EmptyPatternsNoFindings verifies that L-08 returns no findings when the
// SensitiveVarPatterns list is empty, making the defensive guard contract explicit.
func TestL08EmptyPatternsNoFindings(t *testing.T) {
	t.Parallel()

	l08 := findRuleByID(t, "L-08")

	// A stack with an obviously sensitive-looking global var, but empty patterns.
	stacksMap := map[string]any{
		"stack1": map[string]any{
			"vars": map[string]any{
				"my_secret_password": "hunter2",
			},
		},
	}
	ctx := lint.LintContext{
		StacksMap:       stacksMap,
		RawStackConfigs: make(map[string]map[string]any),
		ImportGraph:     make(map[string][]string),
		LintConfig: schema.LintStacksConfig{
			SensitiveVarPatterns: []string{}, // explicitly empty — must produce no findings
		},
	}
	findings, err := l08.Run(ctx)
	require.NoError(t, err)
	assert.Empty(t, findings, "empty patterns must produce no findings")
}

// TestL02CrossStackAbstractNoCollision verifies that L-02 correctly avoids false
// positives when two different stacks each have an abstract component with the same
// name but different vars (High #4 — key by "<stack>/<component>").
func TestL02CrossStackAbstractNoCollision(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l02 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-02" {
			l02 = r
			break
		}
	}
	require.NotNil(t, l02, "L-02 rule must be present")

	// Two stacks each define an abstract component named "base" with different vars.
	// A concrete component in stack-a inherits from base with the same value as stack-a's abstract.
	// It must NOT match stack-b's abstract (which has a different value).
	stacksMap := map[string]any{
		"stack-a": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"base": map[string]any{
						cfg.MetadataSectionName: map[string]any{"type": "abstract"},
						cfg.VarsSectionName:     map[string]any{"region": "us-east-1"},
					},
					"concrete": map[string]any{
						cfg.MetadataSectionName: map[string]any{
							cfg.InheritsSectionName: []string{"base"},
						},
						// Same value as stack-a's abstract base → genuinely redundant
						cfg.VarsSectionName: map[string]any{"region": "us-east-1"},
					},
				},
			},
		},
		"stack-b": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"base": map[string]any{
						cfg.MetadataSectionName: map[string]any{"type": "abstract"},
						cfg.VarsSectionName:     map[string]any{"region": "eu-west-1"},
					},
				},
			},
		},
	}

	ctx := lint.LintContext{
		StacksMap:       stacksMap,
		RawStackConfigs: make(map[string]map[string]any),
		ImportGraph:     make(map[string][]string),
		LintConfig:      schema.LintStacksConfig{},
	}

	findings, err := l02.Run(ctx)
	require.NoError(t, err)
	// The concrete component in stack-a redundantly overrides "region" to the same
	// value as stack-a's own abstract "base" — this IS a valid finding.
	var stackAFindings []lint.LintFinding
	for _, f := range findings {
		if f.Stack == "stack-a" {
			stackAFindings = append(stackAFindings, f)
		}
	}
	assert.NotEmpty(t, stackAFindings, "redundant override in stack-a must be reported")
	// No spurious finding for stack-b (no concrete component defined there).
	for _, f := range findings {
		assert.NotEqual(t, "stack-b", f.Stack,
			"stack-b has no concrete component — must produce no findings")
	}
}

// TestL05CohesionMaxGroupsConfigurable verifies that L-05 respects the
// CohesionMaxGroups setting from lint config (Medium #7).
func TestL05CohesionMaxGroupsConfigurable(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l05 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-05" {
			l05 = r
			break
		}
	}
	require.NotNil(t, l05, "L-05 rule must be present")

	// Build a raw config with 4 concern groups (alpha-, beta-, gamma-, delta-).
	rawConfig := map[string]map[string]any{
		"/stacks/catalog/mixed.yaml": {
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"alpha-vpc":   map[string]any{},
					"beta-rds":    map[string]any{},
					"gamma-cache": map[string]any{},
					"delta-queue": map[string]any{},
				},
			},
		},
	}

	t.Run("default threshold of 3 triggers finding for 4 groups", func(t *testing.T) {
		ctx := lint.LintContext{
			StacksMap:       make(map[string]any),
			RawStackConfigs: rawConfig,
			ImportGraph:     make(map[string][]string),
			LintConfig:      schema.LintStacksConfig{CohesionMaxGroups: 0}, // 0 → use default (3)
		}
		findings, err := l05.Run(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, findings, "4 groups must exceed default threshold of 3")
	})

	t.Run("custom threshold of 5 suppresses finding for 4 groups", func(t *testing.T) {
		ctx := lint.LintContext{
			StacksMap:       make(map[string]any),
			RawStackConfigs: rawConfig,
			ImportGraph:     make(map[string][]string),
			LintConfig:      schema.LintStacksConfig{CohesionMaxGroups: 5},
		}
		findings, err := l05.Run(ctx)
		require.NoError(t, err)
		assert.Empty(t, findings, "4 groups must NOT exceed custom threshold of 5")
	})

	t.Run("custom threshold of 2 flags even 4 groups", func(t *testing.T) {
		ctx := lint.LintContext{
			StacksMap:       make(map[string]any),
			RawStackConfigs: rawConfig,
			ImportGraph:     make(map[string][]string),
			LintConfig:      schema.LintStacksConfig{CohesionMaxGroups: 2},
		}
		findings, err := l05.Run(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, findings, "4 groups must exceed custom threshold of 2")
	})
}

// TestL08FileAttributionWithIndex verifies that L-08 uses the StackNameToFileIndex
// for reliable file attribution when a stack name resolves to a known manifest
// path in the index (High #6).
func TestL08FileAttributionWithIndex(t *testing.T) {
	t.Parallel()

	all := rules.All()
	var l08 lint.LintRule
	for _, r := range all {
		if r.ID() == "L-08" {
			l08 = r
			break
		}
	}
	require.NotNil(t, l08, "L-08 rule must be present")

	stacksMap := map[string]any{
		"prod": map[string]any{
			cfg.VarsSectionName: map[string]any{
				"api_password": "super-secret",
			},
		},
	}

	ctx := lint.LintContext{
		StacksMap:       stacksMap,
		RawStackConfigs: make(map[string]map[string]any),
		ImportGraph:     make(map[string][]string),
		// StacksBasePath alone would yield "" for "prod" (no path separator), but the
		// index provides the correct absolute path.
		StacksBasePath: "/stacks",
		StackNameToFileIndex: map[string]string{
			"prod": "/stacks/deploy/prod.yaml",
		},
		LintConfig: schema.LintStacksConfig{
			SensitiveVarPatterns: defaultSensitiveVarPatterns,
		},
	}

	findings, err := l08.Run(ctx)
	require.NoError(t, err)
	require.Len(t, findings, 1, "exactly one sensitive var finding expected")
	assert.Equal(t, "/stacks/deploy/prod.yaml", findings[0].File,
		"file must come from StackNameToFileIndex, not the heuristic fallback")
}

// TestCfgSectionNamesExtended extends TestCfgSectionNameConstants to also lock
// TerraformSectionName and HelmfileSectionName values (High #3 — L-05 constants).
func TestCfgSectionNamesExtended(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "terraform", cfg.TerraformSectionName,
		"TerraformSectionName changed — update L-05 and all rules using it")
	assert.Equal(t, "helmfile", cfg.HelmfileSectionName,
		"HelmfileSectionName changed — update L-05 and all rules using it")
}
