package exec

import (
	"fmt"
	"os"

	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/validator"
	"gopkg.in/yaml.v3"
)

func ExecuteAtmosValidateSchemaCmd(yamlSource string, customSchema string) error {
	if yamlSource == "" {
		yamlSource = "atmos.yaml"
	}
	if customSchema == "" {
		yamlData, err := validator.GetData(yamlSource)
		if err != nil {
			return err
		}
		yamlGenericData := make(map[string]interface{})
		yaml.Unmarshal(yamlData, yamlGenericData)
		if val, ok := yamlGenericData["schema"]; ok && val != "" {
			if scheama, ok := val.(string); ok {
				customSchema = scheama
			}
		}
		if customSchema == "" && yamlSource == "atmos.yaml" {
			customSchema = "atmos://schema"
		}
	}
	if customSchema == "" {
		return fmt.Errorf("schema not found for %v file", yamlSource)
	}
	validationErrors, err := validator.ValidateYAMLSchema(customSchema, yamlSource)
	if err != nil {
		return err
	}
	if len(validationErrors) == 0 {
		u.LogInfo(fmt.Sprintf("No Validation Errors found in %v using schema %v", yamlSource, customSchema))
	} else {
		u.LogError(fmt.Errorf("Invalid YAML:"))
		for _, err := range validationErrors {
			u.LogError(fmt.Errorf("- %s\n", err))
		}
		os.Exit(1)
	}
	return nil
}
