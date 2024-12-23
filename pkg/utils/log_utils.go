package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"

	"github.com/fatih/color"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	LogLevelTrace   = "Trace"
	LogLevelDebug   = "Debug"
	LogLevelInfo    = "Info"
	LogLevelWarning = "Warning"
)

// PrintMessage prints the message to the console
func PrintMessage(message string) {
	fmt.Println(message)
}

// PrintMessageInColor prints the message to the console using the provided color
func PrintMessageInColor(message string, messageColor *color.Color) {
	_, _ = messageColor.Fprint(os.Stdout, message)
}

// LogErrorAndExit logs errors to std.Error and exits with an error code
func LogErrorAndExit(atmosConfig schema.AtmosConfiguration, err error) {
	if err != nil {
		LogError(atmosConfig, err)

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
func LogError(atmosConfig schema.AtmosConfiguration, err error) {
	if err != nil {
		c := color.New(color.FgRed)
		_, printErr := c.Fprintln(color.Error, err.Error()+"\n")
		if printErr != nil {
			color.Red("Error logging the error:")
			color.Red("%s\n", printErr)
			color.Red("Original error:")
			color.Red("%s\n", err)
		}

		// Print stack trace
		if atmosConfig.Logs.Level == LogLevelTrace {
			debug.PrintStack()
		}
	}
}

// LogTrace logs the provided trace message
func LogTrace(atmosConfig schema.AtmosConfiguration, message string) {
	if atmosConfig.Logs.Level == LogLevelTrace {
		log(atmosConfig, color.New(color.FgCyan), message)
	}
}

// LogDebug logs the provided debug message
func LogDebug(atmosConfig schema.AtmosConfiguration, message string) {
	if atmosConfig.Logs.Level == LogLevelTrace ||
		atmosConfig.Logs.Level == LogLevelDebug {

		log(atmosConfig, color.New(color.FgCyan), message)
	}
}

// LogInfo logs the provided info message
func LogInfo(atmosConfig schema.AtmosConfiguration, message string) {
	// Info level is default, it's used if not set in `atmos.yaml` in the `logs.level` section
	if atmosConfig.Logs.Level == "" ||
		atmosConfig.Logs.Level == LogLevelTrace ||
		atmosConfig.Logs.Level == LogLevelDebug ||
		atmosConfig.Logs.Level == LogLevelInfo {

		log(atmosConfig, color.New(color.Reset), message)
	}
}

// LogWarning logs the provided warning message
func LogWarning(atmosConfig schema.AtmosConfiguration, message string) {
	if atmosConfig.Logs.Level == LogLevelTrace ||
		atmosConfig.Logs.Level == LogLevelDebug ||
		atmosConfig.Logs.Level == LogLevelInfo ||
		atmosConfig.Logs.Level == LogLevelWarning {

		log(atmosConfig, color.New(color.FgYellow), message)
	}
}

func log(atmosConfig schema.AtmosConfiguration, logColor *color.Color, message string) {
	if atmosConfig.Logs.File != "" {
		if atmosConfig.Logs.File == "/dev/stdout" {
			_, err := logColor.Fprintln(os.Stdout, message)
			if err != nil {
				color.Red("%s\n", err)
			}
		} else if atmosConfig.Logs.File == "/dev/stderr" {
			_, err := logColor.Fprintln(os.Stderr, message)
			if err != nil {
				color.Red("%s\n", err)
			}
		} else {
			f, err := os.OpenFile(atmosConfig.Logs.File, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
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
