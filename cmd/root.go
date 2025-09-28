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
	"github.com/elewis787/boa"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	// LogFileMode is the file mode for log files.
	logFileMode = 0o644
)

// atmosConfig This is initialized before everything in the Execute function. So we can directly use this.
var atmosConfig schema.AtmosConfiguration

// logFileHandle holds the opened log file for the lifetime of the program.
var logFileHandle *os.File

// RootCmd represents the base command when called without any subcommands.
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
					log.Warn(err.Error())
				}
			} else {
				errUtils.CheckErrorPrintAndExit(err, "", "")
			}
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()

		// Print a styled Atmos logo to the terminal
		fmt.Println()
		err := tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			return err
		}

		err = e.ExecuteAtmosCmd()
		return err
	},
}

func setupLogger(atmosConfig *schema.AtmosConfiguration) {
	switch atmosConfig.Logs.Level {
	case "Trace":
		log.SetLevel(log.TraceLevel)
	case "Debug":
		log.SetLevel(log.DebugLevel)
	case "Info":
		log.SetLevel(log.InfoLevel)
	case "Warning":
		log.SetLevel(log.WarnLevel)
	case "Off":
		log.SetLevel(math.MaxInt32)
	default:
		log.SetLevel(log.WarnLevel)
	}

	// Always set up styles to ensure trace level shows as "TRCE"
	styles := log.DefaultStyles()

	// Set trace level to show "TRCE" instead of being blank/DEBU
	if debugStyle, ok := styles.Levels[log.DebugLevel]; ok {
		// Copy debug style but set the string to "TRCE"
		styles.Levels[log.TraceLevel] = debugStyle.SetString("TRCE")
	} else {
		// Fallback if debug style doesn't exist
		styles.Levels[log.TraceLevel] = lipgloss.NewStyle().SetString("TRCE")
	}

	// If colors are disabled, clear the colors but keep the level strings
	if !atmosConfig.Settings.Terminal.IsColorEnabled() {
		clearedStyles := &log.Styles{}
		clearedStyles.Levels = make(map[log.Level]lipgloss.Style)
		for k := range styles.Levels {
			if k == log.TraceLevel {
				// Keep TRCE string but remove color
				clearedStyles.Levels[k] = lipgloss.NewStyle().SetString("TRCE")
			} else {
				// For other levels, keep their default strings but remove color
				clearedStyles.Levels[k] = styles.Levels[k].UnsetForeground().Bold(false)
			}
		}
		log.SetStyles(clearedStyles)
	} else {
		log.SetStyles(styles)
	}
	// Only set output if a log file is configured
	if atmosConfig.Logs.File != "" {
		var output io.Writer

		switch atmosConfig.Logs.File {
		case "/dev/stderr":
			output = os.Stderr
		case "/dev/stdout":
			output = os.Stdout
		case "/dev/null":
			output = io.Discard // More efficient than opening os.DevNull
		default:
			logFile, err := os.OpenFile(atmosConfig.Logs.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
			errUtils.CheckErrorPrintAndExit(err, "Failed to open log file", "")
			// Store the file handle for later cleanup instead of deferring close.
			logFileHandle = logFile
			output = logFile
		}

		log.SetOutput(output)
	}
	if _, err := log.ParseLogLevel(atmosConfig.Logs.Level); err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
	log.Debug("Set", "logs-level", log.GetLevelString(), "logs-file", atmosConfig.Logs.File)
}

// cleanupLogFile closes the log file handle if it was opened.
func cleanupLogFile() {
	if logFileHandle != nil {
		// Flush any remaining log data before closing.
		_ = logFileHandle.Sync()
		_ = logFileHandle.Close()
		logFileHandle = nil
	}
}

// Cleanup performs cleanup operations before the program exits.
// This should be called by main when the program is terminating.
func Cleanup() {
	cleanupLogFile()
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
	errUtils.InitializeMarkdown(atmosConfig)

	if initErr != nil && !errors.Is(initErr, cfg.NotFound) {
		if isVersionCommand() {
			log.Debug("Warning: CLI configuration 'atmos.yaml' file not found", "error", initErr)
		} else {
			return initErr
		}
	}

	// Set the log level for the charmbracelet/log package based on the atmosConfig
	setupLogger(&atmosConfig)

	var err error
	// If CLI configuration was found, process its custom commands and command aliases
	if initErr == nil {
		err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
		if err != nil {
			return err
		}

		err = processCommandAliases(atmosConfig, atmosConfig.CommandAliases, RootCmd, true)
		if err != nil {
			return err
		}
	}

	// Cobra for some reason handles root command in such a way that custom usage and help command don't work as per expectations
	RootCmd.SilenceErrors = true
	cmd, err := RootCmd.ExecuteC()
	telemetry.CaptureCmd(cmd, err)
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
	// Add the template function for wrapped flag usages
	cobra.AddTemplateFunc("wrappedFlagUsages", templates.WrappedFlagUsages)

	RootCmd.PersistentFlags().String("redirect-stderr", "", "File descriptor to redirect `stderr` to. "+
		"Errors can be redirected to any file or any standard file descriptor (including `/dev/null`)")

	RootCmd.PersistentFlags().String("logs-level", "Info", "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off. If the log level is set to Off, Atmos will not log any messages")
	RootCmd.PersistentFlags().String("logs-file", "/dev/stderr", "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including '/dev/stdout', '/dev/stderr' and '/dev/null'")
	RootCmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	RootCmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration files (comma-separated or repeated flag)")
	RootCmd.PersistentFlags().StringSlice("config-path", []string{}, "Paths to configuration directories (comma-separated or repeated flag)")
	RootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	RootCmd.PersistentFlags().String("pager", "", "Enable pager for output (--pager or --pager=true to enable, --pager=false to disable, --pager=less to use specific pager)")
	// Set NoOptDefVal so --pager without value means "true"
	RootCmd.PersistentFlags().Lookup("pager").NoOptDefVal = "true"
	// Set custom usage template
	err := templates.SetCustomUsageFunc(RootCmd)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
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
				errUtils.CheckErrorPrintAndExit(err, "", "")
			}

			telemetry.PrintTelemetryDisclosure()

			if err := oldUsageFunc(command); err != nil {
				errUtils.CheckErrorPrintAndExit(err, "", "")
			}

			// Check if pager should be enabled based on flag, env var, or config
			pagerEnabled := atmosConfig.Settings.Terminal.IsPagerEnabled()

			// Check if --pager flag was explicitly set
			if pagerFlag, err := command.Flags().GetString("pager"); err == nil && pagerFlag != "" {
				// Handle --pager flag values using switch for better readability
				switch pagerFlag {
				case "true", "on", "yes", "1":
					pagerEnabled = true
				case "false", "off", "no", "0":
					pagerEnabled = false
				default:
					// Assume it's a pager command like "less" or "more"
					pagerEnabled = true
				}
			}

			pager := pager.NewWithAtmosConfig(pagerEnabled)
			if err := pager.Run("Atmos CLI Help", buf.String()); err != nil {
				log.Error("Failed to run pager", "error", err)
				utils.OsExit(1)
			}
		} else {
			fmt.Println()
			err := tuiUtils.PrintStyledText("ATMOS")
			errUtils.CheckErrorPrintAndExit(err, "", "")
			telemetry.PrintTelemetryDisclosure()

			b.HelpFunc(command, args)
			if err := command.Usage(); err != nil {
				errUtils.CheckErrorPrintAndExit(err, "", "")
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
