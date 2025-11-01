package flagparser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestNewFlagRegistry(t *testing.T) {
	registry := NewFlagRegistry()
	assert.NotNil(t, registry)
	assert.Equal(t, 0, registry.Count())
}

func TestFlagRegistry_Register(t *testing.T) {
	registry := NewFlagRegistry()

	flag := &StringFlag{Name: "test"}
	registry.Register(flag)

	assert.Equal(t, 1, registry.Count())
	assert.True(t, registry.Has("test"))
	assert.Equal(t, flag, registry.Get("test"))
}

func TestFlagRegistry_RegisterReplaces(t *testing.T) {
	registry := NewFlagRegistry()

	flag1 := &StringFlag{Name: "test", Default: "first"}
	registry.Register(flag1)

	flag2 := &StringFlag{Name: "test", Default: "second"}
	registry.Register(flag2)

	// Should have only one flag (replaced)
	assert.Equal(t, 1, registry.Count())

	retrieved := registry.Get("test")
	stringFlag := retrieved.(*StringFlag)
	assert.Equal(t, "second", stringFlag.Default)
}

func TestFlagRegistry_Get(t *testing.T) {
	registry := NewFlagRegistry()

	flag := &StringFlag{Name: "exists"}
	registry.Register(flag)

	assert.Equal(t, flag, registry.Get("exists"))
	assert.Nil(t, registry.Get("nonexistent"))
}

func TestFlagRegistry_Has(t *testing.T) {
	registry := NewFlagRegistry()

	flag := &StringFlag{Name: "exists"}
	registry.Register(flag)

	assert.True(t, registry.Has("exists"))
	assert.False(t, registry.Has("nonexistent"))
}

func TestFlagRegistry_All(t *testing.T) {
	registry := NewFlagRegistry()

	flag1 := &StringFlag{Name: "flag1"}
	flag2 := &BoolFlag{Name: "flag2"}

	registry.Register(flag1)
	registry.Register(flag2)

	all := registry.All()
	assert.Equal(t, 2, len(all))

	// Verify the returned slice is a copy (safe to modify)
	_ = append(all, &IntFlag{Name: "flag3"})
	assert.Equal(t, 2, registry.Count()) // Original registry unchanged
}

func TestCommonFlags(t *testing.T) {
	registry := CommonFlags()

	// Should have 3 common flags: stack, identity, dry-run
	assert.Equal(t, 3, registry.Count())

	// Check stack flag
	stackFlag := registry.Get("stack")
	require.NotNil(t, stackFlag)
	assert.Equal(t, "stack", stackFlag.GetName())
	assert.Equal(t, "s", stackFlag.GetShorthand())
	assert.Equal(t, []string{"ATMOS_STACK"}, stackFlag.GetEnvVars())

	// Check identity flag
	identityFlag := registry.Get(cfg.IdentityFlagName)
	require.NotNil(t, identityFlag)
	assert.Equal(t, "identity", identityFlag.GetName())
	assert.Equal(t, "i", identityFlag.GetShorthand())
	assert.Equal(t, cfg.IdentityFlagSelectValue, identityFlag.GetNoOptDefVal())
	assert.Equal(t, []string{"ATMOS_IDENTITY", "IDENTITY"}, identityFlag.GetEnvVars())

	// Check dry-run flag
	dryRunFlag := registry.Get("dry-run")
	require.NotNil(t, dryRunFlag)
	assert.Equal(t, "dry-run", dryRunFlag.GetName())
	assert.Equal(t, []string{"ATMOS_DRY_RUN"}, dryRunFlag.GetEnvVars())
}

func TestTerraformFlags(t *testing.T) {
	registry := TerraformFlags()

	// Should have common flags + Terraform-specific flags
	assert.GreaterOrEqual(t, registry.Count(), 3)

	// Should include common flags
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("identity"))
	assert.True(t, registry.Has("dry-run"))

	// Should include Terraform-specific flags
	assert.True(t, registry.Has("upload-status"))
	assert.True(t, registry.Has("skip-init"))
	assert.True(t, registry.Has("from-plan"))

	// Check upload-status flag
	uploadFlag := registry.Get("upload-status")
	require.NotNil(t, uploadFlag)
	boolFlag, ok := uploadFlag.(*BoolFlag)
	require.True(t, ok)
	assert.Equal(t, false, boolFlag.Default)
}

func TestHelmfileFlags(t *testing.T) {
	registry := HelmfileFlags()

	// Should have common flags at minimum
	assert.GreaterOrEqual(t, registry.Count(), 3)
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("identity"))
	assert.True(t, registry.Has("dry-run"))
}

func TestPackerFlags(t *testing.T) {
	registry := PackerFlags()

	// Should have common flags at minimum
	assert.GreaterOrEqual(t, registry.Count(), 3)
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("identity"))
	assert.True(t, registry.Has("dry-run"))
}

func TestFlagRegistry_Validate(t *testing.T) {
	tests := []struct {
		name        string
		flags       []Flag
		values      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "all required flags present",
			flags: []Flag{
				&StringFlag{Name: "required", Required: true},
				&StringFlag{Name: "optional", Required: false},
			},
			values: map[string]interface{}{
				"required": "value",
				"optional": "value",
			},
			expectError: false,
		},
		{
			name: "missing required flag",
			flags: []Flag{
				&StringFlag{Name: "required", Required: true},
			},
			values:      map[string]interface{}{},
			expectError: true,
			errorMsg:    "required flag not provided: --required",
		},
		{
			name: "empty required string flag",
			flags: []Flag{
				&StringFlag{Name: "required", Required: true},
			},
			values: map[string]interface{}{
				"required": "",
			},
			expectError: true,
			errorMsg:    "required flag cannot be empty: --required",
		},
		{
			name: "optional flags can be missing",
			flags: []Flag{
				&StringFlag{Name: "optional", Required: false},
			},
			values:      map[string]interface{}{},
			expectError: false,
		},
		{
			name: "bool flags are never required",
			flags: []Flag{
				&BoolFlag{Name: "verbose"},
			},
			values:      map[string]interface{}{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewFlagRegistry()
			for _, flag := range tt.flags {
				registry.Register(flag)
			}

			err := registry.Validate(tt.values)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
