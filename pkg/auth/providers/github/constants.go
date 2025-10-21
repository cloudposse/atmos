package github

// Provider kind constants for GitHub authentication providers.
const (
	// KindUser is the kind for GitHub User authentication via OAuth Device Flow.
	KindUser = "github/user"

	// KindApp is the kind for GitHub App authentication via JWT.
	KindApp = "github/app"

	// KindOIDC is the kind for GitHub OIDC authentication (Actions).
	KindOIDC = "github/oidc"
)

// DefaultClientID is the GitHub CLI OAuth App client ID.
// This is the same OAuth App used by the official GitHub CLI (gh).
// Source: https://github.com/cli/cli/blob/trunk/internal/authflow/flow.go
//
// This value is safe to embed in version control and distributed applications.
// The client ID is considered public information for native/CLI applications.
//
// Users can override this by specifying their own client_id in provider config.
const DefaultClientID = "178c6fc778ccc68e1d6a"

// DefaultClientSecret is the GitHub CLI OAuth App client secret.
// This is the same OAuth App used by the official GitHub CLI (gh).
// Source: https://github.com/cli/cli/blob/trunk/internal/authflow/flow.go
//
// IMPORTANT: While client secrets are typically kept private for server-side apps,
// this secret is publicly available in the GitHub CLI source code and is designed
// for use in native/CLI applications. It's used for Web Application Flow where
// the local HTTP server exchanges the authorization code for an access token.
//
// This approach follows GitHub's OAuth security model for native applications,
// where the redirect URI (http://127.0.0.1) provides the security boundary rather
// than the client secret itself.
//
// Users can override this by specifying their own client_secret in provider config.
const DefaultClientSecret = "34ddeff2b558a23d38fba8a6de74f086ede1cc0b"
