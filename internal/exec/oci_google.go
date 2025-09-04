package exec

import (
	"context"
	"fmt"

	log "github.com/charmbracelet/log"
	"github.com/google/go-containerregistry/pkg/authn"
	"golang.org/x/oauth2/google"
)

// getGCRAuth attempts to get Google Container Registry authentication.
func getGCRAuth(registry string) (authn.Authenticator, error) {
	// Use Google Cloud Application Default Credentials
	ctx := context.Background()
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		log.Debug("Failed to find Google Cloud credentials", "registry", registry, "error", err)
		return nil, fmt.Errorf("failed to find Google Cloud credentials: %w", err)
	}

	if creds == nil || creds.TokenSource == nil {
		log.Debug("No Google Cloud credentials found", "registry", registry)
		return nil, fmt.Errorf("no Google Cloud credentials found for registry %s", registry)
	}

	// Get a token from the credentials
	token, err := creds.TokenSource.Token()
	if err != nil {
		log.Debug("Failed to get Google Cloud token", "registry", registry, "error", err)
		return nil, fmt.Errorf("failed to get Google Cloud token: %w", err)
	}

	// For GCR, we use the token as the password with "_dcg" as username
	// This is the standard pattern for GCR authentication
	log.Debug("Successfully obtained Google Cloud credentials", "registry", registry)
	return &authn.Basic{
		Username: "oauth2accesstoken",
		Password: token.AccessToken,
	}, nil
}
