package exec

import (
	"fmt"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/filematch"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validator"
)

var ErrInvalidYAML = fmt.Errorf("invalid YAML")

type ErrInvalidPattern struct {
	Pattern string
	err     error
}

func (e ErrInvalidPattern) Error() string {
	return fmt.Sprintf("invalid pattern %q: %v", e.Pattern, e.err)
}

type atmosValidatorExecuter struct {
	validator      validator.Validator
	fileDownloader downloader.FileDownloader
	fileMatcher    filematch.FileMatcher
	atmosConfig    *schema.AtmosConfiguration
}

func NewAtmosValidatorExecuter(atmosConfig *schema.AtmosConfiguration) *atmosValidatorExecuter {
	fileDownloader := downloader.NewGoGetterDownloader(atmosConfig)
	return &atmosValidatorExecuter{
		validator:      validator.NewYAMLSchemaValidator(atmosConfig),
		fileDownloader: fileDownloader,
		fileMatcher:    filematch.NewGlobMatcher(),
		atmosConfig:    atmosConfig,
	}
}

func (av *atmosValidatorExecuter) ExecuteAtmosValidateSchemaCmd(sourceKey string, customSchema string) error {
	validationSchemaWithFiles, err := av.buildValidationSchema(sourceKey, customSchema)
	if err != nil {
		return err
	}

	totalErrCount, err := av.validateSchemas(validationSchemaWithFiles)
	if err != nil {
		return err
	}

	if totalErrCount > 0 {
		return ErrInvalidYAML
	}
	return nil
}

func (av *atmosValidatorExecuter) buildValidationSchema(sourceKey, customSchema string) (map[string][]string, error) {
	validationSchemaWithFiles := make(map[string][]string)
	for k := range av.atmosConfig.Schemas {
		if av.shouldSkipSchema(k, sourceKey) {
			continue
		}

		schemaValue := av.prepareSchemaValue(k, sourceKey, customSchema)
		if schemaValue.Schema == "" {
			continue
		}

		files, err := av.fileMatcher.MatchFiles(schemaValue.Matches)
		if err != nil {
			return nil, err
		}
		log.Debug("Files matched", "schema", schemaValue.Schema, "matcher", schemaValue.Matches, "filesMatched", files)
		validationSchemaWithFiles[schemaValue.Schema] = files
	}
	return validationSchemaWithFiles, nil
}

func (av *atmosValidatorExecuter) shouldSkipSchema(k, sourceKey string) bool {
	return (sourceKey != "" && sourceKey != k) || k == "cue" || k == "opa" || k == "jsonschema"
}

func (av *atmosValidatorExecuter) prepareSchemaValue(k, sourceKey, customSchema string) schema.SchemaRegistry {
	value := av.atmosConfig.GetSchemaRegistry(k)
	if sourceKey != "" && customSchema != "" {
		value.Schema = customSchema
	}
	if k == "atmos" {
		if value.Schema == "" && value.Manifest == "" {
			value.Schema = "atmos://schema/atmos/manifest/1.0"
		} else if value.Schema == "" && value.Manifest != "" {
			value.Schema = value.Manifest
		}
		if len(value.Matches) == 0 {
			value.Matches = []string{"atmos.yaml", "atmos.yml"}
		}
	}
	return value
}

func (av *atmosValidatorExecuter) validateSchemas(schemas map[string][]string) (uint, error) {
	totalErrCount := uint(0)
	for k, files := range schemas {
		errCount, err := av.printValidation(k, files)
		if err != nil {
			return 0, err
		}
		totalErrCount += errCount
	}
	return totalErrCount, nil
}

func (av *atmosValidatorExecuter) printValidation(schema string, files []string) (uint, error) {
	count := uint(0)
	for _, file := range files {
		log.Debug("validating", "schema", schema, "file", file)
		validationErrors, err := av.validator.ValidateYAMLSchema(schema, file)
		if err != nil {
			return count, err
		}
		if len(validationErrors) == 0 {
			log.Info("No Validation Errors", "file", file, "schema", schema)
			continue
		}
		log.Error("Invalid YAML", "file", file)
		for _, err := range validationErrors {
			log.Error("", "file", file, "field", err.Field(), "type", err.Type(), "description", err.Description())
			count++
		}
	}
	return count, nil
}
