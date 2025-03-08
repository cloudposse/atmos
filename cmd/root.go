package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/elewis787/boa"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/internal/tui/templates"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/logger"
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
		// TODO: Check if these value being set was actually required
		if cmd.Flags().Changed("logs-level") {
			logsLevel, _ := cmd.Flags().GetString("logs-level")
			configAndStacksInfo.LogsLevel = logsLevel
		}
		if cmd.Flags().Changed("logs-file") {
			logsFile, _ := cmd.Flags().GetString("logs-file")
			configAndStacksInfo.LogsFile = logsFile
		}

		// Only validate the config, don't store it yet since commands may need to add more info
		_, err := config.InitCliConfig(configAndStacksInfo, false)
		if err != nil {
			if errors.Is(err, config.NotFound) {
				// For help commands or when help flag is set, we don't want to show the error
				if !isHelpRequested {
					log.Warn("CLI configuration issue", "error", err)
				}
			} else {
				log.Fatal("CLI configuration error", "error", err)
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

// setupLogger configures the global logger based on application configuration using our logger pkg.
func setupLogger(atmosConfig *schema.AtmosConfiguration) {
	atmosLogger, err := logger.InitializeLoggerFromCliConfig(atmosConfig)
	if err != nil {
		log.Error("Failed to initialize logger from config", "error", err)

		log.SetLevel(log.InfoLevel)
		log.SetOutput(os.Stderr)
		return
	}

	globalLogLevel := mapToCharmLevel(atmosLogger.LogLevel)
	log.SetLevel(globalLogLevel)

	configureLogOutput(atmosConfig.Logs.File)
}

// mapToCharmLevel converts our internal log level to a Charmbracelet log level.
func mapToCharmLevel(logLevel logger.LogLevel) log.Level {
	switch logLevel {
	case logger.LogLevelTrace:
		return log.DebugLevel // Charmbracelet doesn't have Trace
	case logger.LogLevelDebug:
		return log.DebugLevel
	case logger.LogLevelInfo:
		return log.InfoLevel
	case logger.LogLevelWarning:
		return log.WarnLevel
	case logger.LogLevelOff:
		return log.FatalLevel + 1 // Disable logging
	default:
		return log.InfoLevel
	}
}

// configureLogOutput sets up the output destination for the global logger.
func configureLogOutput(logFile string) {
	// Handle standard output destinations
	switch logFile {
	case "/dev/stderr":
		log.SetOutput(os.Stderr)
		return
	case "/dev/stdout", "":
		log.SetOutput(os.Stdout)
		return
	case "/dev/null":
		log.SetOutput(io.Discard)
		return
	}

	// Handle custom log file (anything not a standard stream)
	customFile, err := os.OpenFile(logFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, config.StandardFilePermissions)
	if err != nil {
		log.Error("Failed to open log file, using stderr", "error", err)
		log.SetOutput(os.Stderr)
		return
	}

	// Important: We need to register this file to be closed at program exit
	// to prevent file descriptor leaks, since we can't use defer here
	// The Go runtime will close this file when the program exits, and the OS
	// would clean it up anyway, but this is cleaner practice.
	log.SetOutput(customFile)
}

// TODO: This function works well, but we should generally avoid implementing manual flag parsing,
// as Cobra typically handles this.

// If there's no alternative, this approach may be necessary.
// However, this TODO serves as a reminder to revisit and verify if a better solution exists.

// Function to manually parse flags with double dash "--" like Cobra
func parseFlags(args []string) (map[string]string, error) {
	flags := make(map[string]string)
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Check if the argument starts with '--' (double dash)
		if strings.HasPrefix(arg, "--") {
			// Strip the '--' prefix and check if it's followed by a value
			arg = arg[2:]
			if strings.Contains(arg, "=") {
				// Case like --flag=value
				parts := strings.SplitN(arg, "=", 2)
				flags[parts[0]] = parts[1]
			} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				// Case like --flag value
				flags[arg] = args[i+1]
				i++ // Skip the next argument as it's the value
			} else {
				// Case where flag has no value, e.g., --flag (we set it to "true")
				flags[arg] = "true"
			}
		} else {
			// It's a regular argument, not a flag, so we skip
			continue
		}
	}
	return flags, nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the RootCmd.
func Execute() error {
	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	// Here we need the custom commands from the config
	var initErr error
	atmosConfig, initErr = config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	utils.InitializeMarkdown(atmosConfig)
	if initErr != nil && !errors.Is(initErr, config.NotFound) {
		if isVersionCommand() {
			log.Debug("warning: CLI configuration 'atmos.yaml' file not found", "error", initErr)
		} else {
			u.LogErrorAndExit(initErr)
		}
	}

	// TODO: This is a quick patch to mitigate the issue we can look for better code later
	if os.Getenv("ATMOS_LOGS_LEVEL") != "" {
		atmosConfig.Logs.Level = os.Getenv("ATMOS_LOGS_LEVEL")
	}
	flagKeyValue, _ := parseFlags(os.Args)
	if v, ok := flagKeyValue["logs-level"]; ok {
		atmosConfig.Logs.Level = v
	}
	if os.Getenv("ATMOS_LOGS_FILE") != "" {
		atmosConfig.Logs.File = os.Getenv("ATMOS_LOGS_FILE")
	}
	if v, ok := flagKeyValue["logs-file"]; ok {
		atmosConfig.Logs.File = v
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
	RootCmd.PersistentFlags().String("logs-file", "/dev/stderr", "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including `/dev/stdout`, `/dev/stderr` and `/dev/null`")

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
		fmt.Println()
		if command.Use != "atmos" || command.Flags().Changed("help") {
			err := tuiUtils.PrintStyledText("ATMOS")
			if err != nil {
				u.LogErrorAndExit(err)
			}
			if err := oldUsageFunc(command); err != nil {
				u.LogErrorAndExit(err)
			}
		} else {
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
