package cmd

import (
	"embed"
	"io/fs"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

//go:embed markdown/*
var usageFiles embed.FS

type ExampleContent struct {
	Content string
}

const (
	doubleDashHint string = "Use double dashes to separate Atmos-specific options from native arguments and flags for the command."
	stackHint      string = "The `stack` flag specifies the environment or configuration set for deployment in Atmos CLI."
	componentHint  string = "The `component` flag specifies the name of the component to be managed or deployed in Atmos CLI."
)

var examples = map[string]ExampleContent{}

func init() {
	files, err := fs.ReadDir(usageFiles, "markdown")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	for _, file := range files {
		if !file.IsDir() { // Skip directories
			filename := "markdown/" + file.Name() // Full path inside embed.FS
			data, err := usageFiles.ReadFile(filename)
			if err != nil {
				continue
			}
			examples[removeExtension(file.Name())] = ExampleContent{Content: string(data)}
		}
	}
}

func removeExtension(filename string) string {
	return strings.TrimSuffix(strings.TrimSuffix(filename, filepath.Ext(filename)), "_usage")
}
