package utils

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	LogLevelOff     = "Off"
	LogLevelTrace   = "Trace"
	LogLevelDebug   = "Debug"
	LogLevelInfo    = "Info"
	LogLevelWarning = "Warning"
)

// LogErrorAndExit logs errors to std.Error and exits with an error code
func LogErrorAndExit(err error) {
	if err != nil {
		LogError(err)
		os.Exit(1)
	}
}

// LogError logs errors to std.Error
func LogError(err error) {
	if err != nil {
		c := color.New(color.FgRed)
		_, err2 := c.Fprintln(color.Error, err.Error()+"\n")
		if err2 != nil {
			color.Red("Error logging the error:")
			color.Red("%s\n", err2)
			color.Red("Original error:")
			color.Red("%s\n", err)
		}
	}
}

// LogInfo logs the provided info message
func LogInfo(cliConfig schema.CliConfiguration, message string) {
	log(cliConfig, color.New(color.FgCyan), message)
}

// LogTrace logs the provided trace message
func LogTrace(cliConfig schema.CliConfiguration, message string) {
	log(cliConfig, color.New(color.Reset), message)
}

// LogDebug logs the provided debug message
func LogDebug(cliConfig schema.CliConfiguration, message string) {
	if cliConfig.Logs.Level == LogLevelDebug {
		log(cliConfig, color.New(color.FgYellow), message)
	}
}

// LogWarning logs the provided warning message
func LogWarning(cliConfig schema.CliConfiguration, message string) {
	if cliConfig.Logs.Level == LogLevelWarning {
		log(cliConfig, color.New(color.FgYellow), message)
	}
}

// LogMessage logs the provided message
func LogMessage(cliConfig schema.CliConfiguration, message string) {
	log(cliConfig, color.New(color.Reset), message)
}

func log(cliConfig schema.CliConfiguration, logColor *color.Color, message string) {
	if cliConfig.Logs.Level == LogLevelOff {
		return
	}

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
			f, err := os.OpenFile(cliConfig.Logs.File, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0644)
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
