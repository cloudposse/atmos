package utils

import (
	"fmt"
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
		red := color.New(color.FgRed)
		_, err2 := red.Fprintln(color.Error, err.Error()+"\n")
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
func PrintErrorVerbose(verbose bool, err error) {
	if verbose {
		PrintError(err)
	}
}

// PrintInfo prints the provided info message
func PrintInfo(message string) {
	color.Cyan("%s", message)
}

// PrintInfoVerbose checks the log level and prints the provided info message
func PrintInfoVerbose(verbose bool, message string) {
	if verbose {
		PrintInfo(message)
	}
}

// PrintMessage prints the provided message to the console
func PrintMessage(message string) {
	fmt.Println(message)
}

// PrintMessageVerbose checks the log level and prints the provided message to the console
func PrintMessageVerbose(verbose bool, message string) {
	if verbose {
		PrintMessage(message)
	}
}
