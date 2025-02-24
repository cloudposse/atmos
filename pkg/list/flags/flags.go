package flags

import (
	"github.com/spf13/cobra"
)

// CommonFlags contains common flags for list commands.
type CommonFlags struct {
	Format     string
	MaxColumns int
	Delimiter  string
	Stack      string
	Query      string
}

// AddCommonListFlags adds common flags to list commands.
func AddCommonListFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("format", "", "Output format: table, json, yaml, csv, tsv")
	cmd.PersistentFlags().Int("max-columns", 0, "Maximum number of columns to display")
	cmd.PersistentFlags().String("delimiter", "", "Delimiter for CSV/TSV output")
	cmd.PersistentFlags().String("stack", "", "Stack pattern to filter by")
	cmd.PersistentFlags().String("query", "", "Query to filter values (e.g., .vars.region)")
}

// GetCommonListFlags gets common flags from a command.
func GetCommonListFlags(cmd *cobra.Command) (*CommonFlags, error) {
	format, _ := cmd.Flags().GetString("format")
	maxColumns, _ := cmd.Flags().GetInt("max-columns")
	delimiter, _ := cmd.Flags().GetString("delimiter")
	stack, _ := cmd.Flags().GetString("stack")
	query, _ := cmd.Flags().GetString("query")

	return &CommonFlags{
		Format:     format,
		MaxColumns: maxColumns,
		Delimiter:  delimiter,
		Stack:      stack,
		Query:      query,
	}, nil
}
