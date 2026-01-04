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
