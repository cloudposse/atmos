package utils

import (
	"fmt"
	"github.com/fatih/color"
	"os"
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
			LogMessage("Error sending the error message to std.Error:")
			LogError(err2)
			LogMessage("Original error message:")
			LogError(err)
		}
	}
}

// LogError logs errors
func LogError(err error) {
	if err != nil {
		color.Red("%s\n", err)
	}
}

// LogErrorVerbose checks the log level and logs errors
func LogErrorVerbose(verbose bool, err error) {
	if verbose {
		LogError(err)
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
