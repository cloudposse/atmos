package exec

import (
	c "atmos/internal/config"
	u "atmos/internal/utils"
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"path"
	"strings"
)

// ExecuteTerraform executes terraform commands
func ExecuteTerraform(cmd *cobra.Command, args []string) error {
	if len(args) < 3 {
		return errors.New("invalid number of arguments")
	}

	cmd.DisableFlagParsing = false

	err := cmd.ParseFlags(args)
	if err != nil {
		return err
	}
	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	additionalArgsAndFlags := removeCommonArgsAndFlags(args)
	terraformSubCommand := args[0]
	allArgsAndFlags := append([]string{terraformSubCommand}, additionalArgsAndFlags...)

	component := args[1]
	if len(component) < 1 {
		return errors.New("'component' is required")
	}
	componentPath := path.Join(c.Config.TerraformDirAbsolutePath, component)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' does not exixt at '%s'", component, componentPath))
	}

	fmt.Println(strings.Repeat("-", 120))
	fmt.Println("Terraform command: " + terraformSubCommand)
	fmt.Println("Component: " + component)
	fmt.Println("Stack: " + stack)
	fmt.Printf("Additional arguments: %v\n", additionalArgsAndFlags)
	fmt.Println(strings.Repeat("-", 120))

	err = execCommand("terraform", allArgsAndFlags)
	if err != nil {
		return err
	}

	return nil
}
