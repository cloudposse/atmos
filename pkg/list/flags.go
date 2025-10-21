package list

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/cobra"
)

// CommonListFlags represents the common flags used across list commands.
type CommonListFlags struct {
	Query      string
	MaxColumns int
	Format     string
	Delimiter  string
	Stack      string
}

// DefaultMaxColumns is the default maximum number of columns to display.
const DefaultMaxColumns = 10

// AddCommonListFlags adds the common flags to a command.
func AddCommonListFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("query", "", "YQ query to filter values")
	cmd.PersistentFlags().Int("max-columns", DefaultMaxColumns, "Maximum number of columns to display")
	cmd.PersistentFlags().String("format", "", "Output format (table, json, yaml, csv, tsv)")
	cmd.PersistentFlags().String("delimiter", "\t", "Delimiter for csv/tsv output (default: tab for tsv, comma for csv)")
	cmd.PersistentFlags().String("stack", "", "Stack pattern to filter (supports glob patterns, e.g., '*-dev-*', 'prod-*')")
}

// GetCommonListFlags extracts the common flags from a command.
func GetCommonListFlags(cmd *cobra.Command) (*CommonListFlags, error) {
	flags := cmd.Flags()

	query, err := flags.GetString("query")
	if err != nil {
		log.Error("failed to get query flag", "error", err)
		return nil, err
	}

	maxColumns, err := flags.GetInt("max-columns")
	if err != nil {
		log.Error("failed to get max-columns flag", "error", err)
		return nil, err
	}

	format, err := flags.GetString("format")
	if err != nil {
		log.Error("failed to get format flag", "error", err)
		return nil, err
	}

	// Validate format if provided
	if err := ValidateValuesFormat(format); err != nil {
		log.Error("invalid format", "error", err)
		return nil, err
	}

	delimiter, err := flags.GetString("delimiter")
	if err != nil {
		log.Error("failed to get delimiter flag", "error", err)
		return nil, err
	}

	stack, err := flags.GetString("stack")
	if err != nil {
		log.Error("failed to get stack flag", "error", err)
		return nil, err
	}

	return &CommonListFlags{
		Query:      query,
		MaxColumns: maxColumns,
		Format:     format,
		Delimiter:  delimiter,
		Stack:      stack,
	}, nil
}
