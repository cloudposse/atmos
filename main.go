package main

import (
	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func main() {
	err := cmd.Execute()
	if err != nil {
		u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
	}

}
