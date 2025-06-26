package cmd

import u "github.com/cloudposse/atmos/pkg/utils"

func CheckErrorAndExit(err error, title string, suggestion string) {
	if err != nil {
		u.PrintErrorMarkdownAndExit(title, err, suggestion)
	}
}
