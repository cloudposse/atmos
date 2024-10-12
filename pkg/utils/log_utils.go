package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"sync"

	"github.com/fatih/color"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	LogLevelTrace   = "Trace"
	LogLevelDebug   = "Debug"
	LogLevelInfo    = "Info"
	LogLevelWarning = "Warning"
)

var (
	PrintDebugPart bool
	mu             sync.Mutex
)

// set PrintDebugPart to true if log level is Debug or Trace
func SetPrintDebugPart(val string) error {
	mu.Lock()
	defer mu.Unlock()
	if val == "" {
		return fmt.Errorf("log level not set")
	}
	if val == string(LogLevelDebug) || val == string(LogLevelTrace) {
		PrintDebugPart = true
	} else {
		PrintDebugPart = false
	}
	return nil
}

// ExtendedError is an error type that includes a message and debug info.
type ExtendedError struct {
	Message   string // Error message to be printed with log level Error , Warning or Info
	DebugInfo string // Debug info to be printed with log level Debug or Trace
}

// Error returns the error message . If PrintDebugPart is true, it returns the error message and debug info
func (e *ExtendedError) Error() string {
	// Print debug info if PrintDebugPart is true
	if PrintDebugPart {
		return fmt.Sprintf("%s\n%s", e.Message, e.DebugInfo)
	}
	return e.Message
}

// PrintMessage prints the message to the console
func PrintMessage(message string) {
	fmt.Println(message)
}

// PrintMessageInColor prints the message to the console using the provided color
func PrintMessageInColor(message string, messageColor *color.Color) {
	_, _ = messageColor.Fprint(os.Stdout, message)
}

// LogErrorAndExit logs errors to std.Error and exits with an error code
func LogErrorAndExit(cliConfig schema.CliConfiguration, err error) {
	if err != nil {
		LogError(cliConfig, err)

		// Find the executed command's exit code from the error
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode := exitError.ExitCode()
			os.Exit(exitCode)
		}

		os.Exit(1)
	}
}

// LogError logs errors to std.Error
func LogError(cliConfig schema.CliConfiguration, err error) {
	if err != nil {
		// set PrintDebugPart to true if log level is Debug or Trace
		if cliConfig.Logs.Level == LogLevelDebug || cliConfig.Logs.Level == LogLevelTrace {
			if setErr := SetPrintDebugPart(cliConfig.Logs.Level); setErr != nil {
				color.Red("%s\n", setErr)
			}
		}
		c := color.New(color.FgRed)
		_, printErr := c.Fprintln(color.Error, err.Error()+"\n")
		if printErr != nil {
			color.Red("Error logging the error:")
			color.Red("%s\n", printErr)
			color.Red("Original error:")
			color.Red("%s\n", err)
		}

		// Print stack trace
		if cliConfig.Logs.Level == LogLevelTrace {
			debug.PrintStack()
		}
	}
}

// LogTrace logs the provided trace message
func LogTrace(cliConfig schema.CliConfiguration, message string) {
	if cliConfig.Logs.Level == LogLevelTrace {
		log(cliConfig, color.New(color.FgCyan), message)
	}
}

// LogDebug logs the provided debug message
func LogDebug(cliConfig schema.CliConfiguration, message string) {
	if cliConfig.Logs.Level == LogLevelTrace ||
		cliConfig.Logs.Level == LogLevelDebug {

		log(cliConfig, color.New(color.FgCyan), message)
	}
}

// LogInfo logs the provided info message
func LogInfo(cliConfig schema.CliConfiguration, message string) {
	// Info level is default, it's used if not set in `atmos.yaml` in the `logs.level` section
	if cliConfig.Logs.Level == "" ||
		cliConfig.Logs.Level == LogLevelTrace ||
		cliConfig.Logs.Level == LogLevelDebug ||
		cliConfig.Logs.Level == LogLevelInfo {

		log(cliConfig, color.New(color.Reset), message)
	}
}

// LogWarning logs the provided warning message
func LogWarning(cliConfig schema.CliConfiguration, message string) {
	if cliConfig.Logs.Level == LogLevelTrace ||
		cliConfig.Logs.Level == LogLevelDebug ||
		cliConfig.Logs.Level == LogLevelInfo ||
		cliConfig.Logs.Level == LogLevelWarning {

		log(cliConfig, color.New(color.FgYellow), message)
	}
}

func log(cliConfig schema.CliConfiguration, logColor *color.Color, message string) {
	if cliConfig.Logs.File != "" {
		if cliConfig.Logs.File == "/dev/stdout" {
			_, err := logColor.Fprintln(os.Stdout, message)
			if err != nil {
				color.Red("%s\n", err)
			}
		} else if cliConfig.Logs.File == "/dev/stderr" {
			_, err := logColor.Fprintln(os.Stderr, message)
			if err != nil {
				color.Red("%s\n", err)
			}
		} else {
			f, err := os.OpenFile(cliConfig.Logs.File, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
			if err != nil {
				color.Red("%s\n", err)
				return
			}

			defer func(f *os.File) {
				err = f.Close()
				if err != nil {
					color.Red("%s\n", err)
				}
			}(f)

			_, err = f.Write([]byte(fmt.Sprintf("%s\n", message)))
			if err != nil {
				color.Red("%s\n", err)
			}
		}
	} else {
		_, err := logColor.Fprintln(os.Stdout, message)
		if err != nil {
			color.Red("%s\n", err)
		}
	}
}
