package internal

import (
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// mockCommandProvider is a test implementation of CommandProvider.
type mockCommandProvider struct {
	name    string
	group   string
	cmd     *cobra.Command
	aliases []CommandAlias
}

func (m *mockCommandProvider) GetCommand() *cobra.Command {
	return m.cmd
}

func (m *mockCommandProvider) GetName() string {
	return m.name
}

func (m *mockCommandProvider) GetGroup() string {
	return m.group
}

func (m *mockCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (m *mockCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (m *mockCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

func (m *mockCommandProvider) GetAliases() []CommandAlias {
	return m.aliases
}

func TestRegister(t *testing.T) {
	Reset() // Clear registry for clean test

	provider := &mockCommandProvider{
		name:  "test",
		group: "Test Commands",
		cmd:   &cobra.Command{Use: "test"},
	}

	Register(provider)

	assert.Equal(t, 1, Count())

	retrieved, ok := GetProvider("test")
	assert.True(t, ok)
	assert.Equal(t, provider, retrieved)
}

func TestRegisterMultiple(t *testing.T) {
	Reset()

	provider1 := &mockCommandProvider{
		name:  "test1",
		group: "Group A",
		cmd:   &cobra.Command{Use: "test1"},
	}

	provider2 := &mockCommandProvider{
		name:  "test2",
		group: "Group B",
		cmd:   &cobra.Command{Use: "test2"},
	}

	Register(provider1)
	Register(provider2)

	assert.Equal(t, 2, Count())

	retrieved1, ok1 := GetProvider("test1")
	assert.True(t, ok1)
	assert.Equal(t, provider1, retrieved1)

	retrieved2, ok2 := GetProvider("test2")
	assert.True(t, ok2)
	assert.Equal(t, provider2, retrieved2)
}

func TestRegisterOverride(t *testing.T) {
	Reset()

	provider1 := &mockCommandProvider{
		name:  "test",
		group: "Test",
		cmd:   &cobra.Command{Use: "test", Short: "First"},
	}

	provider2 := &mockCommandProvider{
		name:  "test",
		group: "Test",
		cmd:   &cobra.Command{Use: "test", Short: "Second"},
	}

	Register(provider1)
	Register(provider2)

	// Should only have one provider (override)
	assert.Equal(t, 1, Count())

	// Should retrieve the second provider
	retrieved, ok := GetProvider("test")
	assert.True(t, ok)
	assert.Equal(t, "Second", retrieved.GetCommand().Short)
}

func TestRegisterAll(t *testing.T) {
	Reset()

	rootCmd := &cobra.Command{Use: "root"}

	provider1 := &mockCommandProvider{
		name:  "test1",
		group: "Test",
		cmd:   &cobra.Command{Use: "test1"},
	}

	provider2 := &mockCommandProvider{
		name:  "test2",
		group: "Test",
		cmd:   &cobra.Command{Use: "test2"},
	}

	Register(provider1)
	Register(provider2)

	err := RegisterAll(rootCmd)
	require.NoError(t, err)

	// Root command should have both subcommands
	assert.True(t, rootCmd.HasSubCommands())
	assert.Len(t, rootCmd.Commands(), 2)

	// Verify commands are accessible
	cmd1, _, err1 := rootCmd.Find([]string{"test1"})
	assert.NoError(t, err1)
	assert.Equal(t, "test1", cmd1.Use)

	cmd2, _, err2 := rootCmd.Find([]string{"test2"})
	assert.NoError(t, err2)
	assert.Equal(t, "test2", cmd2.Use)
}

func TestRegisterAllNilCommand(t *testing.T) {
	Reset()

	rootCmd := &cobra.Command{Use: "root"}

	provider := &mockCommandProvider{
		name:  "test",
		group: "Test",
		cmd:   nil, // Nil command should cause error
	}

	Register(provider)

	err := RegisterAll(rootCmd)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrCommandNil), "error should wrap ErrCommandNil")
	assert.Contains(t, err.Error(), "test", "error message should include provider name")
}

func TestGetProviderNotFound(t *testing.T) {
	Reset()

	provider, ok := GetProvider("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, provider)
}

func TestListProviders(t *testing.T) {
	Reset()

	provider1 := &mockCommandProvider{
		name:  "test1",
		group: "Group A",
		cmd:   &cobra.Command{Use: "test1"},
	}

	provider2 := &mockCommandProvider{
		name:  "test2",
		group: "Group A",
		cmd:   &cobra.Command{Use: "test2"},
	}

	provider3 := &mockCommandProvider{
		name:  "test3",
		group: "Group B",
		cmd:   &cobra.Command{Use: "test3"},
	}

	Register(provider1)
	Register(provider2)
	Register(provider3)

	grouped := ListProviders()

	assert.Len(t, grouped, 2)
	assert.Len(t, grouped["Group A"], 2)
	assert.Len(t, grouped["Group B"], 1)

	// Verify providers are in correct groups
	groupA := grouped["Group A"]
	assert.Contains(t, []CommandProvider{provider1, provider2}, groupA[0])
	assert.Contains(t, []CommandProvider{provider1, provider2}, groupA[1])

	groupB := grouped["Group B"]
	assert.Equal(t, provider3, groupB[0])
}

func TestNestedCommands(t *testing.T) {
	Reset()

	// Create parent command with subcommands
	parentCmd := &cobra.Command{Use: "parent"}
	childCmd1 := &cobra.Command{Use: "child1"}
	childCmd2 := &cobra.Command{Use: "child2"}

	parentCmd.AddCommand(childCmd1)
	parentCmd.AddCommand(childCmd2)

	provider := &mockCommandProvider{
		name:  "parent",
		group: "Test",
		cmd:   parentCmd,
	}

	Register(provider)

	// Verify parent is registered
	retrieved, ok := GetProvider("parent")
	assert.True(t, ok)
	assert.True(t, retrieved.GetCommand().HasSubCommands())

	// Verify children are accessible
	subCmd1, _, err1 := retrieved.GetCommand().Find([]string{"child1"})
	assert.NoError(t, err1)
	assert.Equal(t, "child1", subCmd1.Use)

	subCmd2, _, err2 := retrieved.GetCommand().Find([]string{"child2"})
	assert.NoError(t, err2)
	assert.Equal(t, "child2", subCmd2.Use)
}

func TestDeeplyNestedCommands(t *testing.T) {
	Reset()

	// Create deeply nested command hierarchy
	grandparentCmd := &cobra.Command{Use: "grandparent"}
	parentCmd := &cobra.Command{Use: "parent"}
	childCmd := &cobra.Command{Use: "child"}

	parentCmd.AddCommand(childCmd)
	grandparentCmd.AddCommand(parentCmd)

	provider := &mockCommandProvider{
		name:  "grandparent",
		group: "Test",
		cmd:   grandparentCmd,
	}

	Register(provider)

	// Verify grandparent is registered
	retrieved, ok := GetProvider("grandparent")
	assert.True(t, ok)

	// Verify deeply nested child is accessible
	subCmd, _, err := retrieved.GetCommand().Find([]string{"parent", "child"})
	assert.NoError(t, err)
	assert.Equal(t, "child", subCmd.Use)
}

func TestCount(t *testing.T) {
	Reset()

	assert.Equal(t, 0, Count())

	Register(&mockCommandProvider{name: "test1", group: "Test", cmd: &cobra.Command{Use: "test1"}})
	assert.Equal(t, 1, Count())

	Register(&mockCommandProvider{name: "test2", group: "Test", cmd: &cobra.Command{Use: "test2"}})
	assert.Equal(t, 2, Count())

	Register(&mockCommandProvider{name: "test1", group: "Test", cmd: &cobra.Command{Use: "test1"}})
	assert.Equal(t, 2, Count()) // Override doesn't increase count
}

func TestReset(t *testing.T) {
	Reset()

	Register(&mockCommandProvider{name: "test", group: "Test", cmd: &cobra.Command{Use: "test"}})
	assert.Equal(t, 1, Count())

	Reset()
	assert.Equal(t, 0, Count())

	_, ok := GetProvider("test")
	assert.False(t, ok)
}

func TestConcurrency(t *testing.T) {
	Reset()

	// Test concurrent registration
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			provider := &mockCommandProvider{
				name:  fmt.Sprintf("test%d", idx),
				group: "Test",
				cmd:   &cobra.Command{Use: fmt.Sprintf("test%d", idx)},
			}
			Register(provider)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Equal(t, 10, Count())
}

func TestCustomCommandCanExtendRegistryCommand(t *testing.T) {
	Reset()

	// Simulate a registry command (e.g., "terraform")
	registryCmd := &cobra.Command{
		Use:   "terraform",
		Short: "Built-in terraform command",
	}

	provider := &mockCommandProvider{
		name:  "terraform",
		group: "Core Stack Commands",
		cmd:   registryCmd,
	}

	Register(provider)

	// Register with root
	rootCmd := &cobra.Command{Use: "root"}
	err := RegisterAll(rootCmd)
	require.NoError(t, err)

	// Verify command is registered
	tfCmd, _, err := rootCmd.Find([]string{"terraform"})
	require.NoError(t, err)
	assert.Equal(t, "terraform", tfCmd.Use)
	assert.False(t, tfCmd.HasSubCommands(), "should have no subcommands initially")

	// Simulate custom command extending it by adding a subcommand
	// This is what processCustomCommands() would do in cmd/cmd_utils.go
	customSubCmd := &cobra.Command{
		Use:   "custom-plan",
		Short: "Custom terraform plan with extra validation",
	}
	tfCmd.AddCommand(customSubCmd)

	// Verify the registry command now has the custom subcommand
	assert.True(t, tfCmd.HasSubCommands(), "should have subcommands after extension")
	customCmd, _, err := tfCmd.Find([]string{"custom-plan"})
	require.NoError(t, err)
	assert.Equal(t, "custom-plan", customCmd.Use)
}

// resetCompatFlagsRegistry clears the compat flags registry for testing.
// This ensures tests start with a clean state.
func resetCompatFlagsRegistry() {
	commandCompatFlagsRegistry.mu.Lock()
	defer commandCompatFlagsRegistry.mu.Unlock()
	commandCompatFlagsRegistry.flags = make(map[string]map[string]map[string]compat.CompatibilityFlag)
}

func TestRegisterCommandCompatFlags(t *testing.T) {
	resetCompatFlagsRegistry()

	// Register terraform global flags.
	RegisterCommandCompatFlags("terraform", "terraform", map[string]compat.CompatibilityFlag{
		"-chdir":   {Behavior: compat.AppendToSeparated, Description: "Switch to a different working directory"},
		"-help":    {Behavior: compat.AppendToSeparated, Description: "Show terraform help output"},
		"-version": {Behavior: compat.AppendToSeparated, Description: "Show terraform version"},
	})

	// Verify registration.
	flags := GetSubcommandCompatFlags("terraform", "terraform")
	require.NotNil(t, flags)
	assert.Len(t, flags, 3)
	assert.Contains(t, flags, "-chdir")
	assert.Contains(t, flags, "-help")
	assert.Contains(t, flags, "-version")
}

func TestRegisterCommandCompatFlags_MultipleSubcommands(t *testing.T) {
	resetCompatFlagsRegistry()

	// Register terraform global flags.
	RegisterCommandCompatFlags("terraform", "terraform", map[string]compat.CompatibilityFlag{
		"-chdir":   {Behavior: compat.AppendToSeparated, Description: "Switch to a different working directory"},
		"-help":    {Behavior: compat.AppendToSeparated, Description: "Show terraform help output"},
		"-version": {Behavior: compat.AppendToSeparated, Description: "Show terraform version"},
	})

	// Register plan-specific flags.
	RegisterCommandCompatFlags("terraform", "plan", map[string]compat.CompatibilityFlag{
		"-var":               {Behavior: compat.AppendToSeparated, Description: "Set a value for one of the input variables"},
		"-var-file":          {Behavior: compat.AppendToSeparated, Description: "Load variable values from the given file"},
		"-out":               {Behavior: compat.AppendToSeparated, Description: "Write the plan to the given path"},
		"-destroy":           {Behavior: compat.AppendToSeparated, Description: "Create a plan to destroy all objects"},
		"-detailed-exitcode": {Behavior: compat.AppendToSeparated, Description: "Return detailed exit codes"},
	})

	// Register apply-specific flags.
	RegisterCommandCompatFlags("terraform", "apply", map[string]compat.CompatibilityFlag{
		"-var":          {Behavior: compat.AppendToSeparated, Description: "Set a value for one of the input variables"},
		"-var-file":     {Behavior: compat.AppendToSeparated, Description: "Load variable values from the given file"},
		"-auto-approve": {Behavior: compat.AppendToSeparated, Description: "Skip interactive approval"},
		"-backup":       {Behavior: compat.AppendToSeparated, Description: "Path to backup the existing state file"},
	})

	// Verify terraform global flags.
	globalFlags := GetSubcommandCompatFlags("terraform", "terraform")
	require.NotNil(t, globalFlags)
	assert.Len(t, globalFlags, 3)
	assert.Contains(t, globalFlags, "-chdir")
	assert.Contains(t, globalFlags, "-help")
	assert.Contains(t, globalFlags, "-version")
	assert.NotContains(t, globalFlags, "-var") // Should not have subcommand flags.

	// Verify plan flags.
	planFlags := GetSubcommandCompatFlags("terraform", "plan")
	require.NotNil(t, planFlags)
	assert.Len(t, planFlags, 5)
	assert.Contains(t, planFlags, "-var")
	assert.Contains(t, planFlags, "-var-file")
	assert.Contains(t, planFlags, "-out")
	assert.Contains(t, planFlags, "-destroy")
	assert.Contains(t, planFlags, "-detailed-exitcode")
	assert.NotContains(t, planFlags, "-chdir")        // Should not have global flags.
	assert.NotContains(t, planFlags, "-auto-approve") // Should not have apply flags.

	// Verify apply flags.
	applyFlags := GetSubcommandCompatFlags("terraform", "apply")
	require.NotNil(t, applyFlags)
	assert.Len(t, applyFlags, 4)
	assert.Contains(t, applyFlags, "-var")
	assert.Contains(t, applyFlags, "-var-file")
	assert.Contains(t, applyFlags, "-auto-approve")
	assert.Contains(t, applyFlags, "-backup")
	assert.NotContains(t, applyFlags, "-chdir") // Should not have global flags.
	assert.NotContains(t, applyFlags, "-out")   // Should not have plan flags.
}

func TestGetSubcommandCompatFlags_NotFound(t *testing.T) {
	resetCompatFlagsRegistry()

	// Query non-existent provider.
	flags := GetSubcommandCompatFlags("nonexistent", "plan")
	assert.Nil(t, flags)

	// Query non-existent subcommand.
	RegisterCommandCompatFlags("terraform", "plan", map[string]compat.CompatibilityFlag{
		"-var": {Behavior: compat.AppendToSeparated, Description: "Set a variable"},
	})

	flags = GetSubcommandCompatFlags("terraform", "nonexistent")
	assert.Nil(t, flags)
}

func TestRegisterCommandCompatFlags_OverwriteExisting(t *testing.T) {
	resetCompatFlagsRegistry()

	// Register initial flags.
	RegisterCommandCompatFlags("terraform", "plan", map[string]compat.CompatibilityFlag{
		"-var": {Behavior: compat.AppendToSeparated, Description: "Original description"},
	})

	// Overwrite with new flags.
	RegisterCommandCompatFlags("terraform", "plan", map[string]compat.CompatibilityFlag{
		"-var":      {Behavior: compat.AppendToSeparated, Description: "Updated description"},
		"-var-file": {Behavior: compat.AppendToSeparated, Description: "New flag"},
	})

	// Verify flags were overwritten.
	flags := GetSubcommandCompatFlags("terraform", "plan")
	require.NotNil(t, flags)
	assert.Len(t, flags, 2)
	assert.Equal(t, "Updated description", flags["-var"].Description)
	assert.Contains(t, flags, "-var-file")
}

func TestRegisterCommandCompatFlags_MultipleProviders(t *testing.T) {
	resetCompatFlagsRegistry()

	// Register terraform flags.
	RegisterCommandCompatFlags("terraform", "plan", map[string]compat.CompatibilityFlag{
		"-var": {Behavior: compat.AppendToSeparated, Description: "Terraform variable"},
	})

	// Register helmfile flags.
	RegisterCommandCompatFlags("helmfile", "sync", map[string]compat.CompatibilityFlag{
		"-f": {Behavior: compat.AppendToSeparated, Description: "Path to helmfile.yaml"},
	})

	// Verify terraform flags.
	tfFlags := GetSubcommandCompatFlags("terraform", "plan")
	require.NotNil(t, tfFlags)
	assert.Contains(t, tfFlags, "-var")
	assert.NotContains(t, tfFlags, "-f")

	// Verify helmfile flags.
	hfFlags := GetSubcommandCompatFlags("helmfile", "sync")
	require.NotNil(t, hfFlags)
	assert.Contains(t, hfFlags, "-f")
	assert.NotContains(t, hfFlags, "-var")

	// Verify providers are isolated.
	assert.Nil(t, GetSubcommandCompatFlags("terraform", "sync"))
	assert.Nil(t, GetSubcommandCompatFlags("helmfile", "plan"))
}

func TestRegisterCommandCompatFlags_Concurrent(t *testing.T) {
	resetCompatFlagsRegistry()

	done := make(chan bool)

	// Concurrently register flags for different subcommands.
	for i := 0; i < 10; i++ {
		go func(idx int) {
			subcommand := fmt.Sprintf("cmd%d", idx)
			RegisterCommandCompatFlags("terraform", subcommand, map[string]compat.CompatibilityFlag{
				"-var": {Behavior: compat.AppendToSeparated, Description: fmt.Sprintf("Flag for %s", subcommand)},
			})
			done <- true
		}(i)
	}

	// Wait for all goroutines.
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all registrations succeeded.
	for i := 0; i < 10; i++ {
		subcommand := fmt.Sprintf("cmd%d", i)
		flags := GetSubcommandCompatFlags("terraform", subcommand)
		require.NotNil(t, flags, "flags for %s should exist", subcommand)
		assert.Contains(t, flags, "-var")
	}
}
