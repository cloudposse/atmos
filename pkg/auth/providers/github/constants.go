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
