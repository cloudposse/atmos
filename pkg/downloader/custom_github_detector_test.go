package downloader

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCustomGitHubDetector_Detect(t *testing.T) {
	t.Run("should inject token for GitHub URL when GITHUB_TOKEN is set", func(t *testing.T) {
		envMock := func(key string) string {
			if key == "ATMOS_GITHUB_TOKEN" {
				return ""
			}
			if key == "GITHUB_TOKEN" {
				return "test-token"
			}
			return ""
		}

		detector := customGitHubDetector{
			AtmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{InjectGithubToken: true},
			},
			GetEnv: envMock,
		}

		finalURL, detected, err := detector.Detect("https://github.com/owner/repo.git", "")

		assert.NoError(t, err)
		assert.True(t, detected)
		assert.Contains(t, finalURL, "git::https://x-access-token:test-token@github.com/owner/repo.git")
	})

	t.Run("should inject token for GitHub URL when ATMOS_GITHUB_TOKEN is set", func(t *testing.T) {
		envMock := func(key string) string {
			if key == "ATMOS_GITHUB_TOKEN" {
				return "atmos-token"
			}
			return ""
		}

		detector := customGitHubDetector{
			AtmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{InjectGithubToken: true},
			},
			GetEnv: envMock,
		}

		finalURL, detected, err := detector.Detect("https://github.com/owner/repo.git", "")

		assert.NoError(t, err)
		assert.True(t, detected)
		assert.Contains(t, finalURL, "git::https://x-access-token:atmos-token@github.com/owner/repo.git")
	})

	t.Run("should not inject token if InjectGithubToken=false", func(t *testing.T) {
		envMock := func(key string) string {
			if key == "GITHUB_TOKEN" {
				return "test-token"
			}
			return ""
		}

		detector := customGitHubDetector{
			AtmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{InjectGithubToken: false},
			},
			GetEnv: envMock,
		}

		finalURL, detected, err := detector.Detect("https://github.com/owner/repo.git", "")

		assert.NoError(t, err)
		assert.True(t, detected)
		assert.NotContains(t, finalURL, "x-access-token")
	})

	t.Run("should not modify non-GitHub URLs", func(t *testing.T) {
		detector := customGitHubDetector{
			GetEnv: func(_ string) string { return "" },
		}

		finalURL, detected, err := detector.Detect("https://gitlab.com/owner/repo.git", "")

		assert.NoError(t, err)
		assert.False(t, detected)
		assert.Empty(t, finalURL)
	})

	t.Run("should not modify URL if credentials are already present", func(t *testing.T) {
		detector := customGitHubDetector{
			GetEnv:      func(_ string) string { return "" },
			AtmosConfig: &schema.AtmosConfiguration{Settings: schema.AtmosSettings{InjectGithubToken: false}},
		}

		finalURL, detected, err := detector.Detect("https://user:pass@github.com/owner/repo.git", "")

		assert.NoError(t, err)
		assert.True(t, detected)
		assert.Equal(t, "git::https://user:pass@github.com/owner/repo.git", finalURL)
	})

	t.Run("should return an error for invalid URLs", func(t *testing.T) {
		detector := customGitHubDetector{
			GetEnv: func(_ string) string { return "" },
		}

		finalURL, detected, err := detector.Detect("inva^^$$lid-url", "")

		assert.Error(t, err)
		assert.False(t, detected)
		assert.Empty(t, finalURL)
	})

	t.Run("should return an error for empty source URL", func(t *testing.T) {
		detector := customGitHubDetector{
			GetEnv: func(_ string) string { return "" },
		}

		finalURL, detected, err := detector.Detect("", "")

		assert.Error(t, err)
		assert.False(t, detected)
		assert.Empty(t, finalURL)
	})
}
