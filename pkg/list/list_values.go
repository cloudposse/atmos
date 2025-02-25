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

// Error variables for list_values package
var (
	ErrInvalidStackPattern = errors.New("invalid stack pattern")
)

// FilterAndListValues filters and lists component values across stacks
func FilterAndListValues(stacksMap map[string]interface{}, component, query string, includeAbstract bool, maxColumns int, formatStr, delimiter string, stackPattern string) (string, error) {
	// Set default format if not specified
	if formatStr == "" {
		formatStr = string(format.FormatTable)
	}

	if err := format.ValidateFormat(formatStr); err != nil {
		return "", err
	}

	extractor := values.NewDefaultExtractor()

	extractedValues, err := extractor.ExtractStackValues(stacksMap, component, includeAbstract)
	if err != nil {
		return "", err
	}

	// Filter by stack pattern if provided
	if stackPattern != "" {
		filteredValues := make(map[string]interface{})
		for stackName, value := range extractedValues {
			matched, err := filepath.Match(stackPattern, stackName)
			if err != nil {
				return "", errors.Errorf("invalid stack pattern '%s'", stackPattern)
			}
			if matched {
				filteredValues[stackName] = value
			}
		}
		extractedValues = filteredValues
	}

	// Apply max columns limit to filtered values
	if maxColumns > 0 {
		limitedValues := make(map[string]interface{})
		var sortedKeys []string
		for stackName := range extractedValues {
			sortedKeys = append(sortedKeys, stackName)
		}
		sort.Strings(sortedKeys)
		for i, stackName := range sortedKeys {
			if i >= maxColumns {
				break
			}
			limitedValues[stackName] = extractedValues[stackName]
		}
		extractedValues = limitedValues
	}

	// Apply query to values
	queriedValues, err := extractor.ApplyValueQuery(extractedValues, query)
	if err != nil {
		return "", err
	}

	// Create formatter
	formatter, err := format.NewFormatter(format.Format(formatStr))
	if err != nil {
		return "", err
	}

	// Format output
	options := format.FormatOptions{
		MaxColumns: maxColumns,
		Delimiter:  delimiter,
		TTY:        term.IsTTYSupportForStdout(),
		Format:     format.Format(formatStr),
	}

	output, err := formatter.Format(queriedValues, options)
	if err != nil {
		return "", err
	}

	return output, nil
}

// IsNoValuesFoundError checks if an error is a NoValuesFoundError
func IsNoValuesFoundError(err error) bool {
	_, ok := err.(*listerrors.NoValuesFoundError)
	return ok
}

// ValidateFormat validates the output format
func ValidateValuesFormat(formatStr string) error {
	return format.ValidateFormat(formatStr)
}
