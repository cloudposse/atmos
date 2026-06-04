package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// testSaveAndClearRegistry snapshots the CI provider registry, replaces it with
// an empty one, and returns the restore function. Tests that call RunCIHooks
// must use this to avoid depending on the host environment's CI detection
// (e.g., GITHUB_ACTIONS=true on CI runners would make ci.IsCI() return true).
func testSaveAndClearRegistry() func() {
	return ci.SwapRegistryForTest()
}

// testRestoreRegistry restores the CI provider registry from the snapshot
// returned by testSaveAndClearRegistry.
func testRestoreRegistry(restore func()) {
	restore()
}

func TestHasHooks(t *testing.T) {
	tests := []struct {
		name     string
		hooks    Hooks
		expected bool
	}{
		{
			name: "returns false when hooks items is nil",
			hooks: Hooks{
				items: nil,
			},
			expected: false,
		},
		{
			name: "returns false when hooks items is empty",
			hooks: Hooks{
				items: map[string]Hook{},
			},
			expected: false,
		},
		{
			name: "returns true when hooks items has one hook",
			hooks: Hooks{
				items: map[string]Hook{
					"test-hook": {
						Events: []string{"after-terraform-apply"},
						Kind:   "store",
					},
				},
			},
			expected: true,
		},
		{
			name: "returns true when hooks items has multiple hooks",
			hooks: Hooks{
				items: map[string]Hook{
					"hook1": {Events: []string{"after-terraform-apply"}, Kind: "store"},
					"hook2": {Events: []string{"before-terraform-plan"}, Kind: "store"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.hooks.HasHooks()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetHooks(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		info        *schema.ConfigAndStacksInfo
		wantErr     bool
		wantNilMap  bool
	}{
		{
			name:        "empty component and stack returns hooks with nil items",
			atmosConfig: &schema.AtmosConfiguration{},
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
				Stack:            "",
			},
			wantErr:    false,
			wantNilMap: true,
		},
		{
			name:        "empty component only returns hooks with nil items",
			atmosConfig: &schema.AtmosConfiguration{},
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
				Stack:            "test-stack",
			},
			wantErr:    false,
			wantNilMap: true,
		},
		{
			name:        "empty stack only returns hooks with nil items",
			atmosConfig: &schema.AtmosConfiguration{},
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				Stack:            "",
			},
			wantErr:    false,
			wantNilMap: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hooks, err := GetHooks(tt.atmosConfig, tt.info)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, hooks)
			assert.Equal(t, tt.atmosConfig, hooks.config)
			assert.Equal(t, tt.info, hooks.info)

			if tt.wantNilMap {
				assert.Nil(t, hooks.items)
			} else {
				assert.NotNil(t, hooks.items)
			}
		})
	}
}

func TestHooksFromComponent_NoHooksSection(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}

	// Resolved component with no `hooks` key — common for component configs
	// that don't define any lifecycle hooks. Should return empty Hooks, no error.
	resolved := map[string]any{
		"vars":      map[string]any{"foo": "bar"},
		"component": "slack-knowledge",
	}

	hooks, err := HooksFromComponent(atmosConfig, info, resolved)

	require.NoError(t, err)
	require.NotNil(t, hooks)
	assert.False(t, hooks.HasHooks())
	assert.Nil(t, hooks.items)
}

func TestHooksFromComponent_NilComponent(t *testing.T) {
	// nil resolved component (e.g. caller passed nil unintentionally) should
	// degrade to empty Hooks, not nil-deref.
	hooks, err := HooksFromComponent(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, nil)

	require.NoError(t, err)
	require.NotNil(t, hooks)
	assert.False(t, hooks.HasHooks())
}

func TestHooksFromComponent_WithHooks(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "slack-knowledge",
		Stack:            "dev",
	}

	resolved := map[string]any{
		"hooks": map[string]any{
			"store-runtime": map[string]any{
				"events":  []any{"after-apply"},
				"command": "store",
				"name":    "dev/ssm",
				"outputs": map[string]any{
					"/slack-knowledge/agent-id": ".agent_id",
				},
			},
		},
	}

	hooks, err := HooksFromComponent(atmosConfig, info, resolved)

	require.NoError(t, err)
	require.NotNil(t, hooks)
	assert.True(t, hooks.HasHooks())
	require.Contains(t, hooks.items, "store-runtime")

	hook := hooks.items["store-runtime"]
	// The config uses the legacy `command: store` spelling; Eric's Hook
	// UnmarshalYAML promotes it to Kind (clearing Command) for back-compat.
	assert.Equal(t, "store", hook.Kind)
	assert.Equal(t, "dev/ssm", hook.Name)
	assert.Equal(t, []string{"after-apply"}, hook.Events)
	assert.Equal(t, ".agent_id", hook.Outputs["/slack-knowledge/agent-id"])
}

func TestHooksFromComponent_MalformedHooks(t *testing.T) {
	// The `hooks` key is present but not a map (e.g. someone wrote a list by
	// mistake). Should return a clean error, not panic.
	resolved := map[string]any{
		"hooks": []any{"this", "is", "wrong"},
	}

	hooks, err := HooksFromComponent(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, resolved)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "hooks section is not a map")
	assert.NotNil(t, hooks)
	assert.False(t, hooks.HasHooks())
}

func TestGetHooks_WithRealComponent(t *testing.T) {
	// This test uses the hooks-component-scoped test case to test the full GetHooks path
	testDir := "../../tests/test-cases/hooks-component-scoped"

	absTestDir, err := filepath.Abs(testDir)
	require.NoError(t, err)

	// Change to test directory so atmos finds the config
	t.Chdir(absTestDir)

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "acme-dev-test",
	}

	hooks, err := GetHooks(atmosConfig, info)

	require.NoError(t, err)
	assert.NotNil(t, hooks)
	assert.Equal(t, atmosConfig, hooks.config)
	assert.Equal(t, info, hooks.info)
	assert.NotNil(t, hooks.items)
	assert.Contains(t, hooks.items, "vpc-store-outputs")
	assert.Equal(t, "store", hooks.items["vpc-store-outputs"].Kind)
}

func TestGetHooks_DoesNotProcessTemplates(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "stacks", "orgs", "acme"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "stacks", "catalog"), 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "atmos.yaml"),
		[]byte(`base_path: "./"
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  excluded_paths:
    - "catalog/**"
  name_pattern: "{tenant}-{environment}-{stage}"
schemas: {}
logs:
  level: Info
`),
		0o644,
	))

	// The invalid template would fail if ProcessTemplates=true in GetHooks.
	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "stacks", "catalog", "vpc.yaml"),
		[]byte(`components:
  terraform:
    vpc:
      hooks:
        static-hook:
          events:
            - after-terraform-apply
          command: store
          name: prod/ssm
          outputs:
            broken: "{{"
`),
		0o644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "stacks", "orgs", "acme", "_defaults.yaml"),
		[]byte(`import:
  - catalog/vpc
`),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "stacks", "orgs", "acme", "acme-dev-test.yaml"),
		[]byte(`import:
  - orgs/acme/_defaults
vars:
  tenant: acme
  environment: dev
  stage: test
`),
		0o644,
	))

	t.Chdir(tempDir)

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "acme-dev-test",
	}

	hooks, err := GetHooks(atmosConfig, info)
	require.NoError(t, err)
	require.NotNil(t, hooks)
	require.NotNil(t, hooks.items)
	assert.Contains(t, hooks.items, "static-hook")
	assert.Equal(t, "store", hooks.items["static-hook"].Kind)
	assert.Equal(t, "{{", hooks.items["static-hook"].Outputs["broken"])
}

func TestRunAll_RendersStoreHookExecutionFields(t *testing.T) {
	tests := []struct {
		name      string
		storeName string
	}{
		{
			name:      "yaml template function",
			storeName: `!template "{{ index .settings.context.project_to_store .settings.context.project_id }}"`,
		},
		{
			name:      "bare go template",
			storeName: `"{{ index .settings.context.project_to_store .settings.context.project_id }}"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := setupStoreHookTemplateFixture(t, tt.storeName)
			t.Chdir(tempDir)

			mockStore := NewMockStore()
			atmosConfig := &schema.AtmosConfiguration{
				Stores: store.StoreRegistry{
					"staging": mockStore,
				},
			}
			info := &schema.ConfigAndStacksInfo{
				ComponentFromArg: "component",
				Stack:            "acme-dev-test",
			}

			hooks, err := GetHooks(atmosConfig, info)
			require.NoError(t, err)
			require.NotNil(t, hooks)
			require.NotNil(t, hooks.items)
			require.Contains(t, hooks.items["store-outputs"].Name, "project_to_store")

			err = hooks.RunAll(AfterTerraformApply, atmosConfig, info, nil, nil)
			require.NoError(t, err)

			data := mockStore.GetData()
			assert.Equal(t, "my-project", data["acme-dev-test/component/my-project_label"])
		})
	}
}

func TestRunAll_DoesNotRenderNonMatchingStoreHookExecutionFields(t *testing.T) {
	tempDir := setupStoreHookTemplateFixture(t, `!template "{{"`)
	t.Chdir(tempDir)

	atmosConfig := &schema.AtmosConfiguration{Stores: make(store.StoreRegistry)}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "component",
		Stack:            "acme-dev-test",
	}

	hooks, err := GetHooks(atmosConfig, info)
	require.NoError(t, err)
	require.NotNil(t, hooks)
	require.NotNil(t, hooks.items)

	err = hooks.RunAll(BeforeTerraformPlan, atmosConfig, info, nil, nil)
	require.NoError(t, err)
}

func TestResolveHookForExecutionBranches(t *testing.T) {
	t.Run("returns original hook when raw section is unavailable", func(t *testing.T) {
		original := &Hook{Kind: "store", Name: "static-store"}
		hooks := &Hooks{}

		resolved, err := hooks.resolveHookForExecution("missing", original, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{})
		require.NoError(t, err)
		assert.Same(t, original, resolved)
	})

	t.Run("uses stored component sections when info is nil", func(t *testing.T) {
		hooks := &Hooks{
			items: map[string]Hook{
				"store-outputs": {Kind: "store"},
			},
			sections: map[string]any{
				"settings": map[string]any{
					"context": map[string]any{
						"project_id": "my-project",
					},
				},
				"vars": map[string]any{"stage": "test"},
				"hooks": map[string]any{
					"store-outputs": map[string]any{
						"command": "store",
						"name":    "{{ .settings.context.project_id }}",
						"outputs": map[string]any{
							"label": "{{ .vars.stage }}",
						},
					},
				},
			},
		}

		resolved, err := hooks.resolveHookForExecution("store-outputs", &Hook{Kind: "store"}, &schema.AtmosConfiguration{}, nil)
		require.NoError(t, err)
		assert.Equal(t, "store", resolved.Kind)
		assert.Equal(t, "my-project", resolved.Name)
		assert.Equal(t, "test", resolved.Outputs["label"])
	})

	t.Run("returns render error", func(t *testing.T) {
		hooks := &Hooks{
			sections: map[string]any{
				"hooks": map[string]any{
					"broken": map[string]any{
						"command": "store",
						"name":    "{{",
					},
				},
			},
		}

		_, err := hooks.resolveHookForExecution("broken", &Hook{Kind: "store"}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to render hook")
	})
}

func TestProcessHookExecutionValueBranches(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		Stack: "test-stack",
		ComponentSection: map[string]any{
			"settings": map[string]any{
				"context": map[string]any{
					"project_id": "my-project",
				},
			},
		},
	}
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("processes slices from yaml template functions", func(t *testing.T) {
		result, err := processHookExecutionValue(atmosConfig, `!template ["{{ .settings.context.project_id }}", "static"]`, info)
		require.NoError(t, err)
		assert.Equal(t, []any{"my-project", "static"}, result)
	})

	t.Run("processes any maps and non-string keys", func(t *testing.T) {
		result, err := processHookExecutionValue(atmosConfig, map[any]any{
			`{{ .settings.context.project_id }}`: `{{ .settings.context.project_id }}`,
			42:                                   `!template "{{ .settings.context.project_id }}"`,
		}, info)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{
			"my-project": "my-project",
			"42":         "my-project",
		}, result)
	})

	t.Run("preserves scalar defaults and unknown yaml functions", func(t *testing.T) {
		result, err := processHookExecutionValue(atmosConfig, 42, info)
		require.NoError(t, err)
		assert.Equal(t, 42, result)

		result, err = processHookExecutionValue(atmosConfig, "!unknown value", info)
		require.NoError(t, err)
		assert.Equal(t, "!unknown value", result)
	})

	t.Run("returns map key render errors", func(t *testing.T) {
		_, err := processHookExecutionValue(atmosConfig, map[string]any{
			"{{": "value",
		}, info)
		require.Error(t, err)
	})

	t.Run("returns nested value render errors", func(t *testing.T) {
		_, err := processHookExecutionValue(atmosConfig, []any{"{{"}, info)
		require.Error(t, err)
	})
}

func setupStoreHookTemplateFixture(t *testing.T, storeName string) string {
	t.Helper()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "stacks", "orgs", "acme"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "stacks", "catalog"), 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "atmos.yaml"),
		[]byte(`base_path: "./"
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  excluded_paths:
    - "catalog/**"
  name_pattern: "{tenant}-{environment}-{stage}"
schemas: {}
logs:
  level: Info
`),
		0o644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "stacks", "catalog", "component.yaml"),
		[]byte(`components:
  terraform:
    component:
      settings:
        context:
          project_id: my-project
          project_to_store:
            my-project: staging
      vars:
        stage: test
      hooks:
        store-outputs:
          events:
            - after-terraform-apply
          command: store
          name: `+storeName+`
          outputs:
            "{{ .settings.context.project_id }}_label": "{{ .settings.context.project_id }}"
`),
		0o644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "stacks", "orgs", "acme", "_defaults.yaml"),
		[]byte(`import:
  - catalog/component
`),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "stacks", "orgs", "acme", "acme-dev-test.yaml"),
		[]byte(`import:
  - orgs/acme/_defaults
vars:
  tenant: acme
  environment: dev
  stage: test
`),
		0o644,
	))

	return tempDir
}

func TestRunAll(t *testing.T) {
	tests := []struct {
		name        string
		hooks       Hooks
		setupStore  bool
		setupErr    bool
		wantErr     bool
		expectCalls int
	}{
		{
			name: "returns nil when no hooks present",
			hooks: Hooks{
				config: &schema.AtmosConfiguration{},
				info:   &schema.ConfigAndStacksInfo{},
				items:  nil,
			},
			wantErr: false,
		},
		{
			name: "returns nil when hooks map is empty",
			hooks: Hooks{
				config: &schema.AtmosConfiguration{},
				info:   &schema.ConfigAndStacksInfo{},
				items:  map[string]Hook{},
			},
			wantErr: false,
		},
		{
			name: "executes single store hook successfully",
			hooks: Hooks{
				config: &schema.AtmosConfiguration{
					Stores: make(store.StoreRegistry),
				},
				info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
				},
				items: map[string]Hook{
					"test-hook": {
						Events: []string{"after-terraform-apply"},
						Kind:   "store",
						Name:   "test-store",
						Outputs: map[string]string{
							"key1": "value1",
						},
					},
				},
			},
			setupStore:  true,
			wantErr:     false,
			expectCalls: 1,
		},
		{
			name: "executes multiple store hooks successfully",
			hooks: Hooks{
				config: &schema.AtmosConfiguration{
					Stores: make(store.StoreRegistry),
				},
				info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
				},
				items: map[string]Hook{
					"hook1": {
						Events:  []string{"after-terraform-apply"},
						Kind:    "store",
						Name:    "store1",
						Outputs: map[string]string{"key1": "value1"},
					},
					"hook2": {
						Events:  []string{"after-terraform-apply"},
						Kind:    "store",
						Name:    "store2",
						Outputs: map[string]string{"key2": "value2"},
					},
				},
			},
			setupStore:  true,
			wantErr:     false,
			expectCalls: 2,
		},
		{
			name: "returns error when store not found",
			hooks: Hooks{
				config: &schema.AtmosConfiguration{
					Stores: make(store.StoreRegistry),
				},
				info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
				},
				items: map[string]Hook{
					"test-hook": {
						Events:  []string{"after-terraform-apply"},
						Kind:    "store",
						Name:    "nonexistent-store",
						Outputs: map[string]string{"key1": "value1"},
					},
				},
			},
			setupStore:  false,
			wantErr:     true,
			expectCalls: 0,
		},
		{
			name: "returns error when store Set fails",
			hooks: Hooks{
				config: &schema.AtmosConfiguration{
					Stores: make(store.StoreRegistry),
				},
				info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
				},
				items: map[string]Hook{
					"test-hook": {
						Events:  []string{"after-terraform-apply"},
						Kind:    "store",
						Name:    "test-store",
						Outputs: map[string]string{"key1": "value1"},
					},
				},
			},
			setupStore:  true,
			setupErr:    true,
			wantErr:     true,
			expectCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock stores if needed
			if tt.setupStore && tt.hooks.config != nil {
				for _, hook := range tt.hooks.items {
					mockStore := NewMockStore()
					if tt.setupErr {
						mockStore.SetSetError(errors.New("store error"))
					}
					tt.hooks.config.Stores[hook.Name] = mockStore
				}
			}

			err := tt.hooks.RunAll(AfterTerraformApply, tt.hooks.config, tt.hooks.info, nil, nil)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestRunAll_EventFiltering verifies that RunAll only executes hooks whose Events list
// includes the current event. This is the guard that prevents after-terraform-apply hooks
// from firing during before-terraform-apply (and vice-versa).
func TestRunAll_EventFiltering(t *testing.T) {
	makeHooks := func(events []string) Hooks {
		mockStore := NewMockStore()
		cfg := &schema.AtmosConfiguration{Stores: make(store.StoreRegistry)}
		cfg.Stores["test-store"] = mockStore
		return Hooks{
			config: cfg,
			info:   &schema.ConfigAndStacksInfo{ComponentFromArg: "comp", Stack: "stack"},
			items: map[string]Hook{
				"hook": {
					Events: events,
					Kind:   "store",
					Name:   "test-store",
					// Literal value (no dot prefix) — no terraform output call needed.
					Outputs: map[string]string{"label_id": "literal-value"},
				},
			},
		}
	}

	getStore := func(h Hooks) *MockStore {
		return h.config.Stores["test-store"].(*MockStore)
	}

	t.Run("after-apply hook does not run on before-apply event", func(t *testing.T) {
		h := makeHooks([]string{"after-terraform-apply"})
		err := h.RunAll(BeforeTerraformApply, h.config, h.info, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, getStore(h).GetData(), "store must not be called when event does not match")
	})

	t.Run("after-apply hook runs on after-apply event", func(t *testing.T) {
		h := makeHooks([]string{"after-terraform-apply"})
		err := h.RunAll(AfterTerraformApply, h.config, h.info, nil, nil)
		require.NoError(t, err)
		data := getStore(h).GetData()
		assert.Equal(t, "literal-value", data["stack/comp/label_id"], "store must be called when event matches")
	})

	t.Run("hook with dot-format event matches correctly", func(t *testing.T) {
		h := makeHooks([]string{"after.terraform.apply"})
		err := h.RunAll(AfterTerraformApply, h.config, h.info, nil, nil)
		require.NoError(t, err)
		data := getStore(h).GetData()
		assert.Equal(t, "literal-value", data["stack/comp/label_id"], "dot-format event must also match")
	})

	t.Run("hook with multiple events only runs on matching event", func(t *testing.T) {
		h := makeHooks([]string{"before-terraform-plan", "after-terraform-apply"})
		err := h.RunAll(BeforeTerraformApply, h.config, h.info, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, getStore(h).GetData(), "store must not be called for non-matching event")
	})

	// Cross-fire: apply and deploy are aliases — hooks configured for either fire on both.
	t.Run("after-terraform-apply hook fires when deploy command runs", func(t *testing.T) {
		h := makeHooks([]string{"after-terraform-apply"})
		err := h.RunAll(AfterTerraformDeploy, h.config, h.info, nil, nil)
		require.NoError(t, err)
		data := getStore(h).GetData()
		assert.Equal(t, "literal-value", data["stack/comp/label_id"], "apply hook must fire on deploy event")
	})

	t.Run("after-terraform-deploy hook fires when apply command runs", func(t *testing.T) {
		h := makeHooks([]string{"after-terraform-deploy"})
		err := h.RunAll(AfterTerraformApply, h.config, h.info, nil, nil)
		require.NoError(t, err)
		data := getStore(h).GetData()
		assert.Equal(t, "literal-value", data["stack/comp/label_id"], "deploy hook must fire on apply event")
	})
}

func TestRunAll_SkipHooksBypassesPreflightBinaryCheck(t *testing.T) {
	prev := viper.Get("skip-hooks")
	viper.Set("skip-hooks", "missing-tool")
	t.Cleanup(func() { viper.Set("skip-hooks", prev) })

	h := Hooks{
		config: &schema.AtmosConfiguration{},
		info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "test-component",
			Stack:            "test-stack",
		},
		items: map[string]Hook{
			"missing-tool": {
				Kind:    "command",
				Command: "definitely-not-on-path-atmos-test",
			},
		},
	}

	err := h.RunAll(BeforeTerraformPlan, h.config, h.info, nil, nil)
	require.NoError(t, err)
}

func TestHooksPreflight_NoOpBranches(t *testing.T) {
	skipNone := func(string) bool { return false }

	tests := []struct {
		name  string
		hooks Hooks
		cfg   *schema.AtmosConfiguration
		info  *schema.ConfigAndStacksInfo
		skip  func(string) bool
	}{
		{
			name: "already done",
			hooks: Hooks{
				preflightDone: true,
				items: map[string]Hook{
					"missing": {Kind: "command", Command: "definitely-not-on-path-atmos-test"},
				},
			},
			cfg:  &schema.AtmosConfiguration{},
			info: &schema.ConfigAndStacksInfo{ComponentFromArg: "component", Stack: "stack"},
			skip: skipNone,
		},
		{
			name:  "empty hooks",
			hooks: Hooks{items: map[string]Hook{}},
			cfg:   &schema.AtmosConfiguration{},
			info:  &schema.ConfigAndStacksInfo{ComponentFromArg: "component", Stack: "stack"},
			skip:  skipNone,
		},
		{
			name: "nil config",
			hooks: Hooks{items: map[string]Hook{
				"hook": {Kind: "command", Command: "tool"},
			}},
			cfg:  nil,
			info: &schema.ConfigAndStacksInfo{ComponentFromArg: "component", Stack: "stack"},
			skip: skipNone,
		},
		{
			name: "nil info",
			hooks: Hooks{items: map[string]Hook{
				"hook": {Kind: "command", Command: "tool"},
			}},
			cfg:  &schema.AtmosConfiguration{},
			info: nil,
			skip: skipNone,
		},
		{
			name: "all hooks skipped",
			hooks: Hooks{items: map[string]Hook{
				"hook": {Kind: "command", Command: "definitely-not-on-path-atmos-test"},
			}},
			cfg:  &schema.AtmosConfiguration{},
			info: &schema.ConfigAndStacksInfo{ComponentFromArg: "component", Stack: "stack"},
			skip: func(string) bool { return true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.hooks.preflight(tt.cfg, tt.info, tt.skip)
			require.NoError(t, err)
			assert.True(t, tt.hooks.preflightDone)
		})
	}
}

func TestHooksVerifyAllBinaries(t *testing.T) {
	t.Run("skips deprecated unknown skipped and no-command hooks", func(t *testing.T) {
		h := Hooks{items: map[string]Hook{
			"deprecated": {Kind: "ci.summary", Command: "definitely-not-on-path-atmos-test"},
			"unknown":    {Kind: "not-registered", Command: "definitely-not-on-path-atmos-test"},
			"store":      {Kind: "store"},
			"skipped":    {Kind: "command", Command: "definitely-not-on-path-atmos-test"},
		}}

		err := h.verifyAllBinaries(func(name string) bool { return name == "skipped" })
		require.NoError(t, err)
	})

	t.Run("returns command-not-found for missing command hook", func(t *testing.T) {
		h := Hooks{items: map[string]Hook{
			"missing": {Kind: "command", Command: "definitely-not-on-path-atmos-test"},
		}}

		err := h.verifyAllBinaries(nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommandNotFound)
	})
}

func TestIsDeprecatedCIKind(t *testing.T) {
	tests := []struct {
		name string
		kind string
		want bool
	}{
		{name: "ci check", kind: "ci.check", want: true},
		{name: "ci output", kind: "ci.output", want: true},
		{name: "ci summary", kind: "ci.summary", want: true},
		{name: "ci upload", kind: "ci.upload", want: true},
		{name: "ci download", kind: "ci.download", want: true},
		{name: "command", kind: "command", want: false},
		{name: "store", kind: "store", want: false},
		{name: "empty", kind: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDeprecatedCIKind(tt.kind))
		})
	}
}

// TestRunCIHooks_CIEnabledIsHardKillSwitch verifies that ci.enabled in atmos.yaml
// is the authority for CI hooks. When ci.enabled is false (or not set, which defaults
// to false), CI hooks must not run even when --ci flag is passed (forceCIMode=true).
func TestRunCIHooks_CIEnabledIsHardKillSwitch(t *testing.T) {
	tests := []struct {
		name        string
		ciEnabled   bool
		forceCIMode bool
		expectNoop  bool
	}{
		{
			name:        "ci.enabled=false and forceCIMode=false skips CI hooks",
			ciEnabled:   false,
			forceCIMode: false,
			expectNoop:  true,
		},
		{
			name:        "ci.enabled=false and forceCIMode=true still skips CI hooks",
			ciEnabled:   false,
			forceCIMode: true,
			expectNoop:  true,
		},
		{
			name:        "ci.enabled not set (defaults to false) and forceCIMode=true still skips CI hooks",
			ciEnabled:   false, // zero value = not set in YAML.
			forceCIMode: true,
			expectNoop:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{
				CI: schema.CIConfig{Enabled: tc.ciEnabled},
			}
			info := &schema.ConfigAndStacksInfo{
				Stack:            "test-stack",
				ComponentFromArg: "test-component",
			}

			// RunCIHooks should return nil immediately without reaching ci.Execute.
			// If it did reach ci.Execute with a bogus event, the event would not
			// match any binding and ci.Execute returns nil anyway — but the key
			// assertion is that RunCIHooks itself short-circuits.
			err := RunCIHooks(&RunCIHooksOptions{
				Event:       "before.terraform.plan",
				AtmosConfig: config,
				Info:        info,
				ForceCIMode: tc.forceCIMode,
			})
			assert.NoError(t, err)
		})
	}
}

// TestRunCIHooks_LocalRunSkipsExperimentalGate verifies that local runs do not
// hit the experimental CI gate unless CI mode is explicitly forced.
func TestRunCIHooks_LocalRunSkipsExperimentalGate(t *testing.T) {
	// Disable all registered CI providers for the duration of this test so the
	// first subtest's ci.IsCI() check returns false regardless of the host
	// environment (e.g., when this suite runs under GitHub Actions itself).
	// Without this isolation, the github provider's Detect() would see
	// GITHUB_ACTIONS=true and cause the non-force branch to fall through to
	// the experimental gate, failing the test.
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	config := &schema.AtmosConfiguration{
		CI: schema.CIConfig{Enabled: true},
		Settings: schema.AtmosSettings{
			Experimental: "disable",
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "test-stack",
		ComponentFromArg: "test-component",
	}

	t.Run("local run without force skips CI hooks before experimental gate", func(t *testing.T) {
		err := RunCIHooks(&RunCIHooksOptions{
			Event:       "before.terraform.plan",
			AtmosConfig: config,
			Info:        info,
			ForceCIMode: false,
		})
		assert.NoError(t, err)
	})

	t.Run("forced CI mode still evaluates the experimental gate", func(t *testing.T) {
		err := RunCIHooks(&RunCIHooksOptions{
			Event:       "before.terraform.plan",
			AtmosConfig: config,
			Info:        info,
			ForceCIMode: true,
		})
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrExperimentalDisabled), "expected %v, got %v", errUtils.ErrExperimentalDisabled, err)
	})
}

// TestRunCIHooks_ForwardsErrorAndExitCode verifies that RunCIHooksOptions
// fields (CommandError, ExitCode) flow through to ci.Execute when CI is
// enabled. We trigger a clean early exit inside ci.Execute by using an
// unhandled event so no real plugin is invoked.
func TestRunCIHooks_ForwardsErrorAndExitCode(t *testing.T) {
	tests := []struct {
		name         string
		commandError error
		exitCode     int
	}{
		{
			name:         "nil error and zero exit code (success path)",
			commandError: nil,
			exitCode:     0,
		},
		{
			name:         "wrapped ExitCodeError with code 1",
			commandError: errUtils.ExitCodeError{Code: 1},
			exitCode:     1,
		},
		{
			name:         "plan exit code 2 with wrapped error (changes detected)",
			commandError: errUtils.ExitCodeError{Code: 2},
			exitCode:     2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{
				CI:       schema.CIConfig{Enabled: true},
				Settings: schema.AtmosSettings{Experimental: "silence"},
			}
			info := &schema.ConfigAndStacksInfo{
				Stack:            "dev",
				ComponentFromArg: "vpc",
			}

			// "unhandled.event" has no registered plugin binding, so ci.Execute
			// returns nil cleanly after platform/binding lookup. We just need
			// RunCIHooks itself to construct the ExecuteOptions correctly and
			// not panic on the new ExitCode/CommandError fields.
			err := RunCIHooks(&RunCIHooksOptions{
				Event:        "unhandled.event",
				AtmosConfig:  config,
				Info:         info,
				ForceCIMode:  true, // forces generic provider so platform detection succeeds.
				CommandError: tc.commandError,
				ExitCode:     tc.exitCode,
			})
			assert.NoError(t, err)
		})
	}
}

// TestRunCIHooks_NilAtmosConfig verifies RunCIHooks does not panic when
// AtmosConfig is nil — ci.enabled and experimental checks are skipped and
// the call still completes cleanly (ci.Execute returns nil with no platform
// detected).
func TestRunCIHooks_NilAtmosConfig(t *testing.T) {
	err := RunCIHooks(&RunCIHooksOptions{
		Event:       "before.terraform.plan",
		AtmosConfig: nil,
		Info:        &schema.ConfigAndStacksInfo{},
		ForceCIMode: false,
	})
	assert.NoError(t, err)
}

// TestRunCIHooks_ExperimentalDisableReturnsError verifies RunCIHooks
// short-circuits with an experimental-disabled error when CI is enabled
// in atmos.yaml but settings.experimental is set to "disable".
func TestRunCIHooks_ExperimentalDisableReturnsError(t *testing.T) {
	config := &schema.AtmosConfiguration{
		CI:       schema.CIConfig{Enabled: true},
		Settings: schema.AtmosSettings{Experimental: "disable"},
	}

	err := RunCIHooks(&RunCIHooksOptions{
		Event:        "after.terraform.plan",
		AtmosConfig:  config,
		Info:         &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"},
		ForceCIMode:  true,
		CommandError: errUtils.ExitCodeError{Code: 1},
		ExitCode:     1,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrExperimentalDisabled),
		"expected ErrExperimentalDisabled, got %v", err)
}

// TestCheckExperimental verifies that checkExperimental gates CI hooks
// based on settings.experimental, mirroring the command-level behavior.
func TestCheckExperimental(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		expectErr   bool
		expectedErr error
	}{
		{
			name:      "empty defaults to warn (no error)",
			mode:      "",
			expectErr: false,
		},
		{
			name:      "silence allows CI hooks",
			mode:      "silence",
			expectErr: false,
		},
		{
			name:      "warn allows CI hooks",
			mode:      "warn",
			expectErr: false,
		},
		{
			name:        "disable blocks CI hooks",
			mode:        "disable",
			expectErr:   true,
			expectedErr: errUtils.ErrExperimentalDisabled,
		},
		{
			name:        "error blocks CI hooks",
			mode:        "error",
			expectErr:   true,
			expectedErr: errUtils.ErrExperimentalRequiresIn,
		},
		{
			name:      "unknown mode treated as warn",
			mode:      "unknown-value",
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Experimental: tc.mode,
				},
			}

			err := checkExperimental(config)
			if tc.expectErr {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tc.expectedErr), "expected %v, got %v", tc.expectedErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
