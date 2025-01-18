package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/fatih/color"
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

func PrintErrorInColor(message string) {
	messageColor := theme.Colors.Error
	_, _ = messageColor.Fprint(os.Stderr, message)
}

// LogErrorAndExit logs errors to std.Error and exits with an error code
func LogErrorAndExit(atmosConfig schema.AtmosConfiguration, err error) {
	if err != nil {
		LogError(atmosConfig, err)

		// Find the executed command's exit code from the error
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			os.Exit(exitError.ExitCode())
		}

		os.Exit(1)
	}
}

// LogError logs errors to std.Error
func LogError(atmosConfig schema.AtmosConfiguration, err error) {
	if err != nil {
		_, printErr := theme.Colors.Error.Fprintln(color.Error, err.Error())
		if printErr != nil {
			theme.Colors.Error.Println("Error logging the error:")
			theme.Colors.Error.Printf("%s\n", printErr)
			theme.Colors.Error.Println("Original error:")
			theme.Colors.Error.Printf("%s\n", err)
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
		log(atmosConfig, theme.Colors.Info, message)
	}
}

// LogDebug logs the provided debug message
func LogDebug(atmosConfig schema.AtmosConfiguration, message string) {
	if atmosConfig.Logs.Level == LogLevelTrace ||
		atmosConfig.Logs.Level == LogLevelDebug {

		log(atmosConfig, theme.Colors.Info, message)
	}
}

// LogInfo logs the provided info message
func LogInfo(atmosConfig schema.AtmosConfiguration, message string) {
	// Info level is default, it's used if not set in `atmos.yaml` in the `logs.level` section
	if atmosConfig.Logs.Level == "" ||
		atmosConfig.Logs.Level == LogLevelTrace ||
		atmosConfig.Logs.Level == LogLevelDebug ||
		atmosConfig.Logs.Level == LogLevelInfo {

		log(atmosConfig, theme.Colors.Default, message)
	}
}

// LogWarning logs the provided warning message
func LogWarning(atmosConfig schema.AtmosConfiguration, message string) {
	if atmosConfig.Logs.Level == LogLevelTrace ||
		atmosConfig.Logs.Level == LogLevelDebug ||
		atmosConfig.Logs.Level == LogLevelInfo ||
		atmosConfig.Logs.Level == LogLevelWarning {

		log(atmosConfig, theme.Colors.Warning, message)
	}
}

func log(atmosConfig schema.AtmosConfiguration, logColor *color.Color, message string) {
	if atmosConfig.Logs.File != "" {
		if atmosConfig.Logs.File == "/dev/stdout" {
			_, err := logColor.Fprintln(os.Stdout, message)
			if err != nil {
				theme.Colors.Error.Printf("%s\n", err)
			}
		} else if atmosConfig.Logs.File == "/dev/stderr" {
			_, err := logColor.Fprintln(os.Stderr, message)
			if err != nil {
				theme.Colors.Error.Printf("%s\n", err)
			}
		} else {
			f, err := os.OpenFile(atmosConfig.Logs.File, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
			if err != nil {
				theme.Colors.Error.Printf("%s\n", err)
				return
			}

			defer func(f *os.File) {
				err = f.Close()
				if err != nil {
					theme.Colors.Error.Printf("%s\n", err)
				}
			}(f)

			_, err = f.Write([]byte(fmt.Sprintf("%s\n", message)))
			if err != nil {
				theme.Colors.Error.Printf("%s\n", err)
			}
		}
	} else {
		_, err := logColor.Fprintln(os.Stdout, message)
		if err != nil {
			theme.Colors.Error.Printf("%s\n", err)
		}
	}
}
