package main

import (
	"github.com/cloudposse/atmos/cmd"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func main() {

	err := cmd.Execute()
	if err != nil {
		u.LogErrorAndExit(err)
	}
}
