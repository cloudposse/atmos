package cmd

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/elewis787/boa"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/colored"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/internal/tui/templates"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
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
	colored.Init(&colored.Config{
		RootCmd: RootCmd,
	})

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

	RootCmd.PersistentFlags().String("redirect-stderr", "", "File descriptor to redirect 'stderr' to. "+
		"Errors can be redirected to any file or any standard file descriptor (including '/dev/null'): atmos <command> --redirect-stderr /dev/stdout")

	RootCmd.PersistentFlags().String("logs-level", "Info", "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off. If the log level is set to Off, Atmos will not log any messages")
	RootCmd.PersistentFlags().String("logs-file", "/dev/stderr", "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including '/dev/stdout', '/dev/stderr' and '/dev/null'")
	RootCmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	RootCmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration file")
	RootCmd.PersistentFlags().StringSlice("config-path", []string{}, "Path to configuration directory")
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

// https://www.sobyte.net/post/2021-12/create-cli-app-with-cobra/
// https://github.com/spf13/cobra/blob/master/user_guide.md
// https://blog.knoldus.com/create-kubectl-like-cli-with-go-and-cobra/
// https://pkg.go.dev/github.com/c-bata/go-prompt
// https://pkg.go.dev/github.com/spf13/cobra
// https://scene-si.org/2017/04/20/managing-configuration-with-viper/
