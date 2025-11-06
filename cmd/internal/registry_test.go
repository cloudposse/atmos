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
)

// mockCommandProvider is a test implementation of CommandProvider.
type mockCommandProvider struct {
	name  string
	group string
	cmd   *cobra.Command
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

// GetFlagsBuilder returns nil for test mock.
func (m *mockCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns nil for test mock.
func (m *mockCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityAliases returns nil for test mock.
func (m *mockCommandProvider) GetCompatibilityAliases() map[string]flags.CompatibilityAlias {
	return nil
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
