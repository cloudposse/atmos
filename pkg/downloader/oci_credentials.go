package downloader

import (
	"os"

	"github.com/cloudposse/atmos/pkg/schema"
)

// withGHCREnvFallback returns a configuration whose GitHub Container Registry credential settings fall
// back to the live environment when they are empty.
//
// Remote imports in atmos.yaml are fetched while the configuration is still being assembled, before its
// settings are populated from the environment, so credentials supplied only through the environment (the
// GitHub Actions default of GITHUB_TOKEN with GITHUB_ACTOR) would otherwise be missed and a private ghcr.io
// artifact would be pulled anonymously. Values already present in the configuration always win. The
// original configuration is never modified.
func withGHCREnvFallback(atmosConfig *schema.AtmosConfiguration) *schema.AtmosConfiguration {
	effective := &schema.AtmosConfiguration{}
	if atmosConfig != nil {
		*effective = *atmosConfig
	}

	settings := &effective.Settings
	if settings.AtmosGithubToken == "" {
		settings.AtmosGithubToken = liveEnv("ATMOS_GITHUB_TOKEN")
	}
	if settings.GithubToken == "" {
		settings.GithubToken = liveEnv("GITHUB_TOKEN")
	}
	if settings.GithubUsername == "" {
		settings.GithubUsername = firstLiveEnv("ATMOS_GITHUB_USERNAME", "GITHUB_ACTOR", "GITHUB_USERNAME")
	}
	return effective
}

// firstLiveEnv returns the first non-empty value among the named environment variables.
func firstLiveEnv(names ...string) string {
	for _, name := range names {
		if value := liveEnv(name); value != "" {
			return value
		}
	}
	return ""
}

// liveEnv reads an environment variable directly.
func liveEnv(name string) string {
	//nolint:forbidigo // Live env fallback for credentials needed before Settings is populated; mirrors pkg/http/client.go.
	return os.Getenv(name)
}
