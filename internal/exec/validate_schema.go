package exec

import (
	"fmt"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validator"
)

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

func (av *atmosValidatorExecuter) ExecuteAtmosValidateSchemaCmd(yamlSource string, customSchema string, sourceKey string) error {
	if yamlSource == "" {
		yamlSource = "atmos.yaml"
	}
	validationErrors, err := av.validator.ValidateYAMLSchema(customSchema, yamlSource, sourceKey)
	if err != nil {
		return err
	}
	if len(validationErrors) == 0 {
		log.Info("No Validation Errors", "source", yamlSource, "schema", customSchema)
		return nil
	}
	log.Error("Invalid YAML:")
	for _, err := range validationErrors {
		log.Error(fmt.Sprintf("- %s\n", err))
	}
	return ErrInvalidYAML
}
