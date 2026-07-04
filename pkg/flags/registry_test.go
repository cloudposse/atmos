package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestFlagRegistry_RegisterPanicsOnDuplicate(t *testing.T) {
	registry := NewFlagRegistry()

	flag1 := &StringFlag{Name: "test", Default: "first"}
	registry.Register(flag1)

	// Attempting to register the same flag name twice should panic.
	flag2 := &StringFlag{Name: "test", Default: "second"}
	assert.Panics(t, func() {
		registry.Register(flag2)
	})

	// Should still have only one flag (the first one)
	assert.Equal(t, 1, registry.Count())

	retrieved := registry.Get("test")
	stringFlag := retrieved.(*StringFlag)
	assert.Equal(t, "first", stringFlag.Default)
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

	// Should have 2 flags: stack + dry-run
	// Global flags are inherited from RootCmd, not included in CommonFlags
	assert.Equal(t, 2, registry.Count(), "CommonFlags should only include stack + dry-run")

	// Check stack flag (added by CommonFlags)
	stackFlag := registry.Get("stack")
	require.NotNil(t, stackFlag)
	assert.Equal(t, "stack", stackFlag.GetName())
	assert.Equal(t, "s", stackFlag.GetShorthand())
	assert.Equal(t, []string{"ATMOS_STACK"}, stackFlag.GetEnvVars())

	// Check dry-run flag
	dryRunFlag := registry.Get("dry-run")
	require.NotNil(t, dryRunFlag)
	assert.Equal(t, "dry-run", dryRunFlag.GetName())
	assert.Equal(t, []string{"ATMOS_DRY_RUN"}, dryRunFlag.GetEnvVars())

	// Verify identity flag is NOT in CommonFlags (it's inherited from RootCmd)
	identityFlag := registry.Get(cfg.IdentityFlagName)
	assert.Nil(t, identityFlag, "identity should be inherited from RootCmd, not in CommonFlags")
}

func TestHelmfileFlags(t *testing.T) {
	registry := HelmfileFlags()

	// Should have common flags (stack, dry-run)
	// Global flags are inherited from RootCmd, not in registry
	assert.Equal(t, 2, registry.Count())
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("dry-run"))

	// Should NOT include global flags
	assert.False(t, registry.Has("identity"), "identity should be inherited from RootCmd")
}

func TestPackerFlags(t *testing.T) {
	registry := PackerFlags()

	// Should have common flags (stack, dry-run)
	// Global flags are inherited from RootCmd, not in registry
	assert.Equal(t, 2, registry.Count())
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("dry-run"))

	// Should NOT include global flags
	assert.False(t, registry.Has("identity"), "identity should be inherited from RootCmd")
}

func TestFlagRegistry_RegisterStringFlag(t *testing.T) {
	registry := NewFlagRegistry()
	registry.RegisterStringFlag("my-flag", "m", "default-val", "My flag description", false)

	flag := registry.Get("my-flag")
	require.NotNil(t, flag)

	sf, ok := flag.(*StringFlag)
	require.True(t, ok)
	assert.Equal(t, "my-flag", sf.Name)
	assert.Equal(t, "m", sf.Shorthand)
	assert.Equal(t, "default-val", sf.Default)
	assert.Equal(t, "My flag description", sf.Description)
	assert.False(t, sf.Required)
}

func TestFlagRegistry_RegisterStringFlag_Required(t *testing.T) {
	registry := NewFlagRegistry()
	registry.RegisterStringFlag("required-flag", "", "", "A required flag", true)

	flag := registry.Get("required-flag")
	require.NotNil(t, flag)
	assert.True(t, flag.IsRequired())
}

func TestFlagRegistry_RegisterBoolFlag(t *testing.T) {
	registry := NewFlagRegistry()
	registry.RegisterBoolFlag("verbose", "v", true, "Enable verbose output")

	flag := registry.Get("verbose")
	require.NotNil(t, flag)

	bf, ok := flag.(*BoolFlag)
	require.True(t, ok)
	assert.Equal(t, "verbose", bf.Name)
	assert.Equal(t, "v", bf.Shorthand)
	assert.True(t, bf.Default)
	assert.Equal(t, "Enable verbose output", bf.Description)
}

func TestFlagRegistry_RegisterIntFlag(t *testing.T) {
	registry := NewFlagRegistry()
	registry.RegisterIntFlag("count", "c", 5, "Number of items", false)

	flag := registry.Get("count")
	require.NotNil(t, flag)

	intf, ok := flag.(*IntFlag)
	require.True(t, ok)
	assert.Equal(t, "count", intf.Name)
	assert.Equal(t, "c", intf.Shorthand)
	assert.Equal(t, 5, intf.Default)
	assert.Equal(t, "Number of items", intf.Description)
	assert.False(t, intf.Required)
}

func TestFlagRegistry_RegisterFlags(t *testing.T) {
	registry := NewFlagRegistry()
	registry.Register(&StringFlag{Name: "stack", Shorthand: "s", Description: "Stack name"})
	registry.Register(&BoolFlag{Name: "dry-run", Description: "Dry run mode"})
	registry.Register(&IntFlag{Name: "timeout", Description: "Timeout seconds"})

	cmd := &cobra.Command{Use: "test"}
	registry.RegisterFlags(cmd)

	// Verify all flags were registered with the cobra command.
	assert.NotNil(t, cmd.Flags().Lookup("stack"), "stack flag should be registered")
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"), "dry-run flag should be registered")
	assert.NotNil(t, cmd.Flags().Lookup("timeout"), "timeout flag should be registered")

	// Verify shorthands.
	assert.Equal(t, "s", cmd.Flags().Lookup("stack").Shorthand)
}

func TestFlagRegistry_RegisterFlags_WithNoOptDefVal(t *testing.T) {
	registry := NewFlagRegistry()
	registry.Register(&StringFlag{
		Name:        "identity",
		NoOptDefVal: cfg.IdentityFlagSelectValue,
		Description: "Identity selector",
	})

	cmd := &cobra.Command{Use: "test"}
	registry.RegisterFlags(cmd)

	flag := cmd.Flags().Lookup("identity")
	require.NotNil(t, flag)
	assert.Equal(t, cfg.IdentityFlagSelectValue, flag.NoOptDefVal)
}

func TestFlagRegistry_RegisterPersistentFlags(t *testing.T) {
	registry := NewFlagRegistry()
	registry.Register(&StringFlag{Name: "logs-level", Description: "Log level"})
	registry.Register(&BoolFlag{Name: "verbose", Description: "Verbose output"})

	cmd := &cobra.Command{Use: "test"}
	registry.RegisterPersistentFlags(cmd)

	// Persistent flags should appear in PersistentFlags(), not Flags().
	assert.NotNil(t, cmd.PersistentFlags().Lookup("logs-level"), "logs-level should be registered as persistent")
	assert.NotNil(t, cmd.PersistentFlags().Lookup("verbose"), "verbose should be registered as persistent")

	// PersistentFlags are NOT in non-persistent Flags().
	assert.Nil(t, cmd.Flags().Lookup("logs-level"), "logs-level should NOT be in non-persistent flags")
}

func TestFlagRegistry_SetCompletionFunc(t *testing.T) {
	registry := NewFlagRegistry()
	registry.Register(&StringFlag{Name: "stack", Description: "Stack name"})

	completionFn := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dev", "prod", "staging"}, cobra.ShellCompDirectiveNoFileComp
	}

	// Should not panic.
	assert.NotPanics(t, func() {
		registry.SetCompletionFunc("stack", completionFn)
	})

	// Verify the function was set.
	flag := registry.Get("stack")
	sf, ok := flag.(*StringFlag)
	require.True(t, ok)
	require.NotNil(t, sf.CompletionFunc)

	// Verify the completion function works.
	results, _ := sf.CompletionFunc(nil, nil, "")
	assert.Equal(t, []string{"dev", "prod", "staging"}, results)
}

func TestFlagRegistry_SetCompletionFunc_NonExistentFlag(t *testing.T) {
	registry := NewFlagRegistry()

	completionFn := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Should not panic even when flag doesn't exist.
	assert.NotPanics(t, func() {
		registry.SetCompletionFunc("nonexistent", completionFn)
	})
}

func TestFlagRegistry_SetCompletionFunc_BoolFlagIgnored(t *testing.T) {
	registry := NewFlagRegistry()
	registry.Register(&BoolFlag{Name: "verbose", Description: "Verbose output"})

	completionFn := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"true", "false"}, cobra.ShellCompDirectiveNoFileComp
	}

	// Should not panic, but SetCompletionFunc only works for StringFlags.
	assert.NotPanics(t, func() {
		registry.SetCompletionFunc("verbose", completionFn)
	})

	// BoolFlag doesn't support CompletionFunc - setting it on a non-StringFlag is a no-op.
	// The flag should still be registered and functional.
	flag := registry.Get("verbose")
	assert.NotNil(t, flag)
	assert.Equal(t, "verbose", flag.GetName())

	// Verify the flag type was not changed - it must still be a *BoolFlag.
	// BoolFlag has no CompletionFunc field, confirming the call was definitively a no-op.
	_, ok := flag.(*BoolFlag)
	assert.True(t, ok, "flag should still be *BoolFlag after calling SetCompletionFunc on it")
}

func TestFlagRegistry_BindToViper(t *testing.T) {
	registry := NewFlagRegistry()
	registry.Register(&StringFlag{
		Name:    "stack",
		EnvVars: []string{"ATMOS_STACK"},
	})
	registry.Register(&BoolFlag{
		Name:    "dry-run",
		EnvVars: []string{"ATMOS_DRY_RUN"},
	})

	v := viper.New()
	err := registry.BindToViper(v)
	require.NoError(t, err)

	// BindToViper calls viper.BindEnv (not AutomaticEnv) to map flag names to env vars.
	// This is intentional: BindEnv binds a specific env var to a specific key without
	// enabling global env-var lookup. If the implementation changes to rely on AutomaticEnv
	// instead, this test would silently pass while production behavior breaks for keys
	// that don't follow the ATMOS_ prefix convention.
	t.Setenv("ATMOS_STACK", "test-stack")
	got := v.GetString("stack")
	assert.Equal(t, "test-stack", got, "viper should read ATMOS_STACK via bound flag name 'stack'")
}

func TestFlagRegistry_BindToViper_NoEnvVars(t *testing.T) {
	registry := NewFlagRegistry()
	registry.Register(&StringFlag{Name: "no-env-flag", Description: "Flag without env vars"})

	v := viper.New()
	err := registry.BindToViper(v)
	// Flags without env vars should not cause errors.
	assert.NoError(t, err)
}

func TestFlagRegistry_Validate(t *testing.T) {
	tests := []struct {
		name             string
		flags            []Flag
		values           map[string]interface{}
		expectError      bool
		expectedSentinel error
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
			values:           map[string]interface{}{},
			expectError:      true,
			expectedSentinel: errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name: "empty required string flag",
			flags: []Flag{
				&StringFlag{Name: "required", Required: true},
			},
			values: map[string]interface{}{
				"required": "",
			},
			expectError:      true,
			expectedSentinel: errUtils.ErrRequiredFlagEmpty,
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
				require.Error(t, err)
				if tt.expectedSentinel != nil {
					assert.ErrorIs(t, err, tt.expectedSentinel)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
