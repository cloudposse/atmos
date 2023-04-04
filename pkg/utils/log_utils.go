package utils

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	"github.com/cloudposse/atmos/pkg/schema"
)

// LogErrorToStdErrorAndExit logs errors to std.Error and exits with an error code
func LogErrorToStdErrorAndExit(err error) {
	if err != nil {
		LogErrorToStdError(err)
		os.Exit(1)
	}
}

// LogErrorToStdError logs errors to std.Error
func LogErrorToStdError(err error) {
	if err != nil {
		red := color.New(color.FgRed)
		_, err2 := red.Fprintln(color.Error, err.Error()+"\n")
		if err2 != nil {
			color.Red("Error logging the error to std.Error:")
			color.Red("%s\n", err2)
			color.Red("Original error:")
			color.Red("%s\n", err)
		}
	}
}

// LogError logs errors
func LogError(cliConfig schema.CliConfiguration, err error) {
	if err != nil {
		color.Red("%s\n", err)
	}
}

// LogInfo logs the provided info message
func LogInfo(message string) {
	color.Cyan("%s", message)
}

// LogInfoVerbose checks the log level and logs the provided info message
func LogInfoVerbose(verbose bool, message string) {
	if verbose {
		LogInfo(message)
	}
}

// LogMessage logs the provided message to the console
func LogMessage(message string) {
	fmt.Println(message)
}

// LogMessageVerbose checks the log level and logs the provided message to the console
func LogMessageVerbose(verbose bool, message string) {
	if verbose {
		LogMessage(message)
	}
}
