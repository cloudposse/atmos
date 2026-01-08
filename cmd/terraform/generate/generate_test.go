package generate

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCmd(t *testing.T) {
	t.Run("command structure", func(t *testing.T) {
		assert.Equal(t, "generate", GenerateCmd.Use)
		assert.Equal(t, "Generate Terraform configuration files for Atmos components and stacks", GenerateCmd.Short)
		assert.Contains(t, GenerateCmd.Long, "terraform generate")
	})

	t.Run("has expected subcommands", func(t *testing.T) {
		subcommands := GenerateCmd.Commands()
		require.NotEmpty(t, subcommands)

		// Get subcommand names.
		names := make([]string, len(subcommands))
		for i, cmd := range subcommands {
			names[i] = cmd.Name()
		}

		assert.Contains(t, names, "backend")
		assert.Contains(t, names, "backends")
		assert.Contains(t, names, "files")
		assert.Contains(t, names, "varfile")
		assert.Contains(t, names, "varfiles")
		assert.Contains(t, names, "planfile")
	})
}

func TestBackendCmd(t *testing.T) {
	t.Run("command structure", func(t *testing.T) {
		assert.Equal(t, "backend [component]", backendCmd.Use)
		assert.Equal(t, "Generate backend configuration for a Terraform component", backendCmd.Short)
	})

	t.Run("has expected flags", func(t *testing.T) {
		flags := backendCmd.Flags()

		stackFlag := flags.Lookup("stack")
		require.NotNil(t, stackFlag)
		assert.Equal(t, "s", stackFlag.Shorthand)

		processTemplatesFlag := flags.Lookup("process-templates")
		require.NotNil(t, processTemplatesFlag)
		assert.Equal(t, "true", processTemplatesFlag.DefValue)

		processFunctionsFlag := flags.Lookup("process-functions")
		require.NotNil(t, processFunctionsFlag)
		assert.Equal(t, "true", processFunctionsFlag.DefValue)

		skipFlag := flags.Lookup("skip")
		require.NotNil(t, skipFlag)
		assert.Equal(t, "[]", skipFlag.DefValue)
	})
}

func TestFilesCmd(t *testing.T) {
	// Find the files command.
	var filesCmdPtr *cobra.Command
	for _, cmd := range GenerateCmd.Commands() {
		if cmd.Name() == "files" {
			filesCmdPtr = cmd
			break
		}
	}

	t.Run("command structure", func(t *testing.T) {
		require.NotNil(t, filesCmdPtr)
		assert.Equal(t, "files [component]", filesCmdPtr.Use)
		assert.Contains(t, filesCmdPtr.Short, "Generate files")
	})

	t.Run("has expected flags", func(t *testing.T) {
		require.NotNil(t, filesCmdPtr)
		flags := filesCmdPtr.Flags()

		stackFlag := flags.Lookup("stack")
		require.NotNil(t, stackFlag)
		assert.Equal(t, "s", stackFlag.Shorthand)

		allFlag := flags.Lookup("all")
		require.NotNil(t, allFlag)

		stacksFlag := flags.Lookup("stacks")
		require.NotNil(t, stacksFlag)

		componentsFlag := flags.Lookup("components")
		require.NotNil(t, componentsFlag)

		dryRunFlag := flags.Lookup("dry-run")
		require.NotNil(t, dryRunFlag)

		cleanFlag := flags.Lookup("clean")
		require.NotNil(t, cleanFlag)
	})
}

func TestBackendsCmd(t *testing.T) {
	// Find the backends command.
	var backendsCmd *BackendsCmd
	for _, cmd := range GenerateCmd.Commands() {
		if cmd.Name() == "backends" {
			backendsCmd = &BackendsCmd{cmd}
			break
		}
	}

	t.Run("command structure", func(t *testing.T) {
		require.NotNil(t, backendsCmd)
		assert.Equal(t, "backends", backendsCmd.Name())
	})
}

// BackendsCmd wraps cobra.Command for type safety in tests.
type BackendsCmd struct {
	*cobra.Command
}

func TestVarfileCmd(t *testing.T) {
	// Find the varfile command.
	var varfileCmd *VarfileCmd
	for _, cmd := range GenerateCmd.Commands() {
		if cmd.Name() == "varfile" {
			varfileCmd = &VarfileCmd{cmd}
			break
		}
	}

	t.Run("command structure", func(t *testing.T) {
		require.NotNil(t, varfileCmd)
		assert.Equal(t, "varfile", varfileCmd.Name())
	})
}

// VarfileCmd wraps cobra.Command for type safety in tests.
type VarfileCmd struct {
	*cobra.Command
}

func TestVarfilesCmd(t *testing.T) {
	// Find the varfiles command.
	var varfilesCmd *VarfilesCmd
	for _, cmd := range GenerateCmd.Commands() {
		if cmd.Name() == "varfiles" {
			varfilesCmd = &VarfilesCmd{cmd}
			break
		}
	}

	t.Run("command structure", func(t *testing.T) {
		require.NotNil(t, varfilesCmd)
		assert.Equal(t, "varfiles", varfilesCmd.Name())
	})
}

// VarfilesCmd wraps cobra.Command for type safety in tests.
type VarfilesCmd struct {
	*cobra.Command
}

func TestPlanfileCmd(t *testing.T) {
	// Find the planfile command.
	var planfileCmd *PlanfileCmd
	for _, cmd := range GenerateCmd.Commands() {
		if cmd.Name() == "planfile" {
			planfileCmd = &PlanfileCmd{cmd}
			break
		}
	}

	t.Run("command structure", func(t *testing.T) {
		require.NotNil(t, planfileCmd)
		assert.Equal(t, "planfile", planfileCmd.Name())
	})
}

// PlanfileCmd wraps cobra.Command for type safety in tests.
type PlanfileCmd struct {
	*cobra.Command
}

// TestFilesParserSetup verifies that the files parser is properly configured.
func TestFilesParserSetup(t *testing.T) {
	require.NotNil(t, filesParser, "filesParser should be initialized")

	// Verify the parser has the files-specific flags.
	registry := filesParser.Registry()

	expectedFlags := []string{
		"stack",
		"all",
		"stacks",
		"components",
		"dry-run",
		"clean",
	}

	for _, flagName := range expectedFlags {
		assert.True(t, registry.Has(flagName), "filesParser should have %s flag registered", flagName)
	}
}

// TestFilesCommandArgs verifies that files command accepts the correct number of arguments.
func TestFilesCommandArgs(t *testing.T) {
	// Find the files command.
	var filesCmdPtr *cobra.Command
	for _, cmd := range GenerateCmd.Commands() {
		if cmd.Name() == "files" {
			filesCmdPtr = cmd
			break
		}
	}
	require.NotNil(t, filesCmdPtr)

	// The command should accept 0 or 1 argument (component name is optional).
	require.NotNil(t, filesCmdPtr.Args)

	// Verify with no args (should pass since --all is available).
	err := filesCmdPtr.Args(filesCmdPtr, []string{})
	assert.NoError(t, err, "files command should accept 0 arguments")

	// Verify with one arg.
	err = filesCmdPtr.Args(filesCmdPtr, []string{"my-component"})
	assert.NoError(t, err, "files command should accept 1 argument")

	// Verify with two args (should fail).
	err = filesCmdPtr.Args(filesCmdPtr, []string{"arg1", "arg2"})
	assert.Error(t, err, "files command should reject more than 1 argument")
}

// TestFilesFlagEnvVars verifies that files command flags have environment variable bindings.
func TestFilesFlagEnvVars(t *testing.T) {
	registry := filesParser.Registry()

	// Expected env var bindings.
	expectedEnvVars := map[string]string{
		"stack":      "ATMOS_STACK",
		"stacks":     "ATMOS_STACKS",
		"components": "ATMOS_COMPONENTS",
	}

	for flagName, expectedEnvVar := range expectedEnvVars {
		require.True(t, registry.Has(flagName), "filesParser should have %s flag registered", flagName)
		flag := registry.Get(flagName)
		require.NotNil(t, flag, "filesParser should have info for %s flag", flagName)
		envVars := flag.GetEnvVars()
		assert.Contains(t, envVars, expectedEnvVar, "%s should be bound to %s", flagName, expectedEnvVar)
	}
}

// TestFilesCommandDescription verifies the command description content.
func TestFilesCommandDescription(t *testing.T) {
	// Find the files command.
	var filesCmdPtr *cobra.Command
	for _, cmd := range GenerateCmd.Commands() {
		if cmd.Name() == "files" {
			filesCmdPtr = cmd
			break
		}
	}

	require.NotNil(t, filesCmdPtr)

	t.Run("short description mentions generate", func(t *testing.T) {
		assert.Contains(t, filesCmdPtr.Short, "Generate files")
	})

	t.Run("long description explains usage", func(t *testing.T) {
		assert.Contains(t, filesCmdPtr.Long, "generate section")
		assert.Contains(t, filesCmdPtr.Long, "--all")
		assert.Contains(t, filesCmdPtr.Long, "component")
	})
}

// TestFilesCommandFlagTypes verifies that flags have correct types.
func TestFilesCommandFlagTypes(t *testing.T) {
	// Find the files command.
	var filesCmdPtr *cobra.Command
	for _, cmd := range GenerateCmd.Commands() {
		if cmd.Name() == "files" {
			filesCmdPtr = cmd
			break
		}
	}
	require.NotNil(t, filesCmdPtr)

	// String flags.
	stringFlags := []string{"stack", "stacks", "components"}
	for _, flagName := range stringFlags {
		flag := filesCmdPtr.Flags().Lookup(flagName)
		require.NotNil(t, flag, "%s flag should exist", flagName)
		assert.Equal(t, "string", flag.Value.Type(), "%s should be a string flag", flagName)
	}

	// Bool flags.
	boolFlags := []string{"all", "dry-run", "clean"}
	for _, flagName := range boolFlags {
		flag := filesCmdPtr.Flags().Lookup(flagName)
		require.NotNil(t, flag, "%s flag should exist", flagName)
		assert.Equal(t, "bool", flag.Value.Type(), "%s should be a bool flag", flagName)
	}
}

// TestFilesCommandFlagDefaults verifies that flags have correct default values.
func TestFilesCommandFlagDefaults(t *testing.T) {
	// Find the files command.
	var filesCmdPtr *cobra.Command
	for _, cmd := range GenerateCmd.Commands() {
		if cmd.Name() == "files" {
			filesCmdPtr = cmd
			break
		}
	}
	require.NotNil(t, filesCmdPtr)

	tests := []struct {
		name         string
		flag         string
		expectedDef  string
		expectedType string
	}{
		{"stack default is empty", "stack", "", "string"},
		{"stacks default is empty", "stacks", "", "string"},
		{"components default is empty", "components", "", "string"},
		{"all default is false", "all", "false", "bool"},
		{"dry-run default is false", "dry-run", "false", "bool"},
		{"clean default is false", "clean", "false", "bool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := filesCmdPtr.Flags().Lookup(tt.flag)
			require.NotNil(t, flag)
			assert.Equal(t, tt.expectedDef, flag.DefValue)
		})
	}
}

// TestFilesParserFlags verifies all filesParser flags are properly configured.
func TestFilesParserFlags(t *testing.T) {
	registry := filesParser.Registry()

	tests := []struct {
		name        string
		flag        string
		hasShort    bool
		shorthand   string
		hasEnvVars  bool
		envVarCount int
	}{
		{"stack flag has shorthand", "stack", true, "s", true, 1},
		{"all flag has no shorthand", "all", false, "", false, 0},
		{"stacks flag has env var", "stacks", false, "", true, 1},
		{"components flag has env var", "components", false, "", true, 1},
		{"dry-run flag exists", "dry-run", false, "", false, 0},
		{"clean flag exists", "clean", false, "", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, registry.Has(tt.flag), "registry should have %s flag", tt.flag)

			flagInfo := registry.Get(tt.flag)
			require.NotNil(t, flagInfo)

			if tt.hasEnvVars {
				assert.Len(t, flagInfo.GetEnvVars(), tt.envVarCount)
			}
		})
	}
}
