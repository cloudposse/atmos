package exec

import (
	c "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"path"
	"path/filepath"
	"strings"
)

// ExecuteValidateStacks executes `validate stacks` command
func ExecuteValidateStacks(cmd *cobra.Command, args []string) error {
	err := c.InitConfig()
	if err != nil {
		return err
	}

	stacksBasePath := path.Join(c.Config.BasePath, c.Config.Stacks.BasePath)
	stacksBaseAbsPath, err := filepath.Abs(stacksBasePath)
	if err != nil {
		return err
	}

	includedPaths := []string{"**/*"}
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, includedPaths)
	if err != nil {
		return err
	}

	stackConfigFilesAbsolutePaths, _, err := c.FindAllStackConfigsInPaths(
		includeStackAbsPaths,
		[]string{},
	)

	if err != nil {
		return err
	}

	var errorMessages []string

	for _, filePath := range stackConfigFilesAbsolutePaths {
		_, _, err = s.ProcessYAMLConfigFile(stacksBaseAbsPath, filePath, map[string]map[interface{}]interface{}{})
		if err != nil {
			errorMessages = append(errorMessages, err.Error())
		}
	}

	if len(errorMessages) > 0 {
		return errors.New(strings.Join(errorMessages, "\n\n"))
	}

	return nil
}
