package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/elewis787/boa"
	cc "github.com/ivanpirog/coloredcobra"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	e "github.com/cloudposse/atmos/internal/exec"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "atmos",
	Short: "Automated Terraform Management & Orchestration Software",
	Long:  `Atmos is a universal tool for DevOps and cloud automation used for provisioning, managing and orchestrating workflows across various toolchains`,
	PreRun: func(cmd *cobra.Command, args []string) {
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		// Print a styled Atmos logo to the terminal
		fmt.Println()
		err := tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			u.LogErrorAndExit(err)
		}

		err = e.ExecuteAtmosCmd()
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the RootCmd.
func Execute() error {
	cc.Init(&cc.Config{
		RootCmd:  RootCmd,
		Headings: cc.HiCyan + cc.Bold + cc.Underline,
		Commands: cc.HiGreen + cc.Bold,
		Example:  cc.Italic,
		ExecName: cc.Bold,
		Flags:    cc.Bold,
	})

	// Check if the `help` flag is passed and print a styled Atmos logo to the terminal before printing the help
	err := RootCmd.ParseFlags(os.Args)
	if err != nil && errors.Is(err, pflag.ErrHelp) {
		fmt.Println()
		err = tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			u.LogErrorAndExit(err)
		}
	}

	return RootCmd.Execute()
}

func init() {
	RootCmd.PersistentFlags().String("redirect-stderr", "", "File descriptor to redirect 'stderr' to. "+
		"Errors can be redirected to any file or any standard file descriptor (including '/dev/null'): atmos <command> --redirect-stderr /dev/stdout")

	RootCmd.PersistentFlags().String("logs-level", "Info", "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off. If the log level is set to Off, Atmos will not log any messages")
	RootCmd.PersistentFlags().String("logs-file", "/dev/stdout", "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including '/dev/stdout', '/dev/stderr' and '/dev/null'")

	cobra.OnInitialize(initConfig)

	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	// Here we need the custom commands from the config
	cliConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil && !errors.Is(err, cfg.NotFound) {
		u.LogErrorAndExit(err)
	}

	// If CLI configuration was found, add its custom commands
	if err == nil {
		err = processCustomCommands(cliConfig, cliConfig.Commands, RootCmd, true)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	}
}

func initConfig() {
	styles := boa.DefaultStyles()
	styles.Border.BorderTop(false)
	styles.Border.BorderBottom(false)
	styles.Border.BorderLeft(false)
	styles.Border.BorderRight(false)
	styles.Title.BorderTop(false)
	styles.Title.BorderBottom(false)
	styles.Title.BorderLeft(false)
	styles.Title.BorderRight(false)

	b := boa.New(boa.WithStyles(styles))

	RootCmd.SetUsageFunc(b.UsageFunc)

	RootCmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		// Print a styled Atmos logo to the terminal
		fmt.Println()
		err := tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			u.LogErrorAndExit(err)
		}

		b.HelpFunc(command, strings)
	})
}

// https://www.sobyte.net/post/2021-12/create-cli-app-with-cobra/
// https://github.com/spf13/cobra/blob/master/user_guide.md
// https://blog.knoldus.com/create-kubectl-like-cli-with-go-and-cobra/
// https://pkg.go.dev/github.com/c-bata/go-prompt
// https://pkg.go.dev/github.com/spf13/cobra
// https://scene-si.org/2017/04/20/managing-configuration-with-viper/
