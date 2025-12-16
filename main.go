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
		if s, ok := sig.(syscall.Signal); ok {
			os.Exit(128 + int(s))
		}
		// Fallback to SIGINT exit code if signal type assertion fails.
		os.Exit(130)
	}()

	// Disable timestamp in logs so snapshots work. We will address this in a future PR updating styles, etc.
	log.Default().SetReportTimestamp(false)

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
			os.Exit(1)
		}
		err := cmd.ExecuteVersion()
		if err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}
		return // Exit normally after printing version
	}

	err := cmd.Execute()
	if err != nil {
		// Format and print error using centralized formatter.
		formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
		os.Stderr.WriteString(formatted + "\n")

		// Extract and use the correct exit code.
		exitCode := errUtils.GetExitCode(err)
		log.Debug("Exiting with exit code", "code", exitCode)
		errUtils.Exit(exitCode)
	}
}

// hasVersionFlag checks if --version flag is present in args.
// Only checks for --version as the first argument after the program name.
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
