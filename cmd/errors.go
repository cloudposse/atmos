package cmd

import u "github.com/cloudposse/atmos/pkg/utils"

func checkErrorAndExit(err error) {
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}
}
