package configschema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validator"
)

// Compile-time guard: the test below depends on the logs.color field.
var _ = schema.Logs{Color: new(bool)}

// TestGeneratedSchemaLogsColorMatchesLoader keeps `atmos validate config` aligned
// with the logs.color values the loader accepts through strconv.ParseBool.
func TestGeneratedSchemaLogsColorMatchesLoader(t *testing.T) {
	schemaJSON := string(generatedSchema(t))
	yamlValidator := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})

	for _, tt := range []struct {
		value string
		valid bool
	}{
		{"true", true},
		{"false", true},
		{`"true"`, true},
		{`"TRUE"`, true},
		{`"f"`, true},
		{"1", true},
		{"0", true},
		{"null", true},
		{`"nope"`, false},
		{`"yes"`, false},
		{"2", false},
	} {
		t.Run(tt.value, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "atmos.yaml")
			require.NoError(t, os.WriteFile(file, []byte("logs:\n  color: "+tt.value+"\n"), 0o600))

			validationErrors, err := yamlValidator.ValidateYAMLSchema(schemaJSON, file)
			require.NoError(t, err)
			if tt.valid {
				assert.Empty(t, validationErrors, "logs.color: %s must be accepted", tt.value)
			} else {
				fields := make([]string, 0, len(validationErrors))
				for _, validationError := range validationErrors {
					fields = append(fields, validationError.Field())
				}
				assert.Contains(t, fields, "logs.color", "logs.color: %s must be rejected", tt.value)
			}
		})
	}
}
