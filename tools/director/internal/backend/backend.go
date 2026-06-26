package backend

import "context"

// Backend defines the interface for publishing rendered VHS demos to storage backends.
type Backend interface {
	// Validate checks that the backend credentials and configuration are valid.
	Validate(ctx context.Context) error

	// Upload uploads a single file from localPath to the backend at remotePath.
	// The title parameter is used for human-readable display names (e.g., in Stream UI).
	Upload(ctx context.Context, localPath, remotePath, title string) error

	// GetPublicURL returns the public URL for a file at remotePath.
	GetPublicURL(remotePath string) string

	// SupportsFormat checks if the backend supports a given file format.
	SupportsFormat(ext string) bool
}
