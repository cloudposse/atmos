package planfile

import (
	"context"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Type aliases to align planfile types with artifact types.
type FileEntry = artifact.FileEntry
type FileResult = artifact.FileResult
type Query = artifact.Query
type StoreOptions = artifact.StoreOptions

const (
	// PlanFilename is the well-known name for the plan file within a bundle.
	PlanFilename = "plan.tfplan"

	// LockFilename is the well-known name for the lock file within a bundle.
	LockFilename = ".terraform.lock.hcl"
)

// Store defines the interface for planfile storage backends.
// Implementations include S3, Azure Blob, GCS, GitHub Artifacts, and local filesystem.
type Store interface {
	// Name returns the store type name (e.g., "s3", "azure", "gcs", "github", "local").
	Name() string

	// Upload uploads a planfile to the store.
	Upload(ctx context.Context, name string, files []FileEntry, metadata *Metadata) error

	// Download downloads a planfile from the store.
	Download(ctx context.Context, name string) ([]FileResult, *Metadata, error)

	// Delete deletes a planfile from the store.
	Delete(ctx context.Context, name string) error

	// List lists planfiles matching the given query.
	List(ctx context.Context, query Query) ([]PlanfileInfo, error)

	// Exists checks if a planfile exists.
	Exists(ctx context.Context, name string) (bool, error)

	// GetMetadata retrieves metadata for a planfile without downloading the content.
	GetMetadata(ctx context.Context, name string) (*Metadata, error)
}

// Metadata contains metadata about a stored planfile.
// It embeds artifact.Metadata for common CI artifact fields and adds
// planfile-specific fields for Terraform plan data.
type Metadata struct {
	artifact.Metadata

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

	// TerraformVersion is the version of Terraform used.
	TerraformVersion string `json:"terraform_version,omitempty"`

	// TerraformTool is the Terraform tool used (e.g., "terraform", "tofu").
	TerraformTool string `json:"terraform_tool,omitempty"`
}

// Validate checks that required metadata fields are present.
// Delegates to the embedded artifact.Metadata.Validate() for base field validation,
// then wraps the error as ErrPlanfileMetadataInvalid for planfile-specific context.
func (m *Metadata) Validate() error {
	if err := m.Metadata.Validate(); err != nil {
		return errUtils.ErrPlanfileMetadataInvalid
	}
	return nil
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

// KeyPattern holds the pattern configuration for generating planfile keys.
type KeyPattern struct {
	Pattern string
}

// DefaultKeyPattern returns the default planfile key pattern.
func DefaultKeyPattern() KeyPattern {
	defer perf.Track(nil, "planfile.DefaultKeyPattern")()

	return KeyPattern{
		Pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan.tar",
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
// Returns an error if required fields (Stack, Component, SHA) are empty when
// they are used in the pattern.
func (p KeyPattern) GenerateKey(ctx *KeyContext) (string, error) {
	defer perf.Track(nil, "planfile.GenerateKey")()

	// Validate required fields based on pattern usage.
	if err := validateKeyContext(p.Pattern, ctx); err != nil {
		return "", err
	}

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
		// Replace all occurrences, even if value is empty (after validation).
		key = strings.ReplaceAll(key, placeholder, value)
	}

	return key, nil
}

// validateKeyContext validates that required fields are present for the pattern.
func validateKeyContext(pattern string, ctx *KeyContext) error {
	// Check required fields based on their presence in the pattern.
	if strings.Contains(pattern, "{{ .Stack }}") && ctx.Stack == "" {
		return errUtils.ErrPlanfileKeyInvalid
	}
	if strings.Contains(pattern, "{{ .Component }}") && ctx.Component == "" {
		return errUtils.ErrPlanfileKeyInvalid
	}
	if strings.Contains(pattern, "{{ .SHA }}") && ctx.SHA == "" {
		return errUtils.ErrPlanfileKeyInvalid
	}
	return nil
}

// NewAtmosConfig is a helper to build StoreOptions with an AtmosConfiguration.
// This is used to bridge from planfile-specific config to artifact StoreOptions.
func NewAtmosConfig(storeType string, options map[string]any, atmosConfig *schema.AtmosConfiguration) StoreOptions {
	return StoreOptions{
		Type:        storeType,
		Options:     options,
		AtmosConfig: atmosConfig,
	}
}
