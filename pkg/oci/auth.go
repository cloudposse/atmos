// Package oci provides shared helpers for authenticating to OCI registries.
package oci

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GHCRAuth returns authentication credentials for GitHub Container Registry
// (ghcr.io) derived from Atmos settings, along with a human-readable source
// description. It returns (nil, "") when no usable credentials are configured
// so callers can fall back to anonymous access or the default keychain.
func GHCRAuth(atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, string) {
	defer perf.Track(atmosConfig, "oci.GHCRAuth")()

	if atmosConfig == nil {
		return nil, ""
	}
	atmosToken := strings.TrimSpace(atmosConfig.Settings.AtmosGithubToken)
	githubToken := strings.TrimSpace(atmosConfig.Settings.GithubToken)
	githubUsername := strings.TrimSpace(atmosConfig.Settings.GithubUsername)

	var token string
	var tokenSource string

	if atmosToken != "" {
		token = atmosToken
		tokenSource = "ATMOS_GITHUB_TOKEN"
	} else if githubToken != "" {
		token = githubToken
		tokenSource = "GITHUB_TOKEN"
	}

	if token == "" {
		return nil, ""
	}

	// GHCR requires a username; use configured github_username.
	username := githubUsername
	if username == "" {
		// No safe implicit fallback here; return nil to allow caller to choose anon/fail.
		log.Warn("GHCR token found but no username provided; set settings.github_username or ATMOS_GITHUB_USERNAME/GITHUB_ACTOR.")
		return nil, ""
	}

	authMethod := &authn.Basic{
		Username: username,
		Password: token,
	}
	authSource := fmt.Sprintf("environment variable (%s with username %s)", tokenSource, username)

	return authMethod, authSource
}
