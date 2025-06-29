package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/elewis787/boa"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// atmosConfig This is initialized before everything in the Execute function. So we can directly use this.
var atmosConfig schema.AtmosConfiguration

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:                "atmos",
	Short:              "Universal Tool for DevOps and Cloud Automation",
	Long:               `Atmos is a universal tool for DevOps and cloud automation used for provisioning, managing and orchestrating workflows across various toolchains`,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Determine if the command is a help command or if the help flag is set
		isHelpCommand := cmd.Name() == "help"
		helpFlag := cmd.Flags().Changed("help")

		isHelpRequested := isHelpCommand || helpFlag

		if isHelpRequested {
			// Do not silence usage or errors when help is invoked
			cmd.SilenceUsage = false
			cmd.SilenceErrors = false
		} else {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
		}
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		// Only validate the config, don't store it yet since commands may need to add more info
		_, err := cfg.InitCliConfig(configAndStacksInfo, false)
		if err != nil {
			if errors.Is(err, cfg.NotFound) {
				// For help commands or when help flag is set, we don't want to show the error
				if !isHelpRequested {
					u.LogWarning(err.Error())
				}
			} else {
				u.LogErrorAndExit(err)
			}
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		// Print a styled Atmos logo to the terminal
		fmt.Println()
		err := tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}

		err = e.ExecuteAtmosCmd()
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func setupLogger(atmosConfig *schema.AtmosConfiguration) {
	switch atmosConfig.Logs.Level {
	case "Trace":
		log.SetLevel(log.DebugLevel)
	case "Debug":
		log.SetLevel(log.DebugLevel)
	case "Info":
		log.SetLevel(log.InfoLevel)
	case "Warning":
		log.SetLevel(log.WarnLevel)
	case "Off":
		log.SetLevel(math.MaxInt32)
	default:
		log.SetLevel(log.InfoLevel)
	}

	if atmosConfig.Settings.Terminal.NoColor {
		stylesDefault := log.DefaultStyles()
		// Clear colors for levels
		styles := &log.Styles{}
		styles.Levels = make(map[log.Level]lipgloss.Style)
		for k := range stylesDefault.Levels {
			styles.Levels[k] = stylesDefault.Levels[k].UnsetForeground().Bold(false)
		}
		log.SetStyles(styles)
	}
	var output io.Writer

	switch atmosConfig.Logs.File {
	case "/dev/stderr":
		output = os.Stderr
	case "/dev/stdout":
		output = os.Stdout
	case "/dev/null":
		output = io.Discard // More efficient than opening os.DevNull
	default:
		logFile, err := os.OpenFile(atmosConfig.Logs.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err != nil {
			log.Fatal("Failed to open log file:", err)
		}
		defer logFile.Close()
		output = logFile
	}

	log.SetOutput(output)
	if _, err := logger.ParseLogLevel(atmosConfig.Logs.Level); err != nil {
		//nolint:all // The reason to escape this is because it is expected to fail fast. The reason for ignoring all is because we also get a lint error to execute defer logFile.Close() before this but that is not required
		log.Fatal(err)
	}
	log.Debug("Set", "logs-level", log.GetLevel(), "logs-file", atmosConfig.Logs.File)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the RootCmd.
func Execute() error {
	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	// Here we need the custom commands from the config
	var initErr error
	atmosConfig, initErr = cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	utils.InitializeMarkdown(atmosConfig)
	if initErr != nil && !errors.Is(initErr, cfg.NotFound) {
		if isVersionCommand() {
			log.Debug("warning: CLI configuration 'atmos.yaml' file not found", "error", initErr)
		} else {
			u.LogErrorAndExit(initErr)
		}
	}

	// Set the log level for the charmbracelet/log package based on the atmosConfig
	setupLogger(&atmosConfig)

	var err error
	// If CLI configuration was found, process its custom commands and command aliases
	if initErr == nil {
		err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
		if err != nil {
			u.LogErrorAndExit(err)
		}

		err = processCommandAliases(atmosConfig, atmosConfig.CommandAliases, RootCmd, true)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	}

	// Cobra for some reason handles root command in such a way that custom usage and help command don't work as per expectations
	RootCmd.SilenceErrors = true
	err = RootCmd.Execute()
	if err != nil {
		if strings.Contains(err.Error(), "unknown command") {
			command := getInvalidCommandName(err.Error())
			showUsageAndExit(RootCmd, []string{command})
		}
	}
	return err
}

func getInvalidCommandName(input string) string {
	// Regular expression to match the command name inside quotes
	re := regexp.MustCompile(`unknown command "([^"]+)"`)

	// Find the match
	match := re.FindStringSubmatch(input)

	// Check if a match is found
	if len(match) > 1 {
		command := match[1] // The first capturing group contains the command
		return command
	}
	return ""
}

func init() {
	// Add template function for wrapped flag usages
	cobra.AddTemplateFunc("wrappedFlagUsages", templates.WrappedFlagUsages)

	RootCmd.PersistentFlags().String("redirect-stderr", "", "File descriptor to redirect `stderr` to. "+
		"Errors can be redirected to any file or any standard file descriptor (including `/dev/null`)")

	RootCmd.PersistentFlags().String("logs-level", "Info", "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off. If the log level is set to Off, Atmos will not log any messages")
	RootCmd.PersistentFlags().String("logs-file", "/dev/stderr", "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including '/dev/stdout', '/dev/stderr' and '/dev/null'")
	RootCmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	RootCmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration files (comma-separated or repeated flag)")
	RootCmd.PersistentFlags().StringSlice("config-path", []string{}, "Paths to configuration directories (comma-separated or repeated flag)")
	RootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	// Set custom usage template
	err := templates.SetCustomUsageFunc(RootCmd)
	if err != nil {
		u.LogErrorAndExit(err)
	}

	initCobraConfig()
}

func initCobraConfig() {
	RootCmd.SetOut(os.Stdout)
	styles := boa.DefaultStyles()
	b := boa.New(boa.WithStyles(styles))
	oldUsageFunc := RootCmd.UsageFunc()
	RootCmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return showFlagUsageAndExit(c, err)
	})
	RootCmd.SetUsageFunc(func(c *cobra.Command) error {
		if c.Use == "atmos" {
			return b.UsageFunc(c)
		}
		showUsageAndExit(c, c.Flags().Args())
		return nil
	})
	RootCmd.SetHelpFunc(func(command *cobra.Command, args []string) {
		contentName := strings.ReplaceAll(strings.ReplaceAll(command.CommandPath(), " ", "_"), "-", "_")
		if exampleContent, ok := examples[contentName]; ok {
			command.Example = exampleContent.Content
		}

		if !(Contains(os.Args, "help") || Contains(os.Args, "--help") || Contains(os.Args, "-h")) {
			arguments := os.Args[len(strings.Split(command.CommandPath(), " ")):]
			if len(command.Flags().Args()) > 0 {
				arguments = command.Flags().Args()
			}
			showUsageAndExit(command, arguments)
		}
		// Print a styled Atmos logo to the terminal
		if command.Use != "atmos" || command.Flags().Changed("help") {
			var buf bytes.Buffer
			var err error
			command.SetOut(&buf)
			fmt.Println()
			if term.IsTTYSupportForStdout() {
				err = tuiUtils.PrintStyledTextToSpecifiedOutput(&buf, "ATMOS")
			} else {
				err = tuiUtils.PrintStyledText("ATMOS")
			}
			if err != nil {
				u.LogErrorAndExit(err)
			}
			if err := oldUsageFunc(command); err != nil {
				u.LogErrorAndExit(err)
			}
			pager := pager.NewWithAtmosConfig(atmosConfig.Settings.Terminal.IsPagerEnabled())
			if err := pager.Run("Atmos CLI Help", buf.String()); err != nil {
				log.Error("Failed to run pager", "error", err)
				utils.OsExit(1)
			}
		} else {
			fmt.Println()
			err := tuiUtils.PrintStyledText("ATMOS")
			if err != nil {
				u.LogErrorAndExit(err)
			}
			b.HelpFunc(command, args)
			if err := command.Usage(); err != nil {
				u.LogErrorAndExit(err)
			}
		}
		CheckForAtmosUpdateAndPrintMessage(atmosConfig)
	})
}

// https://www.sobyte.net/post/2021-12/create-cli-app-with-cobra/
// https://github.com/spf13/cobra/blob/master/user_guide.md
// https://blog.knoldus.com/create-kubectl-like-cli-with-go-and-cobra/
// https://pkg.go.dev/github.com/c-bata/go-prompt
// https://pkg.go.dev/github.com/spf13/cobra
// https://scene-si.org/2017/04/20/managing-configuration-with-viper/
