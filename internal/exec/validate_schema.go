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
	fileMatcher    filematch.FileMatcherInterface
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
	validationSchemaWithFiles := make(map[string][]string)
	for k := range av.atmosConfig.Schemas {
		if sourceKey != "" && sourceKey != k {
			continue
		}
		// We ignore these because in backward compatibility they are structured different.
		if k == "cue" || k == "opa" || k == "jsonschema" {
			continue
		}
		log.Debug("Collecting", "schemaName", k)
		value := av.atmosConfig.GetSchemaRegistry(k)
		if sourceKey != "" && customSchema != "" {
			value.Schema = customSchema
		}
		if k == "atmos" && value.Schema == "" && value.Manifest == "" {
			value.Schema = "atmos://schema/atmos/manifest/1.0"
		}
		if k == "atmos" && value.Schema == "" && value.Manifest != "" {
			value.Schema = value.Manifest
		}
		if value.Schema == "" {
			continue
		}
		if k == "atmos" && len(value.Matches) == 0 {
			value.Matches = []string{"atmos.yaml", "atmos.yml"}
		}

		files, err := av.fileMatcher.MatchFiles(value.Matches)
		if err != nil {
			return err
		}
		log.Debug("Files matched", "schema", value.Schema, "matcher", value.Matches, "filesMatched", files)
		validationSchemaWithFiles[value.Schema] = files
	}
	totalErrCount := uint(0)
	for k := range validationSchemaWithFiles {
		errCount, err := av.printValidation(k, validationSchemaWithFiles[k])
		if err != nil {
			return err
		}
		totalErrCount += errCount
	}
	if totalErrCount > 0 {
		return ErrInvalidYAML
	}
	return nil
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
