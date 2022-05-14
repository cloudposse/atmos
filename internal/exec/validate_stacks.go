package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"path"
	"strings"
)

// ExecuteValidateStacks executes `validate stacks` command
func ExecuteValidateStacks(cmd *cobra.Command, args []string) error {
	err := c.InitConfig()
	if err != nil {
		return err
	}

	var configAndStacksInfo c.ConfigAndStacksInfo
	err = c.ProcessConfig(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	// Include (process and validate) all YAML files in the `stacks` folder in all subfolders
	includedPaths := []string{"**/*"}
	// Don't exclude any YAML files for validation
	excludedPaths := []string{}
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(c.ProcessedConfig.StacksBaseAbsolutePath, includedPaths)
	if err != nil {
		return err
	}

	stackConfigFilesAbsolutePaths, _, err := c.FindAllStackConfigsInPaths(includeStackAbsPaths, excludedPaths)
	if err != nil {
		return err
	}

	u.PrintInfo(fmt.Sprintf("Validating all YAML files in the '%s' folder and all subfolders\n",
		path.Join(c.Config.BasePath, c.Config.Stacks.BasePath)))

	var errorMessages []string
	for _, filePath := range stackConfigFilesAbsolutePaths {
		_, _, err = s.ProcessYAMLConfigFile(c.ProcessedConfig.StacksBaseAbsolutePath, filePath, map[string]map[interface{}]interface{}{})
		if err != nil {
			errorMessages = append(errorMessages, err.Error())
		}
	}

	if len(errorMessages) > 0 {
		return errors.New(strings.Join(errorMessages, "\n\n"))
	}

	return nil
}
