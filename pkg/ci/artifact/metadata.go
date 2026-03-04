package artifact

import "time"

// Metadata contains metadata about a stored artifact.
type Metadata struct {
	// Stack is the stack name.
	Stack string `json:"stack"`

	// Component is the component name.
	Component string `json:"component"`

	// SHA is the git commit SHA this artifact was generated for.
	SHA string `json:"sha"`

	// BaseSHA is the base commit SHA (target branch) for comparison.
	BaseSHA string `json:"base_sha,omitempty"`

	// Branch is the git branch this artifact was generated from.
	Branch string `json:"branch,omitempty"`

	// PRNumber is the pull request number if applicable.
	PRNumber int `json:"pr_number,omitempty"`

	// RunID is the CI run ID.
	RunID string `json:"run_id,omitempty"`

	// Repository is the repository URL or identifier.
	Repository string `json:"repository,omitempty"`

	// CreatedAt is when the artifact was created.
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt is when the artifact should be considered expired (optional).
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// SHA256 is the SHA-256 checksum of the artifact content.
	SHA256 string `json:"sha256,omitempty"`

	// AtmosVersion is the version of Atmos that created this artifact.
	AtmosVersion string `json:"atmos_version,omitempty"`

	// Custom allows arbitrary key-value pairs for provider-specific metadata.
	Custom map[string]string `json:"custom,omitempty"`
}

// ArtifactInfo contains basic information about a stored artifact.
type ArtifactInfo struct {
	// Name is the artifact name/key.
	Name string `json:"name"`

	// Size is the total size in bytes.
	Size int64 `json:"size"`

	// LastModified is when the artifact was last modified.
	LastModified time.Time `json:"last_modified"`

	// Metadata contains the artifact metadata if available.
	Metadata *Metadata `json:"metadata,omitempty"`
}
