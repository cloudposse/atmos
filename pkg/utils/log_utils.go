package utils

import (
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

// LogError logs errors
func LogError(cliConfig schema.CliConfiguration, err error) {
	log(cliConfig, color.New(color.FgRed), err.Error())
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
	fileName := "/dev/stdout"

	if cliConfig.Logs.File != "" {
		fileName = cliConfig.Logs.File
	}

	f, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
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

	_, err = logColor.Fprintln(f, message)
	if err != nil {
		color.Red("%s\n", err)
	}
}
