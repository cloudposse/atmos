package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGitPushSharedRetryDecode(t *testing.T) {
	for _, policy := range []string{"{max_attempts: 4, initial_delay: 250ms, max_delay: 5s, random_jitter: 0.5, multiplier: 2, backoff_strategy: exponential, max_elapsed_time: 30s}", "{initial_delay: 2}", "{}", "{delay: 1s}"} {
		t.Run(policy, func(t *testing.T) {
			v := viper.New()
			v.SetConfigType("yaml")
			source := "git:\n  repositories:\n    deploy:\n      push:\n        retry: " + policy
			require.NoError(t, v.ReadConfig(strings.NewReader(source)))
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(source), &node))
			require.NoError(t, processNode(&node, v, ""))
			var cfg schema.AtmosConfiguration
			err := v.Unmarshal(&cfg, atmosDecodeHook())
			if strings.Contains(policy, "{delay:") {
				require.ErrorContains(t, err, "unknown retry field")
				return
			}
			require.NoError(t, err)
			retryConfig := cfg.Git.Repositories["deploy"].Push.Retry
			require.NotNil(t, retryConfig)
			switch {
			case strings.Contains(policy, "250ms"):
				require.Equal(t, 250*time.Millisecond, *retryConfig.InitialDelay)
				require.Equal(t, 0.5, *retryConfig.RandomJitter)
				require.Equal(t, 4, *retryConfig.MaxAttempts)
			case strings.Contains(policy, "initial_delay: 2"):
				require.Equal(t, 2*time.Second, *retryConfig.InitialDelay)
			default:
				require.Equal(t, 1, *retryConfig.MaxAttempts)
			}
		})
	}
}

func TestGitPushRetryMergeOverrides(t *testing.T) {
	for _, later := range []string{"{max_attempts: 4}", "!unset"} {
		t.Run(later, func(t *testing.T) {
			dir := t.TempDir()
			createConfigFile(t, dir, "import.yaml", "git: {repositories: {deploy: {push: {retry: {}}}}}")
			createConfigFile(t, dir, "main.yaml", "git: {repositories: {deploy: {push: {retry: "+later+" }}}}")
			v := viper.New()
			v.SetConfigType("yaml")
			require.NoError(t, mergeConfigFile(filepath.Join(dir, "import.yaml"), v))
			require.Equal(t, 1, v.GetInt("git.repositories.deploy.push.retry.max_attempts"))
			require.NoError(t, mergeConfigFile(filepath.Join(dir, "main.yaml"), v))
			if later == "!unset" {
				require.False(t, v.IsSet("git.repositories.deploy.push.retry"))
			} else {
				require.Equal(t, 4, v.GetInt("git.repositories.deploy.push.retry.max_attempts"))
			}
		})
	}
}

func TestGitPushRetryIncludedEmpty(t *testing.T) {
	dir := t.TempDir()
	createConfigFile(t, dir, "retry.yaml", "{}")
	source := "git: {repositories: {deploy: {push: {retry: !include " + filepath.ToSlash(filepath.Join(dir, "retry.yaml")) + "}}}}"
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(source)))
	require.NoError(t, preprocessAtmosYamlFunc([]byte(source), v))
	require.Equal(t, 1, v.GetInt("git.repositories.deploy.push.retry.max_attempts"))
}

func TestGitPushRetryEmptyInIncludedPush(t *testing.T) {
	dir := t.TempDir()
	createConfigFile(t, dir, "push.yaml", "retry: {}")
	source := "git: {repositories: {deploy: {push: !include " + filepath.ToSlash(filepath.Join(dir, "push.yaml")) + "}}}"
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(source)))
	require.NoError(t, preprocessAtmosYamlFunc([]byte(source), v))
	require.Equal(t, 1, v.GetInt("git.repositories.deploy.push.retry.max_attempts"))
}

func TestGitPushRetryNonEmptyInIncludedPushIsUntouched(t *testing.T) {
	dir := t.TempDir()
	createConfigFile(t, dir, "push.yaml", "retry: {max_attempts: 4}")
	source := "git: {repositories: {deploy: {push: !include " + filepath.ToSlash(filepath.Join(dir, "push.yaml")) + "}}}"
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(source)))
	require.NoError(t, preprocessAtmosYamlFunc([]byte(source), v))
	require.Equal(t, 4, v.GetInt("git.repositories.deploy.push.retry.max_attempts"))
}

func TestPreserveEmptyGitPushRetryResolvesAliases(t *testing.T) {
	tests := []struct {
		name         string
		anchor       string
		wantAttempts int
		wantSet      bool
	}{
		{name: "alias to empty policy means one attempt", anchor: "{}", wantAttempts: 1, wantSet: true},
		{name: "alias to populated policy is left to the regular decode", anchor: "{max_attempts: 4}", wantSet: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "defaults: {retry: &shared " + tt.anchor + "}\n" +
				"git: {repositories: {deploy: {push: {retry: *shared}}}}"
			var doc yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
			aliasNode := &doc
			for _, key := range []string{"git", "repositories", "deploy", "push", "retry"} {
				aliasNode = yamlMappingValue(t, aliasNode, key)
			}
			require.Equal(t, yaml.AliasNode, aliasNode.Kind)

			v := viper.New()
			v.SetConfigType("yaml")
			path := "git.repositories.deploy.push.retry"
			require.NoError(t, preserveEmptyGitPushRetry(aliasNode, v, path))
			require.Equal(t, tt.wantSet, v.IsSet(path+".max_attempts"))
			if tt.wantSet {
				require.Equal(t, tt.wantAttempts, v.GetInt(path+".max_attempts"))
			}
		})
	}
}

// yamlMappingValue returns the value node for key in a document or mapping node without resolving aliases.
func yamlMappingValue(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()
	if node.Kind == yaml.DocumentNode {
		node = node.Content[0]
	}
	require.Equal(t, yaml.MappingNode, node.Kind)
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	require.FailNow(t, "key not found", key)
	return nil
}
