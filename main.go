package main

import (
	"github.com/cloudposse/atmos/cmd"
	"github.com/fatih/color"
	"os"
)

func main() {
	err := cmd.Execute()
	if err != nil {
		color.Red("%s", err)
		os.Exit(1)
	}
}
