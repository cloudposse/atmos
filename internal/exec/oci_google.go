package exec

import (
	"context"
	"errors"
	"fmt"
	"time"

	log "github.com/charmbracelet/log"
	"github.com/google/go-containerregistry/pkg/authn"
	"golang.org/x/oauth2/google"
)

var (
	// Static errors for Google Cloud authentication
	errFailedToFindGoogleCloudCredentials = errors.New("failed to find Google Cloud credentials")
	errNoGoogleCloudCredentialsFound      = errors.New("no Google Cloud credentials found for registry")
	errFailedToGetGoogleCloudToken        = errors.New("failed to get Google Cloud token")
)

// getGCRAuth attempts to get Google Container Registry authentication.
func getGCRAuth(registry string) (authn.Authenticator, error) {
	// Use Google Cloud Application Default Credentials
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		log.Debug("Failed to find Google Cloud credentials", logFieldRegistry, registry, "error", err)
		return nil, fmt.Errorf("%w: %w", errFailedToFindGoogleCloudCredentials, err)
	}

	if creds == nil || creds.TokenSource == nil {
		log.Debug("No Google Cloud credentials found", logFieldRegistry, registry)
		return nil, fmt.Errorf("%w %s", errNoGoogleCloudCredentialsFound, registry)
	}

	// Get a token from the credentials
	token, err := creds.TokenSource.Token()
	if err != nil {
		log.Debug("Failed to get Google Cloud token", logFieldRegistry, registry, "error", err)
		return nil, fmt.Errorf("%w: %w", errFailedToGetGoogleCloudToken, err)
	}

	// For GCR/Artifact Registry, use an OAuth2 access token as the password with
	// the username "oauth2accesstoken". This is the standard pattern for GCR/AR authentication.
	log.Debug("Successfully obtained Google Cloud credentials", logFieldRegistry, registry)
	return &authn.Basic{
		Username: "oauth2accesstoken",
		Password: token.AccessToken,
	}, nil
}
