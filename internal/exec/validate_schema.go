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
	validationManifestWithFiles := make(map[string][]string)
	for k := range av.atmosConfig.Schemas {
		if sourceKey != "" && sourceKey != k {
			continue
		}
		log.Debug("Collecting", "schemaName", k)
		value := av.atmosConfig.GetSchemaRegistry(k)
		if sourceKey != "" && customSchema != "" {
			value.Manifest = customSchema
		}
		if value.Manifest == "" {
			continue
		}
		files, err := av.fileMatcher.MatchFiles(value.Matches)
		if err != nil {
			return err
		}
		log.Debug("Files matched", "manifest", value.Manifest, "matcher", value.Matches, "filesMatched", files)
		validationManifestWithFiles[value.Manifest] = files
	}
	totalErrCount := uint(0)
	for k := range validationManifestWithFiles {
		errCount, err := av.printValidation(k, validationManifestWithFiles[k])
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

func (av *atmosValidatorExecuter) printValidation(manifest string, files []string) (uint, error) {
	count := uint(0)
	for _, file := range files {
		log.Debug("validating", "manifest", manifest, "file", file)
		validationErrors, err := av.validator.ValidateYAMLSchema(manifest, file)
		if err != nil {
			return count, err
		}
		if len(validationErrors) == 0 {
			log.Info("No Validation Errors", "file", file, "manifest", manifest)
			continue
		}
		log.Error("Invalid YAML:")
		for _, err := range validationErrors {
			log.Error(fmt.Sprintf("- %s\n", err))
			count++
		}
	}
	return count, nil
}
