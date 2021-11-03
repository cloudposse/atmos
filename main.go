package main

import (
	"os"

	"github.com/cloudposse/atmos/cmd"
	"github.com/fatih/color"
)

func main() {
	err := cmd.Execute()
	if err != nil {
		color.Red("%s", err)
		os.Exit(1)
	}
}
