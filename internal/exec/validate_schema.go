package exec

import (
	"fmt"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validator"
)

var ErrSchemaNotFound = fmt.Errorf("schema not found")
var ErrInvalidYAML = fmt.Errorf("invalid YAML")

type atmosValidatorExecuter struct {
	validator      validator.Validator
	fileDownloader downloader.FileDownloader
}

func NewAtmosValidatorExecuter(atmosConfig *schema.AtmosConfiguration) *atmosValidatorExecuter {
	fileDownloader := downloader.NewGoGetterDownloader(atmosConfig)
	return &atmosValidatorExecuter{
		validator:      validator.NewYAMLSchemaValidator(atmosConfig),
		fileDownloader: fileDownloader,
	}
}

func (av *atmosValidatorExecuter) ExecuteAtmosValidateSchemaCmd(yamlSource string, customSchema string) error {
	if yamlSource == "" {
		yamlSource = "atmos.yaml"
	}
	if customSchema == "" {
		yamlData, err := av.fileDownloader.FetchAndAutoParse(yamlSource)
		if err != nil {
			return err
		}
		yamlGenericData := yamlData.(map[string]interface{})
		if val, ok := yamlGenericData["schema"]; ok && val != "" {
			if schema, ok := val.(string); ok {
				customSchema = schema
			}
		}
		if customSchema == "" && yamlSource == "atmos.yaml" {
			customSchema = "atmos://schema"
		}
	}
	if customSchema == "" {
		return ErrSchemaNotFound
	}
	validationErrors, err := av.validator.ValidateYAMLSchema(customSchema, yamlSource)
	if err != nil {
		return err
	}
	if len(validationErrors) == 0 {
		log.Info("No Validation Errors", "source", yamlSource, "schema", customSchema)
		return nil
	}
	log.Error(fmt.Errorf("Invalid YAML:"))
	for _, err := range validationErrors {
		fmt.Println(err)
		log.Error(fmt.Errorf("- %s\n", err))
	}
	return ErrInvalidYAML
}
