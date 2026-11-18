package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const includeRedactSecret = "hunter2"

// TestConfigIncludeErrorsDoNotEchoCredentials covers every config-level include error path that
// formats the include argument: none may repeat a credential carried by a URL-shaped include.
func TestConfigIncludeErrorsDoNotEchoCredentials(t *testing.T) {
	const target = "https://user:" + includeRedactSecret + "@127.0.0.1:1/"
	tests := []struct {
		name   string
		source string
	}{
		{name: "dotenv include", source: "keys: !include " + target + "settings.env\n"},
		{name: "dotenv include with query", source: "keys: !include " + target + "settings.env .a\n"},
		{name: "yaml include with query", source: "keys: !include " + target + "keys.yaml .a\n"},
		{name: "yaml include with eval", source: "keys: !include " + target + "keys.yaml | eval\n"},
		{name: "raw include", source: "keys: !include.raw " + target + "keys.txt\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := artifactsTestDir(t, "artifacts:\n  verification:\n    "+tt.source)
			_, err := loadArtifactsDir(t, dir)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), includeRedactSecret)

			_, err = decodeYAMLForTestFile(t, tt.source, filepath.Join(dir, "atmos.yaml"))
			require.Error(t, err)
			assert.NotContains(t, err.Error(), includeRedactSecret)
		})
	}
}

func decodeYAMLForTestFile(t *testing.T, source, file string) (any, error) {
	t.Helper()
	node := mustParseYAML(t, source)
	return decodeNodeWithYamlFunctionsForFile(&node, file)
}

func TestLoadDotenvIncludeErrorsDoNotEchoCredentials(t *testing.T) {
	dir := t.TempDir()
	for _, value := range []string{
		"https://user:" + includeRedactSecret + "@127.0.0.1:1/settings.env",
		"https://user:" + includeRedactSecret + "@127.0.0.1:1/settings.env .a",
		"https://user:" + includeRedactSecret + "@127.0.0.1:1/settings.yaml",
	} {
		t.Run(value, func(t *testing.T) {
			_, err := loadDotenvIncludeAsYAMLNode(filepath.Join(dir, "atmos.yaml"), value)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), includeRedactSecret)
		})
	}
}
