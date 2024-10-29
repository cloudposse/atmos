package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const atmosDocsURL = "https://atmos.tools"

// docsCmd opens the Atmos docs or displays component documentation
var docsCmd = &cobra.Command{
	Use:                "docs",
	Short:              "Open the Atmos docs or display component documentation",
	Long:               `This command opens the Atmos docs or displays the documentation for a specified Atmos component.`,
	Example:            "atmos docs vpc",
	Args:               cobra.MaximumNArgs(1),
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 1 {
			info := schema.ConfigAndStacksInfo{
				ComponentFromArg:      args[0],
				FinalComponent:        args[0],
				ComponentFolderPrefix: "terraform",
			}

			cliConfig, err := cfg.InitCliConfig(info, true)
			if err != nil {
				u.LogErrorAndExit(schema.CliConfiguration{}, err)
			}

			// Use only BasePath and FinalComponent to construct the correct path
			componentPath := path.Join(cliConfig.Components.Terraform.BasePath, info.FinalComponent)
			componentPathExists, err := u.IsDirectory(componentPath)
			if err != nil || !componentPathExists {
				u.LogErrorAndExit(schema.CliConfiguration{}, fmt.Errorf("'%s' points to the Terraform component '%s', but it does not exist in '%s'",
					info.ComponentFromArg,
					info.FinalComponent,
					cliConfig.Components.Terraform.BasePath,
				))
			}

			readmePath := path.Join(componentPath, "README.md")
			if _, err := os.Stat(readmePath); os.IsNotExist(err) {
				u.LogErrorAndExit(schema.CliConfiguration{}, fmt.Errorf("No README found for component: %s", info.FinalComponent))
			}

			readmeContent, err := os.ReadFile(readmePath)
			if err != nil {
				u.LogErrorAndExit(schema.CliConfiguration{}, err)
			}
			renderedContent, err := glamour.Render(string(readmeContent), "dark")
			if err != nil {
				u.LogErrorAndExit(schema.CliConfiguration{}, err)
			}
			fmt.Println(renderedContent)
			return
		}

		var err error
		switch runtime.GOOS {
		case "linux":
			err = exec.Command("xdg-open", atmosDocsURL).Start()
		case "windows":
			err = exec.Command("rundll32", "url.dll,FileProtocolHandler", atmosDocsURL).Start()
		case "darwin":
			err = exec.Command("open", atmosDocsURL).Start()
		default:
			err = fmt.Errorf("unsupported platform: %s", runtime.GOOS)
		}

		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
}

func init() {
	RootCmd.AddCommand(docsCmd)
}
