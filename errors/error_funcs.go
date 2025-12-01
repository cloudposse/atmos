package errors

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

const (
	// EnvVerbose is the environment variable name for verbose error output.
	EnvVerbose = "ATMOS_VERBOSE"
)

// OsExit is a variable for testing, so we can mock os.Exit.
var OsExit = os.Exit

// render is the global Markdown renderer instance initialized via InitializeMarkdown.
var render *markdown.Renderer

// atmosConfig is the global Atmos configuration for error handling.
var atmosConfig *schema.AtmosConfiguration

// verboseFlag holds the value of the --verbose flag.
var verboseFlag = false

// verboseFlagSet tracks whether the verbose flag was explicitly set via CLI.
var verboseFlagSet = false

// SetVerboseFlag sets the package-level verboseFlag to the given value and marks verboseFlagSet true.
func SetVerboseFlag(verbose bool) {
	verboseFlag = verbose
	verboseFlagSet = true
}

// InitializeMarkdown initializes a new Markdown renderer.
func InitializeMarkdown(config *schema.AtmosConfiguration) {
	// Bind ATMOS_VERBOSE environment variable once during initialization.
	_ = viper.BindEnv(EnvVerbose, EnvVerbose)

	if config == nil {
		log.Warn("InitializeMarkdown called with nil config")
		return
	}

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

// GetMarkdownRenderer returns the package-level markdown renderer and may return nil
// if the renderer has not been initialized via InitializeMarkdown or has been cleared.
// This function is not safe for concurrent access during initialization.
func GetMarkdownRenderer() *markdown.Renderer {
	return render
}

// printPlainError writes a plain-text error to stderr without Markdown formatting.
func printPlainError(title string, err error, suggestion string) {
	if title != "" {
		title = cases.Title(language.English).String(title)
		fmt.Fprintf(os.Stderr, "\n%s: %v\n", title, err)
	} else {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
	}
	if suggestion != "" {
		fmt.Fprintf(os.Stderr, "%s\n", suggestion)
	}
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
	// Pass title and suggestion to ensure backward compatibility.
	if atmosConfig != nil {
		printFormattedError(err, title, suggestion)
		return
	}

	// Config not available - use markdown renderer fallback.
	printMarkdownError(err, title, suggestion)
}

// printFormattedError prints an error using the new formatter.
// If title or suggestion are provided, they are incorporated into the formatted output.
func printFormattedError(err error, title string, suggestion string) {
	// If legacy title or suggestion are provided, incorporate them into the error.
	if title != "" || suggestion != "" {
		// Wrap error with legacy parameters using error builder.
		builder := Build(err)
		if suggestion != "" {
			// Trim leading/trailing whitespace for analysis.
			trimmed := strings.TrimSpace(suggestion)

			// Check if suggestion contains markdown sections (backward compatibility).
			// Old code passed suggestions like "\n## Explanation\n..." which should be
			// added as explanations, not hints, to avoid rendering empty hint sections.
			if strings.Contains(suggestion, "\n##") || strings.HasPrefix(trimmed, "##") {
				// Strip the "## Explanation" header if present since formatter adds it.
				cleaned := strings.TrimLeft(suggestion, "\n")
				cleaned = strings.TrimPrefix(cleaned, "## Explanation\n")
				cleaned = strings.TrimPrefix(cleaned, "## Explanation")
				cleaned = strings.TrimSpace(cleaned)
				builder = builder.WithExplanation(cleaned)
			} else {
				// Only add as hint if it's not empty after trimming.
				// This prevents rendering empty "## Hints" sections for suggestions
				// that are just whitespace or newlines.
				if trimmed != "" {
					builder = builder.WithHint(trimmed)
				}
			}
		}
		// Note: title is handled by the formatter's title parameter, not as a hint.
		err = builder.Err()
	}

	// Check for --verbose flag (CLI flag > env var > config).
	verbose := atmosConfig.Errors.Format.Verbose

	// Apply precedence: config < env < CLI.
	if viper.IsSet(EnvVerbose) {
		verbose = viper.GetBool(EnvVerbose)
	}
	if verboseFlagSet {
		verbose = verboseFlag
	}

	// Color mode is now determined by terminal settings (--no-color, --force-color, etc.)
	// and does not need to be configured here.
	config := FormatterConfig{
		Verbose:       verbose,
		MaxLineLength: DefaultMaxLineLength,
		Title:         title, // Use provided title if any.
	}
	formatted := Format(err, config)

	// Write directly to os.Stderr instead of using ui.MarkdownMessage() to avoid
	// circular dependencies and ensure error formatting works before UI initialization.
	//
	// Why not use ui.MarkdownMessage():
	// 1. Circular dependency: pkg/ui imports errors package, so errors cannot import ui
	// 2. Initialization order: Error formatting must work before the UI system is initialized
	//    (e.g., early startup errors, configuration loading failures)
	// 3. Self-contained errors package: The errors package is designed to be low-level
	//    and independent, ensuring errors can always be reported regardless of system state
	//
	// The formatted output is already rendered markdown, so writing to stderr is correct.
	_, printErr := os.Stderr.WriteString(formatted + "\n")
	if printErr != nil {
		log.Error(printErr)
		log.Error(err)
	}
}

// printMarkdownError prints an error using the markdown renderer.
func printMarkdownError(err error, title string, suggestion string) {
	// If markdown renderer is not initialized, fall back to plain error output.
	if render == nil {
		printPlainError(title, err, suggestion)
		return
	}

	if title == "" {
		title = "Error"
	}
	title = cases.Title(language.English).String(title)
	errorMarkdown, renderErr := render.RenderError(title, err.Error(), suggestion)
	if renderErr != nil {
		// Rendering failed - fall back to plain error output.
		printPlainError(title, err, suggestion)
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
	OsExit(exitCode)
}
