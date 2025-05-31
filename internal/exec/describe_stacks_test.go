package exec

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestExecuteDescribeStacksCmd(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/atmos-stacks-validation"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	defer func() {
		err := os.Unsetenv("ATMOS_BASE_PATH")
		assert.NoError(t, err)
		err = os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		assert.NoError(t, err)
	}()

	cmd := &cobra.Command{
		Use:                "describe stacks",
		Short:              "Display configuration for Atmos stacks and their components",
		Long:               "This command shows the configuration details for Atmos stacks and the components within those stacks.",
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
		Run: func(cmd *cobra.Command, args []string) {
			err := ExecuteDescribeStacksCmd(cmd, args)
			if err != nil {
				u.PrintErrorMarkdownAndExit("", err, "")
			}
		},
	}

	cmd.PersistentFlags().String("file", "", "Write the result to file")
	cmd.PersistentFlags().String("format", "yaml", "Specify the output format (`yaml` is default)")
	cmd.PersistentFlags().StringP("stack", "s", "",
		"Filter by a specific stack\n"+
			"The filter supports names of the top-level stack manifests (including subfolder paths), and `atmos` stack names (derived from the context vars)",
	)
	cmd.PersistentFlags().String("components", "", "Filter by specific `atmos` components")
	cmd.PersistentFlags().String("component-types", "", "Filter by specific component types. Supported component types: terraform, helmfile")
	cmd.PersistentFlags().String("sections", "", "Output only the specified component sections. Available component sections: `backend`, `backend_type`, `deps`, `env`, `inheritance`, `metadata`, `remote_state_backend`, `remote_state_backend_type`, `settings`, `vars`")
	cmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	cmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
	cmd.PersistentFlags().Bool("include-empty-stacks", false, "Include stacks with no components in the output")
	cmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")
	cmd.PersistentFlags().String("logs-level", "Info", "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off. If the log level is set to Off, Atmos will not log any messages")
	cmd.PersistentFlags().String("logs-file", "/dev/stderr", "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including '/dev/stdout', '/dev/stderr' and '/dev/null'")
	cmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	cmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration files (comma-separated or repeated flag)")
	cmd.PersistentFlags().StringSlice("config-path", []string{}, "Paths to configuration directories (comma-separated or repeated flag)")
	cmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	cmd.PersistentFlags().StringP("query", "q", "", "Query the results of an `atmos describe` command using `yq` expressions")
	cmd.PersistentFlags().String("pager", "true", "Disable / Enable the paging user experience")

	// Execute the command
	err = cmd.Execute()
	assert.NoError(t, err, "'atmos describe stacks' command should execute without error")
}
