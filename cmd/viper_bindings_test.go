package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestViperBindPFlagPollutesGlobalState tests if viper.BindPFlag causes
// the flag's NoOptDefVal to pollute viper's global state when flag is used without value.
//
// This reproduces the ACTUAL bug: auth shell uses BindPFlag, which causes
// viper to persist the __SELECT__ value globally, affecting subsequent commands.
func TestViperBindPFlagPollutesGlobalState(t *testing.T) {
	_ = NewTestKit(t)

	// Simulate two commands running in sequence (like in CI)
	// Command 1: atmos auth shell --identity
	// Command 2: atmos terraform plan (no --identity flag)

	t.Run("REPRODUCTION: auth shell with BindPFlag pollutes viper for terraform", func(t *testing.T) {
		viper.Reset()

		// ===== COMMAND 1: atmos auth shell --identity =====
		authShellCmd := &cobra.Command{Use: "shell"}

		// Parent command (authCmd) defines persistent flag with NoOptDefVal
		parentCmd := &cobra.Command{Use: "auth"}
		parentCmd.PersistentFlags().StringP("identity", "i", "", "Identity flag")
		identityFlag := parentCmd.PersistentFlags().Lookup("identity")
		require.NotNil(t, identityFlag)
		identityFlag.NoOptDefVal = "__SELECT__"

		// Child command uses BindPFlag (like auth_shell.go:241)
		parentCmd.AddCommand(authShellCmd)
		err := viper.BindPFlag("identity", parentCmd.PersistentFlags().Lookup("identity"))
		require.NoError(t, err)

		// User runs: atmos auth shell --identity
		err = parentCmd.ParseFlags([]string{"shell", "--identity"})
		require.NoError(t, err)

		// Check: Flag should have __SELECT__ due to NoOptDefVal
		flagValue, err := parentCmd.PersistentFlags().GetString("identity")
		require.NoError(t, err)
		assert.Equal(t, "__SELECT__", flagValue, "Flag should be __SELECT__")

		// Check: Viper should ALSO have __SELECT__ due to BindPFlag
		viperValue := viper.GetString("identity")
		assert.Equal(t, "__SELECT__", viperValue, "🔥 BUG: Viper got polluted with __SELECT__")

		t.Logf("After 'atmos auth shell --identity':")
		t.Logf("  flag value:  %q", flagValue)
		t.Logf("  viper value: %q", viperValue)

		// ===== COMMAND 2: atmos terraform plan =====
		// Simulate a NEW command that doesn't provide --identity flag
		terraformCmd := &cobra.Command{Use: "plan"}
		terraformCmd.Flags().StringP("identity", "i", "", "Identity flag")
		terraformIdentityFlag := terraformCmd.Flags().Lookup("identity")
		require.NotNil(t, terraformIdentityFlag)
		terraformIdentityFlag.NoOptDefVal = "__SELECT__"

		// Parse with NO --identity flag
		err = terraformCmd.ParseFlags([]string{})
		require.NoError(t, err)

		// Check: Flag should be empty (not provided)
		terraformFlagValue, err := terraformCmd.Flags().GetString("identity")
		require.NoError(t, err)
		assert.Empty(t, terraformFlagValue, "Terraform flag should be empty")

		// Check: flags.Changed should be false
		assert.False(t, terraformCmd.Flags().Changed("identity"), "flags.Changed should be false")

		// 🔥 THE BUG: Viper still has __SELECT__ from previous command!
		viperValueAfter := viper.GetString("identity")
		assert.Equal(t, "__SELECT__", viperValueAfter, "🔥 PROVEN: Viper pollution persists across commands!")

		t.Logf("\nAfter 'atmos terraform plan' (no flag):")
		t.Logf("  flag value:     %q", terraformFlagValue)
		t.Logf("  flags.Changed:  %v", terraformCmd.Flags().Changed("identity"))
		t.Logf("  viper value:    %q (🔥 STILL POLLUTED!)", viperValueAfter)

		t.Log("\n✗ BUG CONFIRMED:")
		t.Log("  1. User runs: atmos auth shell --identity")
		t.Log("  2. BindPFlag in auth_shell.go:241 sets viper to __SELECT__")
		t.Log("  3. User runs: atmos terraform plan")
		t.Log("  4. Original buggy code reads viper.GetString('identity') → __SELECT__")
		t.Log("  5. Triggers TTY check in CI → FAILS!")
	})
}

// TestViperBindPFlagVsBindEnv compares BindPFlag (problematic) vs BindEnv (safe).
func TestViperBindPFlagVsBindEnv(t *testing.T) {
	_ = NewTestKit(t)

	t.Run("BindPFlag creates two-way binding - POLLUTES viper", func(t *testing.T) {
		viper.Reset()

		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().StringP("identity", "i", "", "test")
		flag := cmd.Flags().Lookup("identity")
		flag.NoOptDefVal = "__SELECT__"

		// BindPFlag creates TWO-WAY binding
		err := viper.BindPFlag("identity", flag)
		require.NoError(t, err)

		// Parse with --identity (no value)
		err = cmd.ParseFlags([]string{"--identity"})
		require.NoError(t, err)

		flagValue, _ := cmd.Flags().GetString("identity")
		viperValue := viper.GetString("identity")

		assert.Equal(t, "__SELECT__", flagValue)
		assert.Equal(t, "__SELECT__", viperValue, "BindPFlag syncs flag → viper")
	})

	t.Run("BindEnv is one-way - SAFE", func(t *testing.T) {
		viper.Reset()

		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().StringP("identity", "i", "", "test")
		flag := cmd.Flags().Lookup("identity")
		flag.NoOptDefVal = "__SELECT__"

		// BindEnv ONLY binds env vars, not flag values
		err := viper.BindEnv("identity", "ATMOS_IDENTITY")
		require.NoError(t, err)

		// Parse with --identity (no value)
		err = cmd.ParseFlags([]string{"--identity"})
		require.NoError(t, err)

		flagValue, _ := cmd.Flags().GetString("identity")
		viperValue := viper.GetString("identity")

		assert.Equal(t, "__SELECT__", flagValue)
		assert.Empty(t, viperValue, "BindEnv does NOT sync flag → viper (SAFE)")
	})
}
