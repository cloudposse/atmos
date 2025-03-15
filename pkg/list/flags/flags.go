package flags

import (
	log "github.com/charmbracelet/log"
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

// flagGetter is an interface for getting flag values.
type flagGetter interface {
	GetString(name string) (string, error)
	GetInt(name string) (int, error)
}

// getFlags returns a flagGetter for a command.
var getFlagsFn = func(cmd *cobra.Command) flagGetter {
	return cmd.Flags()
}

// GetCommonListFlags gets common flags from a command.
func GetCommonListFlags(cmd *cobra.Command) (*CommonFlags, error) {
	flags := getFlagsFn(cmd)

	format, err := flags.GetString("format")
	if err != nil {
		log.Error("failed to retrieve format flag", "error", err)
		return nil, err
	}

	maxColumns, err := flags.GetInt("max-columns")
	if err != nil {
		log.Error("failed to retrieve max-columns flag", "error", err)
		return nil, err
	}

	delimiter, err := flags.GetString("delimiter")
	if err != nil {
		log.Error("failed to retrieve delimiter flag", "error", err)
		return nil, err
	}

	stack, err := flags.GetString("stack")
	if err != nil {
		log.Error("failed to retrieve stack flag", "error", err)
		return nil, err
	}

	query, err := flags.GetString("query")
	if err != nil {
		log.Error("failed to retrieve query flag", "error", err)
		return nil, err
	}

	return &CommonFlags{
		Format:     format,
		MaxColumns: maxColumns,
		Delimiter:  delimiter,
		Stack:      stack,
		Query:      query,
	}, nil
}
