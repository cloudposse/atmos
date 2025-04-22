package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestExecuteTerraformGeneratePlanfileCmd(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/terraform-generate-planfile"
	componentPath := filepath.Join(stacksPath, "components", "terraform", "mock")
	component := "component-1"
	stack := "nonprod"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	defer func() {
		err := os.Unsetenv("ATMOS_BASE_PATH")
		assert.NoError(t, err)
		err = os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		assert.NoError(t, err)

		// Delete the generated files and folders after the test
		err = os.RemoveAll(filepath.Join(componentPath, ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(componentPath, "terraform.tfstate.d"))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.terraform.tfvars.json", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component))
		assert.NoError(t, err)
	}()

	cmd := &cobra.Command{
		Use:                "terraform generate planfile",
		Short:              "Generate a planfile for a Terraform component",
		Long:               "This command generates a `planfile` for a specified Atmos Terraform component.",
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
		Run: func(cmd *cobra.Command, args []string) {
			err := ExecuteTerraformGeneratePlanfileCmd(cmd, args)
			if err != nil {
				u.PrintErrorMarkdownAndExit("", err, "")
			}
		},
	}

	cmd.PersistentFlags().StringP("stack", "s", "", "Atmos stack")
	cmd.PersistentFlags().StringP("file", "f", "", "Planfile name")
	cmd.PersistentFlags().String("format", "json", "Output format (json or yaml)")
	cmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	cmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
	cmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function when processing Atmos stack manifests")
	cmd.PersistentFlags().String("logs-level", "Info", "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off. If the log level is set to Off, Atmos will not log any messages")
	cmd.PersistentFlags().String("logs-file", "/dev/stderr", "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including '/dev/stdout', '/dev/stderr' and '/dev/null'")
	cmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	cmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration files (comma-separated or repeated flag)")
	cmd.PersistentFlags().StringSlice("config-path", []string{}, "Paths to configuration directories (comma-separated or repeated flag)")

	// Execute the command
	cmd.SetArgs([]string{component, "-s", stack, "--format", "json"})
	err = cmd.Execute()
	assert.NoError(t, err, "'atmos terraform generate planfile' command should execute without error")
}

func TestExecuteTerraformGeneratePlanfile(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/terraform-generate-planfile"
	componentPath := filepath.Join(stacksPath, "components", "terraform", "mock")
	component := "component-1"
	stack := "nonprod"
	info := schema.ConfigAndStacksInfo{}

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	defer func() {
		err := os.Unsetenv("ATMOS_BASE_PATH")
		assert.NoError(t, err)
		err = os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		assert.NoError(t, err)

		// Delete the generated files and folders after the test
		err = os.RemoveAll(filepath.Join(componentPath, ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(componentPath, "terraform.tfstate.d"))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.terraform.tfvars.json", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.planfile.yaml", componentPath, stack, component))
		assert.NoError(t, err)
	}()

	err = ExecuteTerraformGeneratePlanfile(
		component,
		stack,
		"",
		"json",
		true,
		true,
		nil,
		info,
	)
	assert.NoError(t, err)

	filePath := fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	err = ExecuteTerraformGeneratePlanfile(
		component,
		stack,
		"",
		"yaml",
		true,
		true,
		nil,
		info,
	)
	assert.NoError(t, err)

	filePath = fmt.Sprintf("%s/%s-%s.planfile.yaml", componentPath, stack, component)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}
}
