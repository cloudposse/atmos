package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands",
	Long:               `This command executes Terraform commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		//checkAtmosConfig()

		var argsAfterDoubleDash []string
		var finalArgs = args

		doubleDashIndex := lo.IndexOf(args, "--")
		if doubleDashIndex > 0 {
			finalArgs = lo.Slice(args, 0, doubleDashIndex)
			argsAfterDoubleDash = lo.Slice(args, doubleDashIndex+1, len(args))
		}
		info, err := e.ProcessCommandLineArgs("terraform", cmd, finalArgs, argsAfterDoubleDash)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}

		// Exit on help
		if info.NeedHelp {
			// Check for the latest Atmos release on GitHub and print update message
			CheckForAtmosUpdateAndPrintMessage(atmosConfig)
			return
		}
		// Check Atmos configuration
		checkAtmosConfig()

		//Load stack from Github
		folderFlag, _ := cmd.Flags().GetString("folder")
		if folderFlag != "" && u.IsGithubURL(info.Stack) {

			data, err := u.DownloadFileFromGitHub(info.Stack)
			if err != nil {
				u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
			}
			fileName := u.ParseFilenameFromURL(info.Stack)
			if fileName == "" {
				fileName = "stack.yaml" // fallback
			}
			localPath := filepath.Join(folderFlag, fileName)

			// Overwrite if it exists
			err = os.WriteFile(localPath, data, 0o644)
			if err != nil {
				u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
			}
			shortStackName := strings.TrimSuffix(fileName, filepath.Ext(fileName))
			info.Stack = shortStackName
		}

		err = e.ExecuteTerraform(info)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	terraformCmd.PersistentFlags().String(
		"folder",
		"",
		"If set, download the remote stack file into this folder, then treat it as a local stack",
	)
	RootCmd.AddCommand(terraformCmd)
}
