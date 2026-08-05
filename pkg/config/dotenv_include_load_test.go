package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/config/casemap"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLoadConfigFile_RetriesDotenvMergeIncludes(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("DATABASE_URL=postgres://localhost/db\nAWS_REGION=from-dotenv\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName), []byte(`base_path: ./
env:
  <<: !include .env
  AWS_REGION: inline
`), 0o644))

	v, err := loadConfigFile(tmpDir, CliConfigFileName)
	require.NoError(t, err)

	assert.Equal(t, "inline", v.GetString("env.AWS_REGION"))
	assert.Equal(t, "postgres://localhost/db", v.GetString("env.DATABASE_URL"))
}

func TestResolveDotenvMergeIncludeKeys(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("DATABASE_URL=postgres://localhost/db\nAWS_REGION=from-dotenv\nSHARED=base\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env.local"), []byte("AWS_REGION=from-local\nLOCAL_ONLY=true\nSHARED=local\n"), 0o644))

	content := []byte(`env:
  <<:
    - !include .env.local
    - !include .env
  AWS_REGION: inline
templates:
  settings:
    env:
      <<: !include .env
`)

	resolved, err := resolveDotenvMergeIncludeKeys(configFile, content)
	require.NoError(t, err)
	assert.NotEqual(t, string(content), string(resolved))

	var parsed struct {
		Env       map[string]string `yaml:"env"`
		Templates struct {
			Settings struct {
				Env map[string]string `yaml:"env"`
			} `yaml:"settings"`
		} `yaml:"templates"`
	}
	require.NoError(t, yaml.Unmarshal(resolved, &parsed))
	assert.Equal(t, map[string]string{
		"AWS_REGION":   "inline",
		"DATABASE_URL": "postgres://localhost/db",
		"LOCAL_ONLY":   "true",
		"SHARED":       "local",
	}, parsed.Env)
	assert.Equal(t, "from-dotenv", parsed.Templates.Settings.Env["AWS_REGION"])
}

func TestResolveDotenvMergeIncludeKeys_NoChangeAndErrors(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, AtmosConfigFileName)

	unchanged := []byte("env:\n  AWS_REGION: inline\n")
	resolved, err := resolveDotenvMergeIncludeKeys(configFile, unchanged)
	require.NoError(t, err)
	assert.Equal(t, unchanged, resolved)

	_, err = resolveDotenvMergeIncludeKeys(configFile, []byte("env:\n  - : invalid\n"))
	assert.Error(t, err)

	_, err = resolveDotenvMergeIncludeKeys(configFile, []byte("env:\n  <<: !include settings.yaml\n"))
	assert.Error(t, err)
}

func TestResolveDotenvMergeIncludeValueBranches(t *testing.T) {
	changed, err := resolveDotenvMergeIncludeKeysInNode("atmos.yaml", nil)
	require.NoError(t, err)
	assert.False(t, changed)

	changed, err = resolveDotenvMergeIncludeValue("atmos.yaml", &yaml.Node{Kind: yaml.ScalarNode, Value: "plain"})
	require.NoError(t, err)
	assert.False(t, changed)

	changed, err = resolveDotenvMergeIncludeValue("atmos.yaml", &yaml.Node{
		Kind: yaml.SequenceNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "plain"},
		},
	})
	require.NoError(t, err)
	assert.False(t, changed)

	_, err = loadDotenvIncludeAsYAMLNode("atmos.yaml", "")
	assert.Error(t, err)
}

func TestFindConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yml"), []byte("base_path: ./\n"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "directory.yaml"), 0o755))

	configFile, err := findConfigFile(tmpDir, "atmos")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "atmos.yml"), configFile)

	_, err = findConfigFile(tmpDir, "directory")
	assert.ErrorIs(t, err, ErrResolvedConfigFileNotFound)
}

func TestDotenvIncludeCaseMaps(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("DATABASE_URL=postgres://localhost/db\nAWS_REGION=from-dotenv\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env.local"), []byte("LOCAL_ONLY=true\nAWS_REGION=from-local\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env.templates"), []byte("TEMPLATE_TOKEN=value\n"), 0o644))

	rawYAML := []byte(`env:
  <<:
    - !include .env.local
    - !include .env
  Inline_Key: inline
templates:
  settings:
    env: !include .env.templates
`)
	caseMaps := casemap.New()
	mergeDotenvIncludeCaseMaps(configFile, rawYAML, caseMaps)

	envCaseMap := caseMaps.Get(envKey)
	assert.Equal(t, "DATABASE_URL", envCaseMap["database_url"])
	assert.Equal(t, "AWS_REGION", envCaseMap["aws_region"])
	assert.Equal(t, "LOCAL_ONLY", envCaseMap["local_only"])
	assert.Equal(t, "Inline_Key", envCaseMap["inline_key"])

	templateCaseMap := caseMaps.Get("templates.settings.env")
	assert.Equal(t, "TEMPLATE_TOKEN", templateCaseMap["template_token"])
}

func TestDotenvIncludeCaseMapsSkipsInvalidInputs(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env.invalid"), []byte("INVALID LINE WITH SPACES\n"), 0o644))

	caseMaps := casemap.New()
	mergeDotenvIncludeCaseMaps(configFile, []byte("env:\n  - : invalid\n"), caseMaps)
	assert.Empty(t, caseMaps.Get(envKey))

	rawYAML := []byte(`env:
  <<:
    - !include .env.missing
    - !include .env.invalid
  Inline_Key: inline
`)
	mergeDotenvIncludeCaseMaps(configFile, rawYAML, caseMaps)
	assert.Equal(t, "Inline_Key", caseMaps.Get(envKey)["inline_key"])
}

func TestDotenvIncludeHelpers(t *testing.T) {
	assert.True(t, canRetryWithResolvedDotenvMergeIncludes(errors.New("yaml: map merge requires map or sequence of maps")))
	assert.False(t, canRetryWithResolvedDotenvMergeIncludes(nil))
	assert.False(t, canRetryWithResolvedDotenvMergeIncludes(errors.New("different error")))

	tests := []struct {
		includeValue string
		expectedFile string
		expectedOK   bool
	}{
		{includeValue: ".env", expectedFile: ".env", expectedOK: true},
		{includeValue: ".env.local", expectedFile: ".env.local", expectedOK: true},
		{includeValue: "config/foo.env", expectedFile: "config/foo.env", expectedOK: true},
		{includeValue: "foo.env.local", expectedFile: "foo.env.local", expectedOK: false},
		{includeValue: ".envrc", expectedFile: ".envrc", expectedOK: false},
		{includeValue: "", expectedOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.includeValue, func(t *testing.T) {
			includeFile, ok := parseDotenvIncludeFile(tt.includeValue)
			assert.Equal(t, tt.expectedOK, ok)
			assert.Equal(t, tt.expectedFile, includeFile)
		})
	}
}

func TestFindYAMLPathNode(t *testing.T) {
	var root yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(`env:
  AWS_REGION: us-east-2
templates:
  settings:
    env:
      TEMPLATE_TOKEN: value
`), &root))

	assert.Nil(t, findYAMLPathNode(nil, []string{envKey}))
	assert.Nil(t, findYAMLPathNode(&yaml.Node{Kind: yaml.DocumentNode}, []string{envKey}))
	assert.Nil(t, findYAMLPathNode(&yaml.Node{Kind: yaml.SequenceNode}, []string{envKey}))
	assert.NotNil(t, findYAMLPathNode(&root, []string{envKey}))
	assert.NotNil(t, findYAMLPathNode(&root, []string{"templates", "settings", envKey}))
	assert.Nil(t, findYAMLPathNode(&root, []string{"templates", "missing", envKey}))
}

func TestExtractAndRestoreEnvMapsFromViper(t *testing.T) {
	v := viper.New()
	v.SetConfigType(yamlType)
	require.NoError(t, v.ReadConfig(strings.NewReader(`env:
  AWS_REGION: us-east-2
templates:
  settings:
    env:
      TEMPLATE_TOKEN: value
`)))

	var atmosConfig schema.AtmosConfiguration
	extractEnvMapsFromViper(v, &atmosConfig)
	assert.Equal(t, "us-east-2", atmosConfig.Env["aws_region"])
	assert.Equal(t, "value", atmosConfig.Templates.Settings.Env["template_token"])

	caseMaps := casemap.New()
	caseMaps.Set(envKey, casemap.CaseMap{"aws_region": "AWS_REGION"})
	caseMaps.Set("templates.settings.env", casemap.CaseMap{"template_token": "TEMPLATE_TOKEN"})
	atmosConfig.CaseMaps = caseMaps
	restoreCaseSensitiveEnvMaps(&atmosConfig)
	assert.Equal(t, map[string]string{"AWS_REGION": "us-east-2"}, atmosConfig.Env)
	assert.Equal(t, map[string]string{"TEMPLATE_TOKEN": "value"}, atmosConfig.Templates.Settings.Env)
}
