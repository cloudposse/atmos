package hooks

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// nopEngine is a no-op engine used only for registry tests.
type nopEngine struct{}

func (nopEngine) Run(_ *ExecContext) (*Output, error) { return nil, nil }

func TestRegisterKind(t *testing.T) {
	// Snapshot and restore the registry around each subtest so registrations
	// from init() (e.g., the built-in "store" kind) aren't disturbed.
	withCleanRegistry := func(t *testing.T, fn func(t *testing.T)) {
		t.Helper()
		saved := snapshotKinds()
		t.Cleanup(func() { restoreKinds(saved) })
		fn(t)
	}

	t.Run("registers a new kind", func(t *testing.T) {
		withCleanRegistry(t, func(t *testing.T) {
			ClearKinds()
			err := RegisterKind(&Kind{Name: "mykind", Engine: nopEngine{}})
			require.NoError(t, err)

			got, ok := GetKind("mykind")
			require.True(t, ok)
			assert.Equal(t, "mykind", got.Name)
		})
	})

	t.Run("rejects nil kind", func(t *testing.T) {
		err := RegisterKind(nil)
		require.Error(t, err)
	})

	t.Run("rejects empty name", func(t *testing.T) {
		err := RegisterKind(&Kind{Engine: nopEngine{}})
		require.Error(t, err)
	})

	t.Run("rejects kind without engine", func(t *testing.T) {
		withCleanRegistry(t, func(t *testing.T) {
			ClearKinds()
			err := RegisterKind(&Kind{Name: "noengine"})
			require.Error(t, err)
		})
	})

	t.Run("rejects duplicate registration", func(t *testing.T) {
		withCleanRegistry(t, func(t *testing.T) {
			ClearKinds()
			require.NoError(t, RegisterKind(&Kind{Name: "dup", Engine: nopEngine{}}))
			err := RegisterKind(&Kind{Name: "dup", Engine: nopEngine{}})
			require.Error(t, err)
		})
	})

	t.Run("ListKinds returns sorted names", func(t *testing.T) {
		withCleanRegistry(t, func(t *testing.T) {
			ClearKinds()
			require.NoError(t, RegisterKind(&Kind{Name: "trivy", Engine: nopEngine{}}))
			require.NoError(t, RegisterKind(&Kind{Name: "checkov", Engine: nopEngine{}}))
			require.NoError(t, RegisterKind(&Kind{Name: "infracost", Engine: nopEngine{}}))
			assert.Equal(t, []string{"checkov", "infracost", "trivy"}, ListKinds())
		})
	})

	t.Run("built-in store kind is registered from init()", func(t *testing.T) {
		k, ok := GetKind("store")
		require.True(t, ok, "store kind should self-register via init()")
		require.NotNil(t, k.Engine)
	})
}

func TestKind_ResolveDefaults(t *testing.T) {
	kind := &Kind{
		Name:        "infracost",
		Command:     "infracost",
		DefaultArgs: []string{"breakdown", "--path", "$ATMOS_COMPONENT_PATH"},
		DefaultEnv:  map[string]string{"INFRACOST_LOG_LEVEL": "info"},
		OnFailure:   "warn",
	}

	t.Run("fills in defaults when hook fields are empty", func(t *testing.T) {
		hook := &Hook{Kind: "infracost"}
		resolved := kind.ResolveDefaults(hook)

		assert.Equal(t, "infracost", resolved.Command)
		assert.Equal(t, []string{"breakdown", "--path", "$ATMOS_COMPONENT_PATH"}, resolved.Args)
		assert.Equal(t, map[string]string{"INFRACOST_LOG_LEVEL": "info"}, resolved.Env)
		assert.Equal(t, "warn", resolved.OnFailure)
	})

	t.Run("preserves hook overrides", func(t *testing.T) {
		hook := &Hook{
			Kind:      "infracost",
			Command:   "/custom/infracost",
			Args:      []string{"--custom"},
			Env:       map[string]string{"OTHER": "var"},
			OnFailure: "fail",
		}
		resolved := kind.ResolveDefaults(hook)

		assert.Equal(t, "/custom/infracost", resolved.Command)
		assert.Equal(t, []string{"--custom"}, resolved.Args)
		assert.Equal(t, map[string]string{"OTHER": "var"}, resolved.Env)
		assert.Equal(t, "fail", resolved.OnFailure)
	})

	t.Run("does not mutate the input hook", func(t *testing.T) {
		hook := &Hook{Kind: "infracost"}
		_ = kind.ResolveDefaults(hook)
		assert.Empty(t, hook.Command, "input hook should remain unchanged")
		assert.Empty(t, hook.Args)
	})

	t.Run("default args are copied (not shared with kind)", func(t *testing.T) {
		hook := &Hook{Kind: "infracost"}
		resolved := kind.ResolveDefaults(hook)
		require.NotEmpty(t, resolved.Args)
		resolved.Args[0] = "mutated"
		assert.Equal(t, "breakdown", kind.DefaultArgs[0], "kind defaults must not be mutated")
	})
}

func TestHook_UnmarshalYAML_LegacyAlias(t *testing.T) {
	tests := []struct {
		name        string
		yamlInput   string
		wantKind    string
		wantCommand string
		wantName    string
	}{
		{
			name: "legacy command:store is treated as kind:store",
			yamlInput: `
command: store
name: vpc/id
outputs:
  id: .vpc_id
`,
			wantKind:    "store",
			wantCommand: "",
			wantName:    "vpc/id",
		},
		{
			name: "new kind:store with no command",
			yamlInput: `
kind: store
name: vpc/id
outputs:
  id: .vpc_id
`,
			wantKind:    "store",
			wantCommand: "",
			wantName:    "vpc/id",
		},
		{
			name: "new kind:command with command:binary",
			yamlInput: `
kind: command
command: trivy
args: ["--format", "sarif"]
`,
			wantKind:    "command",
			wantCommand: "trivy",
		},
		{
			name: "kind:infracost with no command (kind defaults supply it later)",
			yamlInput: `
kind: infracost
`,
			wantKind:    "infracost",
			wantCommand: "",
		},
		{
			name: "both kind and command set explicitly — neither moves",
			yamlInput: `
kind: command
command: my-scanner
`,
			wantKind:    "command",
			wantCommand: "my-scanner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h Hook
			err := yaml.Unmarshal([]byte(tt.yamlInput), &h)
			require.NoError(t, err)
			assert.Equal(t, tt.wantKind, h.Kind, "Kind")
			assert.Equal(t, tt.wantCommand, h.Command, "Command")
			if tt.wantName != "" {
				assert.Equal(t, tt.wantName, h.Name)
			}
		})
	}
}

func TestHook_UnmarshalYAML_NewFields(t *testing.T) {
	yamlInput := `
kind: command
command: my-scanner
args:
  - "--component"
  - "$ATMOS_COMPONENT_PATH"
env:
  SCANNER_PROFILE: strict
format: markdown
on_failure: fail
events:
  - after-terraform-plan
`
	var h Hook
	require.NoError(t, yaml.Unmarshal([]byte(yamlInput), &h))

	assert.Equal(t, "command", h.Kind)
	assert.Equal(t, "my-scanner", h.Command)
	assert.Equal(t, []string{"--component", "$ATMOS_COMPONENT_PATH"}, h.Args)
	assert.Equal(t, "strict", h.Env["SCANNER_PROFILE"])
	assert.Equal(t, "markdown", h.Format)
	assert.Equal(t, "fail", h.OnFailure)
	assert.Equal(t, []string{"after-terraform-plan"}, h.Events)
}

// errEngine returns the provided error for failure-path testing.
type errEngine struct{ err error }

func (e errEngine) Run(_ *ExecContext) (*Output, error) { return nil, e.err }

func TestRegisterKind_NilPropagatesErrors(t *testing.T) {
	saved := snapshotKinds()
	t.Cleanup(func() { restoreKinds(saved) })

	ClearKinds()
	require.NoError(t, RegisterKind(&Kind{
		Name:   "explodes",
		Engine: errEngine{err: errors.New("boom")},
	}))

	k, ok := GetKind("explodes")
	require.True(t, ok)
	_, err := k.Engine.Run(&ExecContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// snapshotKinds + restoreKinds let tests mutate the kind registry without
// destroying the init-registered built-in `store` kind for sibling tests.
func snapshotKinds() map[string]*Kind {
	kindsMu.RLock()
	defer kindsMu.RUnlock()
	out := make(map[string]*Kind, len(kinds))
	for k, v := range kinds {
		out[k] = v
	}
	return out
}

func restoreKinds(saved map[string]*Kind) {
	kindsMu.Lock()
	defer kindsMu.Unlock()
	kinds = make(map[string]*Kind, len(saved))
	for k, v := range saved {
		kinds[k] = v
	}
}
