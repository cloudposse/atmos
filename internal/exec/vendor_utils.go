package exec

import (
	"context"
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/hashicorp/go-getter"
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

	return executeVendorCommandInternal(componentConfig, component, componentPath, dryRun, vendorCommand)
}

// Supports all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP),
// URL and archive formats described in https://github.com/hashicorp/go-getter
// https://www.allee.xyz/en/posts/getting-started-with-go-getter/
func executeVendorCommandInternal(
	componentConfig c.VendorComponentConfig,
	component string,
	componentPath string,
	dryRun bool,
	vendorCommand string,
) error {

	if vendorCommand == "pull" {

		client := &getter.Client{
			Ctx: context.Background(),
			// Define the destination to where the files will be stored. This will create the directory if it doesn't exist
			Dst: componentPath,
			Dir: true,
			// Source
			Src:  componentConfig.Source.Uri,
			Mode: getter.ClientModeDir,
			// Define the type of detectors go getter should use, in this case only github is needed
			Detectors: []getter.Detector{
				&getter.GitHubDetector{},
			},
			// Provide the getter needed to download the files
			Getters: map[string]getter.Getter{
				"git": &getter.GitGetter{},
			},
		}

		if err := client.Get(); err != nil {
			return err
		}

		//_, err := git.PlainClone(componentPath, false,
		//	&git.CloneOptions{
		//		URL:           componentConfig.Source.Uri,
		//		SingleBranch:  true,
		//		Depth:         1,
		//		ReferenceName: plumbing.NewTagReferenceName(componentConfig.Source.Version),
		//	})
		//
		//if err != nil {
		//	return err
		//}

		//args := []string{
		//	"subtree",
		//	"add",
		//	"--prefix",
		//	componentPath + "2",
		//	componentConfig.Source.Uri,
		//	componentConfig.Source.Version,
		//	"--squash",
		//}
		//
		//if err := ExecuteShellCommand("git", args, ".", []string{}, dryRun); err != nil {
		//	return err
		//}
	}

	return nil
}
