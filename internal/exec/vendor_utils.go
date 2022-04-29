package exec

import (
	"context"
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/hashicorp/go-getter"
	cp "github.com/otiai10/copy"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
	"strings"
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

	return executeVendorCommandInternal(componentConfig, component, componentPath, dryRun, vendorCommand)
}

// Supports all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP),
// URL and archive formats described in https://github.com/hashicorp/go-getter
// https://www.allee.xyz/en/posts/getting-started-with-go-getter
// https://github.com/otiai10/copy
func executeVendorCommandInternal(
	componentConfig c.VendorComponentConfig,
	component string,
	componentPath string,
	dryRun bool,
	vendorCommand string,
) error {

	if vendorCommand == "pull" {
		u.PrintInfo(fmt.Sprintf("Pulling sources for the component '%s' and writing to '%s'", component, componentPath))

		if dryRun {
			return nil
		}

		// Create temp folder
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			return err
		}

		defer func(path string) {
			err := os.RemoveAll(path)
			if err != nil {
				u.PrintError(err)
			}
		}(tempDir)

		// Download the source into the temp folder
		client := &getter.Client{
			Ctx: context.Background(),
			// Define the destination to where the files will be stored. This will create the directory if it doesn't exist
			Dst: tempDir,
			Dir: true,
			// Source
			Src:  componentConfig.Source.Uri,
			Mode: getter.ClientModeDir,
		}

		if err = client.Get(); err != nil {
			return err
		}

		// Copy from the temp folder to the destination folder with skipping of some files
		copyOptions := cp.Options{
			// Skip specifies which files should be skipped
			Skip: func(src string) (bool, error) {
				return strings.HasSuffix(src, ".git"), nil
			},

			// Preserve the atime and the mtime of the entries
			// On linux we can preserve only up to 1 millisecond accuracy
			PreserveTimes: false,

			// Preserve the uid and the gid of all entries.
			PreserveOwner: false,
		}

		if err = cp.Copy(tempDir, componentPath, copyOptions); err != nil {
			return err
		}
	}

	return nil
}
