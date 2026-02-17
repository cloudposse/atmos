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
	Content    string
	Suggestion string
}

const (
	doubleDashHint string = "Use double dashes to separate Atmos-specific options from native arguments and flags for the command."
	stackHint      string = "The `stack` flag specifies the environment or configuration set for deployment in Atmos CLI."
	componentHint  string = "The `component` flag specifies the name of the component to be managed or deployed in Atmos CLI."
)

var examples = map[string]ExampleContent{
	"atmos_terraform": {
		Suggestion: "https://atmos.tools/cli/commands/terraform/usage",
	},
	"atmos_terraform_plan": {
		// TODO: We should update this once we have a page for terraform plan
		Suggestion: "https://atmos.tools/cli/commands/terraform/usage",
	},
	"atmos_terraform_apply": {
		// TODO: We should update this once we have a page for terraform apply
		Suggestion: "https://atmos.tools/cli/commands/terraform/usage",
	},
	"atmos_workflow": {
		Suggestion: "https://atmos.tools/cli/commands/workflow/",
	},
	"atmos_aws_eks_update_kubeconfig": {
		Suggestion: "https://atmos.tools/cli/commands/aws/eks-update-kubeconfig",
	},
	"atmos_ai": {
		Suggestion: "https://atmos.tools/cli/commands/ai/",
	},
	"atmos_ai_chat": {
		Suggestion: "https://atmos.tools/cli/commands/ai/chat",
	},
	"atmos_ai_ask": {
		Suggestion: "https://atmos.tools/cli/commands/ai/ask",
	},
	"atmos_ai_help": {
		Suggestion: "https://atmos.tools/cli/commands/ai/help",
	},
}

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
			if val, ok := examples[removeExtension(file.Name())]; ok {
				examples[removeExtension(file.Name())] = ExampleContent{
					Content:    string(data),
					Suggestion: val.Suggestion,
				}
			} else {
				examples[removeExtension(file.Name())] = ExampleContent{
					Content: string(data),
				}
			}
		}
	}
}

func removeExtension(filename string) string {
	return strings.TrimSuffix(strings.TrimSuffix(filename, filepath.Ext(filename)), "_usage")
}
