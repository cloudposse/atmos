package sopsauth

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/getsops/sops/v3/gcpkms"
	"golang.org/x/oauth2"

	"github.com/cloudposse/atmos/pkg/perf"
)

// errNoGCPCredentials indicates the resolved GCP auth context carried neither an access token nor a
// credentials file to authenticate the SOPS GCP KMS operation.
var errNoGCPCredentials = errors.New("no GCP credentials (access token or credentials file) resolved")

// GCPKMS resolves the identity's GCP credentials. An access token (the common federated case) is
// wrapped as a static OAuth2 token source; otherwise a service-account credentials file is used.
func (b *resolverBuilder) GCPKMS(ctx context.Context, identity string) (GCPApplier, error) {
	defer perf.Track(nil, "sopsauth.GCPKMS")()

	authContext, err := b.resolver.ResolveGCPAuthContext(ctx, identity)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve GCP auth context for SOPS identity %q: %w", identity, err)
	}

	if authContext.AccessToken != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: authContext.AccessToken,
			Expiry:      authContext.TokenExpiry,
		})
		return gcpkms.NewTokenSource(ts), nil
	}

	if authContext.CredentialsFile != "" {
		data, readErr := os.ReadFile(authContext.CredentialsFile)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read GCP credentials file for SOPS identity %q: %w", identity, readErr)
		}
		return gcpkms.CredentialJSON(data), nil
	}

	return nil, fmt.Errorf("%w for SOPS identity %q", errNoGCPCredentials, identity)
}
