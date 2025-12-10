package stack

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// convertCmd converts stack configuration files between formats.
var convertCmd = &cobra.Command{
	Use:   "convert <input-file>",
	Short: "Convert stack configuration between formats",
	Long: `Convert stack configuration files between YAML, JSON, and HCL formats.

All formats parse to the same internal data structure, enabling seamless
bidirectional conversion between any supported format.

Examples:
  # Convert YAML to HCL (output to stdout)
  atmos stack convert stacks/prod.yaml --to hcl

  # Convert YAML to HCL (output to file)
  atmos stack convert stacks/prod.yaml --to hcl --output prod.hcl

  # Convert HCL to YAML
  atmos stack convert stacks/prod.hcl --to yaml

  # Convert JSON to YAML with output file
  atmos stack convert stacks/prod.json --to yaml --output prod.yaml

  # Preview conversion without writing (dry-run)
  atmos stack convert stacks/prod.yaml --to hcl --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flag values.
		toFormat, _ := cmd.Flags().GetString("to")
		outputPath, _ := cmd.Flags().GetString("output")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Load atmos configuration.
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			// Config is optional for this command - we just need file conversion.
			atmosConfig = schema.AtmosConfiguration{}
		}

		return e.ExecuteStackConvert(&atmosConfig, args[0], toFormat, outputPath, dryRun)
	},
}

func init() {
	convertCmd.Flags().StringP("to", "t", "", "Target format: yaml, json, or hcl (required)")
	convertCmd.Flags().StringP("output", "o", "", "Output file path (optional, defaults to stdout)")
	convertCmd.Flags().BoolP("dry-run", "n", false, "Preview conversion without writing")

	// Mark --to as required.
	_ = convertCmd.MarkFlagRequired("to")
}
