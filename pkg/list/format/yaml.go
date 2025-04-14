package format

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/utils"
)

// Format implements the Formatter interface for YAMLFormatter.
func (f *YAMLFormatter) Format(data map[string]interface{}, options FormatOptions) (string, error) {
	yamlBytes, err := utils.ConvertToYAML(data)
	if err != nil {
		return "", fmt.Errorf("error formatting YAML output: %w", err)
	}
	return yamlBytes, nil
}
