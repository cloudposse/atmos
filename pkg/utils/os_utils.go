package utils

import (
	"fmt"
	g "github.com/cloudposse/atmos/pkg/globals"
	"github.com/fatih/color"
	"os"
)

// PrintErrorToStdErrorAndExit prints errors to std.Error and exits with an error code
func PrintErrorToStdErrorAndExit(err error) {
	if err != nil {
		PrintErrorToStdError(err)
		os.Exit(1)
	}
}

// PrintErrorToStdError prints errors to std.Error
func PrintErrorToStdError(err error) {
	if err != nil {
		c := color.New(color.FgRed)
		_, err2 := c.Fprintln(color.Error, err.Error()+"\n")
		if err2 != nil {
			fmt.Println("Error sending the error message to std.Error:")
			PrintError(err2)
			fmt.Println("Original error message:")
			PrintError(err)
		}
	}
}

// PrintError prints errors to std.Output
func PrintError(err error) {
	if err != nil {
		color.Red("%s\n", err)
	}
}

// PrintErrorVerbose checks the log level and prints errors to std.Output
func PrintErrorVerbose(err error) {
	if g.LogVerbose {
		PrintError(err)
	}
}

// PrintInfo prints the provided message
func PrintInfo(message string) {
	color.Cyan("%s", message)
}

// PrintInfoVerbose checks the log level and prints the provided message
func PrintInfoVerbose(message string) {
	if g.LogVerbose {
		PrintInfo(message)
	}
}
