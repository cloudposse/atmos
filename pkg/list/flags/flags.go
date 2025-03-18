package flags

import (
	"fmt"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

// Error constants for flag operations
var (
	ErrFetchingFormat     = fmt.Errorf("error fetching format flag")
	ErrFetchingMaxColumns = fmt.Errorf("error fetching max-columns flag")
	ErrFetchingDelimiter  = fmt.Errorf("error fetching delimiter flag")
	ErrFetchingStack      = fmt.Errorf("error fetching stack flag")
	ErrFetchingQuery      = fmt.Errorf("error fetching query flag")
)

// CommonFlags contains common flags for list commands.
type CommonFlags struct {
	Format     string
	MaxColumns int
	Delimiter  string
	Stack      string
	Query      string
}

// ProcessingFlags holds flags for processing templates and YAML functions.
type ProcessingFlags struct {
	Templates bool
	Functions bool
}

// AddCommonListFlags adds common flags to list commands.
func AddCommonListFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("format", "", "Output format: `table`, `json`, `yaml`, `csv`, `tsv`")
	cmd.PersistentFlags().Int("max-columns", 0, "Maximum number of columns to display")
	cmd.PersistentFlags().String("delimiter", "", "Delimiter for CSV/TSV output")
	cmd.PersistentFlags().String("stack", "", "Stack pattern to filter by")
	cmd.PersistentFlags().String("query", "", "YQ expression to filter values (e.g., `.vars.region`)")
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
		return nil, fmt.Errorf("%w: %v", ErrFetchingFormat, err)
	}

	maxColumns, err := flags.GetInt("max-columns")
	if err != nil {
		log.Error("failed to retrieve max-columns flag", "error", err)
		return nil, fmt.Errorf("%w: %v", ErrFetchingMaxColumns, err)
	}

	delimiter, err := flags.GetString("delimiter")
	if err != nil {
		log.Error("failed to retrieve delimiter flag", "error", err)
		return nil, fmt.Errorf("%w: %v", ErrFetchingDelimiter, err)
	}

	stack, err := flags.GetString("stack")
	if err != nil {
		log.Error("failed to retrieve stack flag", "error", err)
		return nil, fmt.Errorf("%w: %v", ErrFetchingStack, err)
	}

	query, err := flags.GetString("query")
	if err != nil {
		log.Error("failed to retrieve query flag", "error", err)
		return nil, fmt.Errorf("%w: %v", ErrFetchingQuery, err)
	}

	return &CommonFlags{
		Format:     format,
		MaxColumns: maxColumns,
		Delimiter:  delimiter,
		Stack:      stack,
		Query:      query,
	}, nil
}

// GetProcessingFlags gets template and function processing flags from a command.
func GetProcessingFlags(cmd *cobra.Command) *ProcessingFlags {
	processTemplates := true
	if cmd.Flags().Lookup("process-templates") != nil {
		templateVal, err := cmd.Flags().GetBool("process-templates")
		if err != nil {
			log.Warn("failed to get process-templates flag, using default",
				"default", true,
				"error", err)
		} else {
			processTemplates = templateVal
		}
	}

	processYamlFunctions := true
	if cmd.Flags().Lookup("process-functions") != nil {
		functionsVal, err := cmd.Flags().GetBool("process-functions")
		if err != nil {
			log.Warn("failed to get process-functions flag, using default",
				"default", true,
				"error", err)
		} else {
			processYamlFunctions = functionsVal
		}
	}

	return &ProcessingFlags{
		Templates: processTemplates,
		Functions: processYamlFunctions,
	}
}
