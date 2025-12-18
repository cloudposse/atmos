package planfile

import (
	"context"
	"io"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Store defines the interface for planfile storage backends.
// Implementations include S3, Azure Blob, GCS, GitHub Artifacts, and local filesystem.
type Store interface {
	// Name returns the store type name (e.g., "s3", "azure", "gcs", "github", "local").
	Name() string

	// Upload uploads a planfile to the store.
	Upload(ctx context.Context, key string, data io.Reader, metadata *Metadata) error

	// Download downloads a planfile from the store.
	Download(ctx context.Context, key string) (io.ReadCloser, *Metadata, error)

	// Delete deletes a planfile from the store.
	Delete(ctx context.Context, key string) error

	// List lists planfiles matching the given prefix.
	List(ctx context.Context, prefix string) ([]PlanfileInfo, error)

	// Exists checks if a planfile exists.
	Exists(ctx context.Context, key string) (bool, error)

	// GetMetadata retrieves metadata for a planfile without downloading the content.
	GetMetadata(ctx context.Context, key string) (*Metadata, error)
}

// Metadata contains metadata about a stored planfile.
type Metadata struct {
	// Stack is the stack name.
	Stack string `json:"stack"`

	// Component is the component name.
	Component string `json:"component"`

	// ComponentPath is the path to the component.
	ComponentPath string `json:"component_path"`

	// SHA is the git commit SHA this plan was generated for.
	SHA string `json:"sha"`

	// BaseSHA is the base commit SHA (target branch) for comparison.
	BaseSHA string `json:"base_sha,omitempty"`

	// Branch is the git branch this plan was generated from.
	Branch string `json:"branch,omitempty"`

	// PRNumber is the pull request number if applicable.
	PRNumber int `json:"pr_number,omitempty"`

	// RunID is the CI run ID.
	RunID string `json:"run_id,omitempty"`

	// Repository is the repository URL or identifier.
	Repository string `json:"repository,omitempty"`

	// CreatedAt is when the planfile was created.
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt is when the planfile should be considered expired (optional).
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// PlanSummary contains a human-readable summary of the plan.
	PlanSummary string `json:"plan_summary,omitempty"`

	// HasChanges indicates whether the plan has changes.
	HasChanges bool `json:"has_changes"`

	// Additions is the number of resources to add.
	Additions int `json:"additions"`

	// Changes is the number of resources to change.
	Changes int `json:"changes"`

	// Destructions is the number of resources to destroy.
	Destructions int `json:"destructions"`

	// Custom allows arbitrary key-value pairs for provider-specific metadata.
	Custom map[string]string `json:"custom,omitempty"`
}

// PlanfileInfo contains basic information about a stored planfile.
type PlanfileInfo struct {
	// Key is the storage key/path.
	Key string `json:"key"`

	// Size is the file size in bytes.
	Size int64 `json:"size"`

	// LastModified is when the file was last modified.
	LastModified time.Time `json:"last_modified"`

	// Metadata contains the planfile metadata if available.
	Metadata *Metadata `json:"metadata,omitempty"`
}

// StoreOptions contains options for creating a store.
type StoreOptions struct {
	// Type is the store type (s3, azure, gcs, github, local).
	Type string

	// Options contains type-specific configuration options.
	Options map[string]any

	// AtmosConfig is the Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration
}

// StoreFactory is a function that creates a Store from options.
type StoreFactory func(opts StoreOptions) (Store, error)

// KeyPattern holds the pattern configuration for generating planfile keys.
type KeyPattern struct {
	Pattern string
}

// DefaultKeyPattern returns the default planfile key pattern.
func DefaultKeyPattern() KeyPattern {
	defer perf.Track(nil, "planfile.DefaultKeyPattern")()

	return KeyPattern{
		Pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan",
	}
}

// KeyContext contains the context for generating a planfile key.
type KeyContext struct {
	Stack         string
	Component     string
	ComponentPath string
	SHA           string
	BaseSHA       string
	Branch        string
	PRNumber      int
	RunID         string
}

// GenerateKey generates a planfile key from the context using the pattern.
func (p KeyPattern) GenerateKey(ctx KeyContext) (string, error) {
	defer perf.Track(nil, "planfile.GenerateKey")()

	// Simple template replacement for now.
	// In a full implementation, we'd use text/template.
	key := p.Pattern
	replacements := map[string]string{
		"{{ .Stack }}":         ctx.Stack,
		"{{ .Component }}":     ctx.Component,
		"{{ .ComponentPath }}": ctx.ComponentPath,
		"{{ .SHA }}":           ctx.SHA,
		"{{ .BaseSHA }}":       ctx.BaseSHA,
		"{{ .Branch }}":        ctx.Branch,
	}

	for placeholder, value := range replacements {
		if value != "" {
			key = replaceAll(key, placeholder, value)
		}
	}

	return key, nil
}

// replaceAll replaces all occurrences of old with new in s.
func replaceAll(s, old, new string) string {
	for {
		i := indexOf(s, old)
		if i == -1 {
			return s
		}
		s = s[:i] + new + s[i+len(old):]
	}
}

// indexOf returns the index of substr in s, or -1 if not found.
func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
