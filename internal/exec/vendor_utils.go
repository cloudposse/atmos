package exec

import (
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path"
)

const (
	componentConfigFileName = "component.yaml"
)

// ExecuteVendorCommand executes `atmos vendor` commands
func ExecuteVendorCommand(cmd *cobra.Command, args []string, vendorCommand string) error {
	err := c.InitConfig()
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	dryRun, err := flags.GetBool("dry-run")
	if err != nil {
		return err
	}

	component, err := flags.GetString("component")
	if err != nil {
		return err
	}

	componentType, err := flags.GetString("type")
	if err != nil {
		return err
	}

	if componentType == "" {
		componentType = "terraform"
	}

	var componentBasePath string

	if componentType == "terraform" {
		componentBasePath = c.Config.Components.Terraform.BasePath
	} else if componentType == "helmfile" {
		componentBasePath = c.Config.Components.Helmfile.BasePath
	} else {
		return errors.New(fmt.Sprintf("type '%s' is not supported. Valid types are 'terraform' and 'helmfile'", componentType))
	}

	componentPath := path.Join(c.Config.BasePath, componentBasePath, component)

	dirExists, err := u.IsDirectory(componentPath)
	if err != nil {
		return err
	}

	if !dirExists {
		return errors.New(fmt.Sprintf("Folder '%s' does not exist", componentPath))
	}

	componentConfigFile := path.Join(componentPath, componentConfigFileName)
	if !u.FileExists(componentConfigFile) {
		return errors.New(fmt.Sprintf("Vendor config file '%s' does not exist in the '%s' folder", componentConfigFileName, componentPath))
	}

	componentConfigFileContent, err := ioutil.ReadFile(componentConfigFile)
	if err != nil {
		return err
	}

	var componentConfig c.VendorComponentConfig
	if err = yaml.Unmarshal(componentConfigFileContent, &componentConfig); err != nil {
		return err
	}

	return executeVendorCommandInternal(componentConfig, component, dryRun)
}

func executeVendorCommandInternal(componentConfig c.VendorComponentConfig, component string, dryRun bool) error {
	return nil
}
