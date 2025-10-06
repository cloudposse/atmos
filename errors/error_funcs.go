package errors

import (
	"os"

	"github.com/spf13/viper"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

// render is the global Markdown renderer instance initialized via InitializeMarkdown.
var render *markdown.Renderer

// atmosConfig is the global Atmos configuration for error handling.
var atmosConfig *schema.AtmosConfiguration

// verboseFlag holds the value of the --verbose flag.
var verboseFlag = false

// SetVerboseFlag sets the verbose flag value.
func SetVerboseFlag(verbose bool) {
	verboseFlag = verbose
}

// InitializeMarkdown initializes a new Markdown renderer.
func InitializeMarkdown(config *schema.AtmosConfiguration) {
	var err error
	render, err = markdown.NewTerminalMarkdownRenderer(*config)
	if err != nil {
		log.Error("failed to initialize Markdown renderer", "error", err)
	}

	// Store config for error formatting.
	atmosConfig = config

	// Initialize Sentry if configured.
	if config.Errors.Sentry.Enabled {
		if err := InitializeSentry(&config.Errors.Sentry); err != nil {
			log.Warn("failed to initialize Sentry", "error", err)
		}
	}
}

// GetMarkdownRenderer returns the global markdown renderer.
func GetMarkdownRenderer() *markdown.Renderer {
	return render
}

// CheckErrorAndPrint prints an error message.
func CheckErrorAndPrint(err error, title string, suggestion string) {
	if err == nil {
		return
	}

	// Capture error to Sentry if configured.
	if atmosConfig != nil && atmosConfig.Errors.Sentry.Enabled {
		CaptureError(err)
	}

	// Use new error formatter if config is available.
	if atmosConfig != nil {
		printFormattedError(err)
		return
	}

	// Fallback to old markdown renderer.
	printMarkdownError(err, title, suggestion)
}

// printFormattedError prints an error using the new formatter.
func printFormattedError(err error) {
	// Bind ATMOS_VERBOSE environment variable.
	_ = viper.BindEnv("ATMOS_VERBOSE", "ATMOS_VERBOSE")

	// Check for --verbose flag (CLI flag > env var > config).
	verbose := atmosConfig.Errors.Format.Verbose
	if verboseFlag {
		verbose = true
	}
	if viper.GetBool("ATMOS_VERBOSE") {
		verbose = true
	}

	// Determine color mode.
	colorMode := atmosConfig.Errors.Format.Color
	if colorMode == "" {
		colorMode = "auto"
	}

	config := FormatterConfig{
		Verbose:       verbose,
		Color:         colorMode,
		MaxLineLength: DefaultMaxLineLength,
	}
	formatted := Format(err, config)
	_, printErr := os.Stderr.WriteString(formatted + "\n")
	if printErr != nil {
		log.Error(printErr)
		log.Error(err)
	}
}

// printMarkdownError prints an error using the markdown renderer.
func printMarkdownError(err error, title string, suggestion string) {
	if render == nil {
		log.Error(err)
		return
	}
	if title == "" {
		title = "Error"
	}
	title = cases.Title(language.English).String(title)
	errorMarkdown, renderErr := render.RenderError(title, err.Error(), suggestion)
	if renderErr != nil {
		log.Error(renderErr)
		log.Error(err)
		return
	}
	_, printErr := os.Stderr.WriteString(errorMarkdown + "\n")
	if printErr != nil {
		log.Error(printErr)
		log.Error(err)
	}
}

// CheckErrorPrintAndExit prints an error message and exits with exit code 1.
func CheckErrorPrintAndExit(err error, title string, suggestion string) {
	if err == nil {
		return
	}

	CheckErrorAndPrint(err, title, suggestion)

	// Close Sentry before exiting.
	if atmosConfig != nil && atmosConfig.Errors.Sentry.Enabled {
		CloseSentry()
	}

	// Get exit code from error (supports custom codes and exec.ExitError).
	exitCode := GetExitCode(err)

	// TODO: Refactor so that we only call `os.Exit` in `main()` or `init()` functions.
	// Exiting here makes it difficult to test.
	// revive:disable-next-line:deep-exit
	Exit(exitCode)
}

// Exit exits the program with the specified exit code.
func Exit(exitCode int) {
	os.Exit(exitCode)
}
