package exec

import (
	"errors"
	"fmt"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Static errors for GitHub authentication
var errNoGitHubAuthenticationFound = errors.New("no GitHub authentication found for registry")

// getGitHubAuth attempts to get GitHub Container Registry authentication.
func getGitHubAuth(registry string, atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, error) {
	// Check for GitHub Container Registry
	if strings.EqualFold(registry, "ghcr.io") {
		// Create a Viper instance for environment variable access
		v := viper.New()
		bindEnv(v, "github_token", "ATMOS_OCI_GITHUB_TOKEN", "GITHUB_TOKEN")

		// Try Atmos-specific token first, then fallback to standard GITHUB_TOKEN
		token := atmosConfig.Settings.OCI.GithubToken
		if token == "" {
			token = v.GetString("github_token") // Use Viper instead of os.Getenv
		}
		if token != "" {
			log.Debug("Using GitHub token for authentication", "registry", registry)
			return &authn.Basic{
				Username: "oauth2",
				Password: token,
			}, nil
		}
	}

	return nil, fmt.Errorf("%w %s", errNoGitHubAuthenticationFound, registry)
}
