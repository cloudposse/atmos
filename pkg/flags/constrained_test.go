package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func withNonInteractive(t *testing.T) {
	t.Helper()
	original := viper.GetBool("interactive")
	viper.Set("interactive", false)
	t.Cleanup(func() { viper.Set("interactive", original) })
}

func TestValidateConstrainedFields_NoConstraintsIsNoop(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	commandConfig := &schema.Command{
		Arguments: []schema.CommandArgument{{Name: "env"}},
		Flags:     []schema.CommandFlag{{Name: "stack"}},
	}
	args := map[string]string{"env": "dev"}
	flagsData := map[string]any{"stack": "plat-ue2-dev"}

	err := ValidateConstrainedFields(cmd, commandConfig, args, flagsData)
	require.NoError(t, err)
}

func TestValidateConstrainedFields_ValidArgumentValuePasses(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	commandConfig := &schema.Command{
		Arguments: []schema.CommandArgument{{Name: "env", Values: []string{"dev", "prod"}}},
	}
	args := map[string]string{"env": "prod"}

	err := ValidateConstrainedFields(cmd, commandConfig, args, map[string]any{})
	require.NoError(t, err)
}

func TestValidateConstrainedFields_InvalidArgumentValueErrors(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	commandConfig := &schema.Command{
		Arguments: []schema.CommandArgument{{Name: "env", Values: []string{"dev", "prod"}}},
	}
	args := map[string]string{"env": "typo"}

	err := ValidateConstrainedFields(cmd, commandConfig, args, map[string]any{})
	require.Error(t, err)
}

func TestValidateConstrainedFields_ValidFlagValuePasses(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	commandConfig := &schema.Command{
		Flags: []schema.CommandFlag{{Name: "environment", Values: []string{"dev", "staging", "prod"}}},
	}
	flagsData := map[string]any{"environment": "staging"}

	err := ValidateConstrainedFields(cmd, commandConfig, map[string]string{}, flagsData)
	require.NoError(t, err)
}

func TestValidateConstrainedFields_InvalidFlagValueErrors(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	commandConfig := &schema.Command{
		Flags: []schema.CommandFlag{{Name: "environment", Values: []string{"dev", "staging", "prod"}}},
	}
	flagsData := map[string]any{"environment": "typo"}

	err := ValidateConstrainedFields(cmd, commandConfig, map[string]string{}, flagsData)
	require.Error(t, err)
}

func TestValidateConstrainedFields_MissingOptionalArgumentSkipsSilently(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	commandConfig := &schema.Command{
		Arguments: []schema.CommandArgument{{Name: "env", Values: []string{"dev", "prod"}, Required: false}},
	}
	err := ValidateConstrainedFields(cmd, commandConfig, map[string]string{}, map[string]any{})
	require.NoError(t, err)
}

func TestValidateConstrainedFields_MissingOptionalFlagSkipsSilently(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	commandConfig := &schema.Command{
		Flags: []schema.CommandFlag{{Name: "environment", Values: []string{"dev", "prod"}, Required: false}},
	}
	err := ValidateConstrainedFields(cmd, commandConfig, map[string]string{}, map[string]any{})
	require.NoError(t, err)
}

func TestValidateConstrainedFields_MissingRequiredNonInteractiveSkipsWithoutPrompting(t *testing.T) {
	withNonInteractive(t)
	cmd := &cobra.Command{Use: "test"}
	commandConfig := &schema.Command{
		Arguments: []schema.CommandArgument{{Name: "env", Values: []string{"dev", "prod"}, Required: true}},
		Flags:     []schema.CommandFlag{{Name: "stack", Values: []string{"a", "b"}, Required: true}},
	}
	err := ValidateConstrainedFields(cmd, commandConfig, map[string]string{}, map[string]any{})
	require.NoError(t, err, "a required-but-missing constrained field must not error outside an interactive terminal")
}

func TestValidateConstrainedFields_FlagValueOfWrongTypeIsTreatedAsMissing(t *testing.T) {
	withNonInteractive(t)
	cmd := &cobra.Command{Use: "test"}
	commandConfig := &schema.Command{
		Flags: []schema.CommandFlag{{Name: "count", Values: []string{"1", "2"}}},
	}
	// A non-string value in flagsData (e.g. an int flag) must not panic the type assertion.
	flagsData := map[string]any{"count": 42}
	err := ValidateConstrainedFields(cmd, commandConfig, map[string]string{}, flagsData)
	require.NoError(t, err)
}

func TestValidateConstrainedFields_ArgumentsAndFlagsBothChecked(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	commandConfig := &schema.Command{
		Arguments: []schema.CommandArgument{{Name: "env", Values: []string{"dev", "prod"}}},
		Flags:     []schema.CommandFlag{{Name: "stack", Values: []string{"a", "b"}}},
	}
	args := map[string]string{"env": "dev"}
	flagsData := map[string]any{"stack": "invalid"}

	err := ValidateConstrainedFields(cmd, commandConfig, args, flagsData)
	require.Error(t, err, "an invalid flag value must fail even when the argument is valid")
	assert.Contains(t, err.Error(), "stack")
}
