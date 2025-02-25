package list

import (
	"path/filepath"
	"sort"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/values"
	"github.com/pkg/errors"
)

// Error variables for list_values package.
var (
	ErrInvalidStackPattern = errors.New("invalid stack pattern")
)

// FilterOptions contains the options for filtering and listing component values.
type FilterOptions struct {
	Component       string
	Query           string
	IncludeAbstract bool
	MaxColumns      int
	FormatStr       string
	Delimiter       string
	StackPattern    string
}

// FilterAndListValues filters and lists component values across stacks.
func FilterAndListValues(stacksMap map[string]interface{}, options *FilterOptions) (string, error) {
	// Set default format if not specified
	if options.FormatStr == "" {
		options.FormatStr = string(format.FormatTable)
	}

	if err := format.ValidateFormat(options.FormatStr); err != nil {
		return "", err
	}

	// Extract stack values
	extractedValues, err := extractComponentValues(stacksMap, options.Component, options.IncludeAbstract)
	if err != nil {
		return "", err
	}

	// Apply filters
	filteredValues, err := applyFilters(extractedValues, options.StackPattern, options.MaxColumns)
	if err != nil {
		return "", err
	}

	// Apply query to values
	queriedValues, err := applyQuery(filteredValues, options.Query)
	if err != nil {
		return "", err
	}

	// Format the output
	return formatOutput(queriedValues, options.FormatStr, options.Delimiter, options.MaxColumns)
}

// extractComponentValues extracts the component values from all stacks.
func extractComponentValues(stacksMap map[string]interface{}, component string, includeAbstract bool) (map[string]interface{}, error) {
	extractor := values.NewDefaultExtractor()
	return extractor.ExtractStackValues(stacksMap, component, includeAbstract)
}

// applyFilters applies stack pattern and column limits to the values.
func applyFilters(extractedValues map[string]interface{}, stackPattern string, maxColumns int) (map[string]interface{}, error) {
	// Apply stack pattern filter
	filteredByPattern, err := filterByStackPattern(extractedValues, stackPattern)
	if err != nil {
		return nil, err
	}

	// Apply column limit
	return limitColumns(filteredByPattern, maxColumns), nil
}

// filterByStackPattern filters values by a glob pattern.
func filterByStackPattern(values map[string]interface{}, pattern string) (map[string]interface{}, error) {
	if pattern == "" {
		return values, nil
	}

	filteredValues := make(map[string]interface{})
	for stackName, value := range values {
		matched, err := filepath.Match(pattern, stackName)
		if err != nil {
			return nil, errors.Errorf("invalid stack pattern '%s'", pattern)
		}
		if matched {
			filteredValues[stackName] = value
		}
	}
	return filteredValues, nil
}

// limitColumns limits the number of columns in the output.
func limitColumns(values map[string]interface{}, maxColumns int) map[string]interface{} {
	if maxColumns <= 0 {
		return values
	}

	limitedValues := make(map[string]interface{})
	var sortedKeys []string
	for stackName := range values {
		sortedKeys = append(sortedKeys, stackName)
	}
	sort.Strings(sortedKeys)

	count := len(sortedKeys)
	if count > maxColumns {
		count = maxColumns
	}

	for i := 0; i < count; i++ {
		limitedValues[sortedKeys[i]] = values[sortedKeys[i]]
	}
	return limitedValues
}

// applyQuery applies a query to the filtered values.
func applyQuery(filteredValues map[string]interface{}, query string) (map[string]interface{}, error) {
	extractor := values.NewDefaultExtractor()
	return extractor.ApplyValueQuery(filteredValues, query)
}

// formatOutput formats the output based on the specified format.
func formatOutput(values map[string]interface{}, formatStr, delimiter string, maxColumns int) (string, error) {
	formatter, err := format.NewFormatter(format.Format(formatStr))
	if err != nil {
		return "", err
	}

	options := format.FormatOptions{
		MaxColumns: maxColumns,
		Delimiter:  delimiter,
		TTY:        term.IsTTYSupportForStdout(),
		Format:     format.Format(formatStr),
	}

	return formatter.Format(values, options)
}

// IsNoValuesFoundError checks if an error is a NoValuesFoundError.
func IsNoValuesFoundError(err error) bool {
	_, ok := err.(*listerrors.NoValuesFoundError)
	return ok
}

// ValidateFormat validates the output format.
func ValidateValuesFormat(formatStr string) error {
	return format.ValidateFormat(formatStr)
}
