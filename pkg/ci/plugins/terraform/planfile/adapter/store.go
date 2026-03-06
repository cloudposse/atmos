package adapter

import (
	"context"
	"fmt"
	"io"
	"maps"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// planFilename is the well-known name for the plan file within an artifact bundle.
	planFilename = "plan.tfplan"

	// Metadata custom key prefixes for planfile-specific fields.
	customKeyPlanSummary      = "planfile.plan_summary"
	customKeyHasChanges       = "planfile.has_changes"
	customKeyAdditions        = "planfile.additions"
	customKeyChanges          = "planfile.changes"
	customKeyDestructions     = "planfile.destructions"
	customKeyTerraformVersion = "planfile.terraform_version"
	customKeyTerraformTool    = "planfile.terraform_tool"
)

// Compile-time check that Store implements planfile.Store.
var _ planfile.Store = (*Store)(nil)

// Store adapts an artifact.Store to implement planfile.Store.
// It wraps single plan files as artifact bundles and converts metadata between formats.
type Store struct {
	backend artifact.Store
}

// NewStore creates a new adapter Store wrapping the given artifact backend.
func NewStore(backend artifact.Store) *Store {
	return &Store{backend: backend}
}

// Name returns the backend store type name.
func (s *Store) Name() string {
	defer perf.Track(nil, "adapter.Store.Name")()

	return s.backend.Name()
}

// Upload uploads a planfile by wrapping it as a single-file artifact bundle.
func (s *Store) Upload(ctx context.Context, key string, data io.Reader, metadata *planfile.Metadata) error {
	defer perf.Track(nil, "adapter.Store.Upload")()

	files := []artifact.FileEntry{
		{
			Name: planFilename,
			Data: data,
			Size: -1,
		},
	}

	artMeta := planfileToArtifactMeta(metadata)

	if err := s.backend.Upload(ctx, key, files, artMeta); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	return nil
}

// Download downloads a planfile from the artifact bundle.
// It extracts the plan.tfplan file and closes all other file handles.
func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, *planfile.Metadata, error) {
	defer perf.Track(nil, "adapter.Store.Download")()

	results, artMeta, err := s.backend.Download(ctx, key)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}

	var planReader io.ReadCloser
	for _, r := range results {
		if r.Name == planFilename {
			planReader = r.Data
		} else {
			// Close non-plan file handles.
			r.Data.Close()
		}
	}

	if planReader == nil {
		return nil, nil, fmt.Errorf("%w: %s not found in artifact bundle", errUtils.ErrPlanfileDownloadFailed, planFilename)
	}

	meta := artifactToPlanfileMeta(artMeta)

	return planReader, meta, nil
}

// Delete deletes a planfile artifact.
func (s *Store) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "adapter.Store.Delete")()

	if err := s.backend.Delete(ctx, key); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrPlanfileDeleteFailed, err)
	}

	return nil
}

// List lists planfiles matching the given prefix by converting it to an artifact query.
func (s *Store) List(ctx context.Context, prefix string) ([]planfile.PlanfileInfo, error) {
	defer perf.Track(nil, "adapter.Store.List")()

	query := prefixToQuery(prefix)

	infos, err := s.backend.List(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPlanfileListFailed, err)
	}

	result := make([]planfile.PlanfileInfo, 0, len(infos))
	for _, info := range infos {
		result = append(result, planfile.PlanfileInfo{
			Key:          info.Name,
			Size:         info.Size,
			LastModified: info.LastModified,
			Metadata:     artifactToPlanfileMeta(info.Metadata),
		})
	}

	return result, nil
}

// Exists checks if a planfile artifact exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	defer perf.Track(nil, "adapter.Store.Exists")()

	exists, err := s.backend.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("%w: %w", errUtils.ErrPlanfileStatFailed, err)
	}

	return exists, nil
}

// GetMetadata retrieves planfile metadata without downloading content.
func (s *Store) GetMetadata(ctx context.Context, key string) (*planfile.Metadata, error) {
	defer perf.Track(nil, "adapter.Store.GetMetadata")()

	artMeta, err := s.backend.GetMetadata(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPlanfileMetadataFailed, err)
	}

	return artifactToPlanfileMeta(artMeta), nil
}

// planfileToArtifactMeta converts planfile metadata to artifact metadata.
// Shared fields come from the embedded artifact.Metadata; planfile-specific fields
// are stored in the Custom map with "planfile." prefixed keys.
func planfileToArtifactMeta(meta *planfile.Metadata) *artifact.Metadata {
	if meta == nil {
		return nil
	}

	// Start from the embedded base — all shared fields come for free.
	artMeta := meta.Metadata // copy the embedded artifact.Metadata.

	// Ensure Custom map exists (don't mutate the original).
	custom := make(map[string]string)
	maps.Copy(custom, artMeta.Custom)

	// Store planfile-specific fields.
	if meta.PlanSummary != "" {
		custom[customKeyPlanSummary] = meta.PlanSummary
	}
	custom[customKeyHasChanges] = strconv.FormatBool(meta.HasChanges)
	custom[customKeyAdditions] = strconv.Itoa(meta.Additions)
	custom[customKeyChanges] = strconv.Itoa(meta.Changes)
	custom[customKeyDestructions] = strconv.Itoa(meta.Destructions)
	if meta.TerraformVersion != "" {
		custom[customKeyTerraformVersion] = meta.TerraformVersion
	}
	if meta.TerraformTool != "" {
		custom[customKeyTerraformTool] = meta.TerraformTool
	}

	artMeta.Custom = custom
	return &artMeta
}

// artifactToPlanfileMeta converts artifact metadata to planfile metadata.
// Shared fields are preserved via embedding; planfile-specific fields are extracted from Custom.
func artifactToPlanfileMeta(meta *artifact.Metadata) *planfile.Metadata {
	if meta == nil {
		return nil
	}

	// Embed the full artifact metadata — SHA256, AtmosVersion, etc. are preserved.
	result := &planfile.Metadata{
		Metadata: *meta,
	}

	// Replace Custom with a clean map (planfile-specific keys will be extracted).
	cleanCustom := make(map[string]string)

	// Extract planfile-specific fields from custom map.
	for k, v := range meta.Custom {
		switch k {
		case customKeyPlanSummary:
			result.PlanSummary = v
		case customKeyHasChanges:
			result.HasChanges, _ = strconv.ParseBool(v)
		case customKeyAdditions:
			result.Additions, _ = strconv.Atoi(v)
		case customKeyChanges:
			result.Changes, _ = strconv.Atoi(v)
		case customKeyDestructions:
			result.Destructions, _ = strconv.Atoi(v)
		case customKeyTerraformVersion:
			result.TerraformVersion = v
		case customKeyTerraformTool:
			result.TerraformTool = v
		default:
			// Pass through non-planfile custom entries.
			cleanCustom[k] = v
		}
	}

	result.Custom = cleanCustom
	return result
}

// prefixToQuery converts a prefix string to an artifact query.
// The prefix follows the default key pattern: {{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan.
func prefixToQuery(prefix string) artifact.Query {
	if prefix == "" {
		return artifact.Query{All: true}
	}

	parts := strings.SplitN(prefix, "/", 3)

	query := artifact.Query{}

	if len(parts) >= 1 && parts[0] != "" {
		query.Stacks = []string{parts[0]}
	}

	if len(parts) >= 2 && parts[1] != "" {
		query.Components = []string{parts[1]}
	}

	if len(parts) >= 3 && parts[2] != "" {
		query.SHAs = []string{parts[2]}
	}

	return query
}
