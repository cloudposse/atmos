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
// OAuth Device Flow doesn't require client secrets, and the client ID is
// considered public information for native/CLI applications.
//
// Users can override this by specifying their own client_id in provider config.
const DefaultClientID = "178c6fc778ccc68e1d6a"
