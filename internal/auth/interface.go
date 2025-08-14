package auth

// LoginMethod defines the interface that all authentication providers must implement.
// This interface provides methods for the complete authentication lifecycle including
// validation, login, role assumption, and logout.
type LoginMethod interface {
	// Validate ensures the authentication configuration is valid before attempting to use it
	Validate() error

	// Login authenticates with the identity provider and obtains credentials
	Login() error

	// AssumeRole uses the authenticated credentials to assume a specific IAM role
	AssumeRole() error

	// Logout removes any cached credentials and logs out from the identity provider
	Logout() error
}
