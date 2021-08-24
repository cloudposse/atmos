package main

import (
	"atmos/cmd"
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
