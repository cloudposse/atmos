package main

import (
	"os"
	"os/signal"
	"strings"
	"syscall"

	log "github.com/cloudposse/atmos/pkg/logger"

	"github.com/cloudposse/atmos/cmd"
	errUtils "github.com/cloudposse/atmos/errors"
)

func main() {
	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		// Clean up resources before exit.
		cmd.Cleanup()
		// Exit with correct POSIX exit code (128 + signal number).
		// Use errUtils.OsExit to allow test interception (Go 1.25+ panics on os.Exit in tests).
		if s, ok := sig.(syscall.Signal); ok {
			errUtils.OsExit(128 + int(s))
		}
		// Fallback to SIGINT exit code if signal type assertion fails.
		errUtils.OsExit(130)
	}()

	// Disable timestamp in logs so snapshots work. We will address this in a future PR updating styles, etc.
	log.Default().SetReportTimestamp(false)

	// Run the application and exit with the appropriate code.
	// Use errUtils.OsExit to allow test interception (Go 1.25+ panics on os.Exit in tests).
	errUtils.OsExit(run())
}

// run executes the main application logic and returns an exit code.
// This separation allows proper cleanup via defer before os.Exit in main().
func run() int {
	// Ensure cleanup happens on normal exit.
	defer cmd.Cleanup()

	// Handle --version flag at application entry point to avoid deep exit in command infrastructure.
	// This eliminates the need for os.Exit in PersistentPreRun, making tests work with Go 1.25.
	// Check os.Args directly since we're in main() (tests call cmd.Execute() directly).
	// Note: Only intercept --version flag here. The "version" subcommand should go through
	// normal Cobra flow to ensure PersistentPreRun executes (needed for proper logging setup).
	if hasVersionFlag(os.Args) {
		// Check for conflicting flags: --version and --use-version cannot be used together.
		if hasUseVersionFlag(os.Args) {
			// Print error directly since config/formatters aren't initialized yet.
			os.Stderr.WriteString("\nError: --version and --use-version cannot be used together\n\n")
			os.Stderr.WriteString("Hints:\n")
			os.Stderr.WriteString("  - Use --version to display the current Atmos version\n")
			os.Stderr.WriteString("  - Use --use-version to run a command with a specific Atmos version\n\n")
			return 1
		}
		err := cmd.ExecuteVersion()
		if err != nil {
			errUtils.CaptureError(err)
			formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
			os.Stderr.WriteString(formatted + "\n")
			return errUtils.GetExitCode(err)
		}
		return 0 // Exit normally after printing version
	}

	err := cmd.Execute()
	if err != nil {
		// Capture error to Sentry if configured (safe to call even if Sentry not initialized).
		errUtils.CaptureError(err)

		// Format and print error using centralized formatter.
		formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
		os.Stderr.WriteString(formatted + "\n")

		// Extract and use the correct exit code.
		exitCode := errUtils.GetExitCode(err)
		log.Debug("Exiting with exit code", "code", exitCode)
		return exitCode
	}

	return 0
}

// hasVersionFlag checks if --version flag is present in args.
// Only checks for --version as the first argument after the program name
// to catch the simple "atmos --version" case for early exit; other flag
// combinations go through normal Cobra processing.
func hasVersionFlag(args []string) bool {
	return len(args) > 1 && args[1] == "--version"
}

// hasUseVersionFlag checks if --use-version flag is present in args.
func hasUseVersionFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--use-version" || strings.HasPrefix(arg, "--use-version=") {
			return true
		}
	}
	return false
}
