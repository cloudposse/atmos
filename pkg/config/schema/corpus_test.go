package configschema

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	stjsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validator"
)

// TestGeneratedSchemaCompiles asserts the generated document is a valid JSON
// Schema under a strict draft 2020-12 compiler.
func TestGeneratedSchemaCompiles(t *testing.T) {
	compiler := stjsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("atmos-config.json", bytes.NewReader(generatedSchema(t))))

	_, err := compiler.Compile("atmos-config.json")
	require.NoError(t, err, "the generated atmos.yaml schema must compile as draft 2020-12")
}

// TestGeneratedSchemaAcceptsRealConfigs is the over-strictness backstop: every
// atmos.yaml in examples/ — plus profile and atmos.d fragments — must validate
// against the generated schema through the same engine `atmos validate schema`
// uses (which stringifies YAML function tags like `!include`).
func TestGeneratedSchemaAcceptsRealConfigs(t *testing.T) {
	files := corpusFiles(t)
	require.Positive(t, len(files), "no atmos.yaml corpus files found under examples/; the corpus scan is misconfigured")

	schemaJSON := string(generatedSchema(t))
	yamlValidator := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})

	for _, file := range files {
		validationErrors, err := yamlValidator.ValidateYAMLSchema(schemaJSON, file)
		require.NoError(t, err, "validating %s", file)
		for _, validationError := range validationErrors {
			assert.Failf(t, "real config rejected by the generated schema",
				"%s: field %q: %s", file, validationError.Field(), validationError.Description())
		}
	}
}

// corpusFiles returns every atmos.yaml/atmos.yml under examples/, plus profile
// and atmos.d fragments (which are partial atmos.yaml documents and must
// validate standalone). Paths are absolute so the scan is CWD-independent.
func corpusFiles(t *testing.T) []string {
	t.Helper()

	repoRoot, err := RepoRoot()
	require.NoError(t, err)
	examplesDir := filepath.Join(repoRoot, "examples")

	var files []string
	err = filepath.WalkDir(examplesDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if isCorpusFile(path, entry.Name()) {
			files = append(files, path)
		}
		return nil
	})
	require.NoError(t, err)
	return files
}

// isCorpusFile reports whether a file is an atmos.yaml document or a config
// fragment (profile or atmos.d file) that must validate against the schema.
func isCorpusFile(path, name string) bool {
	if name == "atmos.yaml" || name == "atmos.yml" {
		return true
	}
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		return false
	}
	slashed := filepath.ToSlash(path)
	return strings.Contains(slashed, "/profiles/") || strings.Contains(slashed, "/atmos.d/") || strings.Contains(slashed, "/.atmos.d/")
}
