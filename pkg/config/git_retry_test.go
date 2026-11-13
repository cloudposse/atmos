package config

import (
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
