package utils

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	"github.com/cloudposse/atmos/pkg/schema"
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
			color.Red("Error logging the error to std.Error:")
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

// LogMessage logs the provided message
func LogMessage(cliConfig schema.CliConfiguration, message string) {
	log(cliConfig, color.New(color.Reset), message)
}

func log(cliConfig schema.CliConfiguration, logColor *color.Color, message string) {
	if cliConfig.Logs.Level == "Off" {
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
