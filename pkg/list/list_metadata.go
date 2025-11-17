package list

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Default columns for list metadata if not specified in atmos.yaml.
var defaultMetadataColumns = []column.Config{
	{Name: "Stack", Value: "{{ .stack }}"},
	{Name: "Component", Value: "{{ .component }}"},
	{Name: "Type", Value: "{{ .type }}"},
	{Name: "Enabled", Value: "{{ .enabled }}"},
	{Name: "Locked", Value: "{{ .locked }}"},
	{Name: "Component (base)", Value: "{{ .component_base }}"},
	{Name: "Inherits", Value: "{{ .inherits }}"},
	{Name: "Description", Value: "{{ .description }}"},
}

// getMetadataColumns returns column configuration from atmos.yaml or defaults.
func getMetadataColumns(atmosConfig *schema.AtmosConfiguration) []column.Config {
	// Check if custom columns are configured in atmos.yaml.
	if len(atmosConfig.Components.List.Columns) > 0 {
		columns := make([]column.Config, len(atmosConfig.Components.List.Columns))
		for i, col := range atmosConfig.Components.List.Columns {
			columns[i] = column.Config{
				Name:  col.Name,
				Value: col.Value,
			}
		}
		return columns
	}

	// Return default columns.
	return defaultMetadataColumns
}

// ExecuteListMetadataCmd executes the list metadata command using the renderer pipeline.
func ExecuteListMetadataCmd(info *schema.ConfigAndStacksInfo, cmd *cobra.Command, args []string) error {
	// Initialize CLI config.
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToInitConfig, err)
	}

	// Get flags.
	formatFlag, _ := cmd.Flags().GetString("format")

	// Process instances (same as list instances, but we'll extract metadata).
	instances, err := processInstances(&atmosConfig)
	if err != nil {
		return errors.Join(errUtils.ErrProcessInstances, err)
	}

	// Extract metadata into renderer-compatible format.
	data := ExtractMetadata(instances)

	// Get column configuration.
	columns := getMetadataColumns(&atmosConfig)

	// Create column selector.
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("failed to create column selector: %w", err)
	}

	// Create renderer.
	r := renderer.New(nil, selector, nil, format.Format(formatFlag))

	// Render output.
	if err := r.Render(data); err != nil {
		return fmt.Errorf("failed to render metadata: %w", err)
	}

	return nil
}
