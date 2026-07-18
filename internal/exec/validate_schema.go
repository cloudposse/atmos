package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/filematch"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
	"gopkg.in/yaml.v3"
)

var ErrInvalidYAML = fmt.Errorf("invalid YAML")

const (
	// The well-known `schemas:` key that validates atmos.yaml (and its
	// fragments) against the embedded schema generated from the Atmos
	// configuration structs (see pkg/config/schema). It is seeded by default so
	// `atmos validate schema` covers atmos.yaml with zero configuration; a
	// `schemas.config` entry in atmos.yaml overrides it.
	builtinConfigSchemaKey = "config"

	// The embedded generated atmos.yaml JSON Schema — the same document
	// `atmos config schema` prints.
	configSchemaSource = "atmos://schema/atmos/config/1.0"
)

// builtinConfigSchemaMatches returns the project-local files the config loader
// reads: atmos.yaml (including hidden variants), atmos.d fragments, and
// project-local profile directories. Profile files and atmos.d fragments are
// partial configs; the schema has no required fields, so they validate
// standalone. Fragment directories are optional and the glob matcher fails hard
// on missing directories, so only existing ones are included.
func builtinConfigSchemaMatches() []string {
	matches := []string{
		"atmos.yaml",
		"atmos.yml",
		".atmos.yaml",
		".atmos.yml",
	}
	fragmentDirs := []string{
		"atmos.d",
		".atmos.d",
		"profiles",
		filepath.Join(".atmos", "profiles"),
	}
	for _, dir := range fragmentDirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			matches = append(
				matches,
				filepath.Join(dir, "**", "*.yaml"),
				filepath.Join(dir, "**", "*.yml"),
			)
		}
	}
	return matches
}

type ErrInvalidPattern struct {
	Pattern string
	err     error
}

func (e ErrInvalidPattern) Error() string {
	return fmt.Sprintf("invalid pattern %q: %v", e.Pattern, e.err)
}

type atmosValidatorExecutor struct {
	validator      validator.Validator
	fileDownloader downloader.FileDownloader
	fileMatcher    filematch.FileMatcher
	atmosConfig    *schema.AtmosConfiguration
}

func NewAtmosValidatorExecutor(atmosConfig *schema.AtmosConfiguration) *atmosValidatorExecutor {
	defer perf.Track(atmosConfig, "exec.NewAtmosValidatorExecutor")()

	fileDownloader := downloader.NewGoGetterDownloader(atmosConfig)
	return &atmosValidatorExecutor{
		validator:      validator.NewYAMLSchemaValidator(atmosConfig),
		fileDownloader: fileDownloader,
		fileMatcher:    filematch.NewGlobMatcher(),
		atmosConfig:    atmosConfig,
	}
}

func (av *atmosValidatorExecutor) ExecuteAtmosValidateSchemaCmd(sourceKey string, customSchema string) error {
	return av.executeAtmosValidateSchemaCmd(sourceKey, customSchema, nil)
}

// ExecuteAtmosValidateSchemaCmdForFiles validates only the supplied files.
// A nil file list retains the established behavior of validating every match.
func (av *atmosValidatorExecutor) ExecuteAtmosValidateSchemaCmdForFiles(sourceKey string, customSchema string, files []string) error {
	return av.executeAtmosValidateSchemaCmd(sourceKey, customSchema, files)
}

func (av *atmosValidatorExecutor) executeAtmosValidateSchemaCmd(sourceKey string, customSchema string, files []string) error {
	defer perf.Track(nil, "exec.ExecuteAtmosValidateSchemaCmd")()

	var totalErrCount uint

	err := spinner.ExecWithSpinner(
		"Validating YAML schemas...",
		"All schemas validated successfully",
		func() error {
			validationSchemaWithFiles, err := av.buildValidationSchema(sourceKey, customSchema)
			if err != nil {
				return err
			}
			validationSchemaWithFiles = filterValidationSchemaFiles(validationSchemaWithFiles, files)

			totalErrCount, err = av.validateSchemas(validationSchemaWithFiles)
			if err != nil {
				return err
			}

			if totalErrCount > 0 {
				return ErrInvalidYAML
			}
			return nil
		},
	)

	return err
}

// ValidateAtmosSchemaReport collects schema findings without writing terminal
// output. It is used by the rich renderer; the established spinner/log path
// above remains the default text behavior.
func (av *atmosValidatorExecutor) ValidateAtmosSchemaReport(sourceKey string, customSchema string) (validation.Report, error) {
	return av.validateAtmosSchemaReport(sourceKey, customSchema, nil)
}

// ValidateAtmosSchemaReportForFiles collects findings for only the supplied files.
// A nil file list retains the established behavior of validating every match.
func (av *atmosValidatorExecutor) ValidateAtmosSchemaReportForFiles(sourceKey string, customSchema string, files []string) (validation.Report, error) {
	return av.validateAtmosSchemaReport(sourceKey, customSchema, files)
}

func (av *atmosValidatorExecutor) validateAtmosSchemaReport(sourceKey string, customSchema string, files []string) (validation.Report, error) {
	schemas, err := av.buildValidationSchema(sourceKey, customSchema)
	if err != nil {
		return validation.Report{}, err
	}
	schemas = filterValidationSchemaFiles(schemas, files)
	report := validation.Report{}
	for schemaSource, files := range schemas {
		for _, file := range files {
			errors, validateErr := av.validator.ValidateYAMLSchema(schemaSource, file)
			if validateErr != nil {
				return validation.Report{}, validateErr
			}
			report.FilesChecked++
			positions := schemaFilePositions(file)
			for _, resultError := range errors {
				position := u.GetYAMLPosition(positions, resultError.Field())
				report.Diagnostics = append(report.Diagnostics, validation.Diagnostic{
					Source:   "schema",
					RuleID:   resultError.Type(),
					Severity: validation.SeverityError,
					Message:  resultError.Description(),
					File:     displayPath(file),
					Line:     position.Line,
					Column:   position.Column,
				})
			}
		}
	}
	return report, nil
}

func filterValidationSchemaFiles(schemas map[string][]string, files []string) map[string][]string {
	if files == nil {
		return schemas
	}

	selected := make(map[string]struct{}, len(files))
	for _, file := range files {
		absolute, err := filepath.Abs(file)
		if err == nil {
			selected[filepath.Clean(absolute)] = struct{}{}
		}
	}
	filtered := make(map[string][]string, len(schemas))
	for schemaSource, schemaFiles := range schemas {
		for _, file := range schemaFiles {
			absolute, err := filepath.Abs(file)
			if err == nil {
				if _, ok := selected[filepath.Clean(absolute)]; ok {
					filtered[schemaSource] = append(filtered[schemaSource], file)
				}
			}
		}
	}
	return filtered
}

func schemaFilePositions(file string) u.PositionMap {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var node yaml.Node
	if err := yaml.Unmarshal(content, &node); err != nil {
		return nil
	}
	return u.ExtractYAMLPositions(&node, true)
}

// schemaKeys returns the configured `schemas:` keys plus the built-in config
// key when the user has not overridden it, so atmos.yaml is validated by
// default.
func (av *atmosValidatorExecutor) schemaKeys() []string {
	keys := make([]string, 0, len(av.atmosConfig.Schemas)+1)
	for k := range av.atmosConfig.Schemas {
		keys = append(keys, k)
	}
	if _, configured := av.atmosConfig.Schemas[builtinConfigSchemaKey]; !configured {
		keys = append(keys, builtinConfigSchemaKey)
	}
	return keys
}

func (av *atmosValidatorExecutor) buildValidationSchema(sourceKey, customSchema string) (map[string][]string, error) {
	validationSchemaWithFiles := make(map[string][]string)
	log.Debug("Building validation schema with files", "sourceKey", sourceKey, "customSchema", customSchema, "schemas", av.atmosConfig.Schemas)
	for _, k := range av.schemaKeys() {
		if av.shouldSkipSchema(k, sourceKey) {
			log.Debug("Skipping schema", "key", k, "sourceKey", sourceKey)
			continue
		}

		schemaValue := av.prepareSchemaValue(k, sourceKey, customSchema)
		if schemaValue.Schema == "" {
			log.Debug("Skipping schema with empty schema", "key", k, "sourceKey", sourceKey, "schemaValue", schemaValue)
			continue
		}

		files, err := av.fileMatcher.MatchFiles(schemaValue.Matches)
		if err != nil {
			return nil, err
		}
		log.Debug("Files matched", "schema", schemaValue.Schema, "matcher", schemaValue.Matches, "filesMatched", files)
		validationSchemaWithFiles[schemaValue.Schema] = files
	}
	log.Debug("Validation schema with files", "validationSchemaWithFiles", validationSchemaWithFiles)
	return validationSchemaWithFiles, nil
}

func (av *atmosValidatorExecutor) shouldSkipSchema(k, sourceKey string) bool {
	return (sourceKey != "" && sourceKey != k) || k == "cue" || k == "opa" || k == "jsonschema"
}

func (av *atmosValidatorExecutor) prepareSchemaValue(k, sourceKey, customSchema string) schema.SchemaRegistry {
	value := av.atmosConfig.GetSchemaRegistry(k)
	if sourceKey != "" && customSchema != "" {
		value.Schema = customSchema
	}
	switch {
	case value.Schema == "" && value.Manifest == "" && k == builtinConfigSchemaKey:
		value.Schema = configSchemaSource
	case value.Schema == "" && value.Manifest == "":
		value.Schema = fmt.Sprintf("atmos://schema/%s/manifest/1.0", k)
	case value.Schema == "" && value.Manifest != "":
		value.Schema = value.Manifest
	case customSchema != "":
		value.Schema = customSchema
	}
	if len(value.Matches) == 0 && k == builtinConfigSchemaKey {
		value.Matches = builtinConfigSchemaMatches()
	}

	return value
}

func (av *atmosValidatorExecutor) validateSchemas(schemas map[string][]string) (uint, error) {
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

// displayPath returns the file path relative to the current working directory
// when the file is inside it; user-facing output must not leak machine-specific
// absolute paths.
func displayPath(file string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return file
	}
	rel, err := filepath.Rel(cwd, file)
	if err != nil || strings.HasPrefix(rel, "..") {
		return file
	}
	return rel
}

func (av *atmosValidatorExecutor) printValidation(schema string, files []string) (uint, error) {
	count := uint(0)
	for _, file := range files {
		log.Debug("validating", "schema", schema, "file", file)
		validationErrors, err := av.validator.ValidateYAMLSchema(schema, file)
		if err != nil {
			return count, err
		}
		if len(validationErrors) == 0 {
			ui.Successf("Validated %s", displayPath(file))
			log.Debug("Schema validation passed", "file", file, "schema", schema)
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
