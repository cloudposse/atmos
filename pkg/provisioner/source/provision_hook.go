package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/duration"
	"github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// HookEventBeforeTerraformInit is the hook event for before terraform init.
const HookEventBeforeTerraformInit = provisioner.HookEvent("before.terraform.init")

// DirPermissions is the default permission mode for directories.
const DirPermissions = 0o755

// invocationDoneKey is the componentConfig key set after AutoProvisionSource completes
// (whether it provisioned or skipped). It prevents re-provisioning within the same command
// invocation when the source provisioner is called both from resolveAndProvisionComponentPath
// and from the before.terraform.init hook (prepareInitExecution). Without this guard a zero
// TTL deletes the varfiles and backend config written between the two calls.
const invocationDoneKey = "_atmos_source_provisioned"

func init() {
	// Register source provisioner to run before terraform init.
	// This enables JIT (Just-in-Time) source vendoring on first use.
	_ = provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "source",
		HookEvent: HookEventBeforeTerraformInit,
		Func:      autoProvisionSourceTerraform,
	})
}

// autoProvisionSourceTerraform is a wrapper for the hook system that always uses terraform as component type.
// This matches the HookEventBeforeTerraformInit event.
func autoProvisionSourceTerraform(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
	writers provisioner.OutputWriters,
	_ *provisioner.TerraformExecContext,
) error {
	return AutoProvisionSource(ctx, atmosConfig, cfg.TerraformComponentType, componentConfig, authContext, writers)
}

// AutoProvisionSource automatically vendors component source on first use.
// This enables JIT (Just-in-Time) vendoring - sources are fetched automatically
// when running terraform/helmfile/packer commands if the target directory doesn't exist.
//
// Parameters:
// - componentType: the type of component ("terraform", "helmfile", or "packer").
//
// Behavior:
// - If component has no source configured: skip (not an error).
// - If target directory already exists: skip (use CRUD commands for updates).
// - If workdir is enabled: download source directly to workdir path.
// - If workdir is NOT enabled: download source to component path.
func AutoProvisionSource(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentType string,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
	writers provisioner.OutputWriters,
) (retErr error) {
	defer perf.Track(atmosConfig, "source.AutoProvisionSource")()
	out := ui.New(writers.Stderr)
	if err := ctx.Err(); err != nil {
		return err
	}

	sourceSpec, component, err := extractSourceAndComponent(componentConfig)
	if err != nil {
		return err
	}
	if sourceSpec == nil {
		return nil // No source configured - skip.
	}

	// Apply global TTL default if not set per-component.
	applyGlobalTTLDefault(sourceSpec, atmosConfig, componentType)

	stack, _ := componentConfig["atmos_stack"].(string)

	// ResolveTarget is the single destination resolver shared with `source pull` / `source delete`.
	target, err := ResolveTarget(atmosConfig, componentType, component, componentConfig)
	if err != nil {
		return wrapProvisionError(err, "Failed to determine target directory", component)
	}
	if err := validateTargetOwner(target, stack); err != nil {
		return err
	}
	targetDir, isWorkdir := target.Dir, target.IsWorkdir

	key := provisioningKey(target, componentType, componentConfig, sourceSpec, authContext)
	marker := provisionedMarker{key: key, invocation: currentInvocation.Load()}
	if done, ok := componentConfig[invocationDoneKey].(provisionedMarker); ok && marker.invocation == nil && done == marker && isNonEmptyDir(targetDir) {
		if isWorkdir {
			setProvisionedWorkdir(componentConfig, targetDir)
		}
		return nil
	}
	reused, finish := beginProvision(key, targetDir)
	if reused {
		if isWorkdir {
			setProvisionedWorkdir(componentConfig, targetDir)
		}
		componentConfig[invocationDoneKey] = marker
		return nil
	}
	defer func() {
		finish(retErr == nil)
		if retErr == nil {
			componentConfig[invocationDoneKey] = marker
		}
	}()

	// Check if provisioning is needed (version change, URI change, or fresh workdir).
	needs, reason := needsProvisioning(targetDir, sourceSpec, isWorkdir)
	if !needs {
		// No provisioning needed - touch LastAccessed and return.
		if isWorkdir {
			if err := workdir.UpdateLastAccessed(targetDir); err != nil {
				// Non-critical error - log and continue.
				out.Warningf("Failed to update workdir last accessed time: %s", err)
			}
			setProvisionedWorkdir(componentConfig, targetDir)
		}
		return nil
	}

	// Log reason for re-provisioning.
	if reason != "" {
		if !workdir.OutputSuppressed(ctx) {
			ui.Info(reason)
		} else if writers.Stderr != nil {
			out.Info(reason)
		}
	}

	if authContext != nil {
		ctx = downloader.WithAWSAuthContext(ctx, authContext.AWS)
	}

	hadSource := isNonEmptyDir(targetDir)
	// Vendor the source to target directory.
	var receipt downloader.ResolvedArtifact
	options := []VendorSourceOption{WithArtifactReceipt(&receipt)}
	if componentType == cfg.TerraformComponentType {
		options = append(options, WithRuntimePreservation(componentConfig), withProvenance(target, stack, sourceSpec, &receipt))
	}
	if err := vendorToTarget(ctx, atmosConfig, sourceSpec, vendorTarget{path: targetDir, component: component, writers: writers, options: options, preserveRuntime: componentType == cfg.TerraformComponentType}); err != nil {
		return err
	}

	// Record provenance: workdir metadata for workdir targets, the `.atmos/source.json` marker for
	// shared component directories. Non-critical: a failure only limits `source delete`.
	if componentType != cfg.TerraformComponentType {
		if err := recordProvenance(target, stack, sourceSpec, &receipt); err != nil {
			out.Warningf("Failed to record source provenance: %s", err)
		}
	}
	if isWorkdir {
		setProvisionedWorkdir(componentConfig, targetDir)
		// Only first materialization has no backend cache. Refresh preserves .terraform;
		// normal init fingerprinting decides whether its actual inputs changed.
		if !hadSource {
			componentConfig[workdir.WorkdirReprovisionedKey] = struct{}{}
		}
	}
	return nil
}

// applyGlobalTTLDefault sets the source TTL from the global config if not already set per-component.
func applyGlobalTTLDefault(sourceSpec *schema.VendorComponentSource, atmosConfig *schema.AtmosConfiguration, componentType string) {
	if sourceSpec.TTL != "" {
		return
	}
	switch componentType {
	case cfg.TerraformComponentType:
		if atmosConfig.Components.Terraform.Source != nil {
			sourceSpec.TTL = atmosConfig.Components.Terraform.Source.TTL
		}
	case cfg.HelmfileComponentType:
		if atmosConfig.Components.Helmfile.Source != nil {
			sourceSpec.TTL = atmosConfig.Components.Helmfile.Source.TTL
		}
	case cfg.PackerComponentType:
		if atmosConfig.Components.Packer.Source != nil {
			sourceSpec.TTL = atmosConfig.Components.Packer.Source.TTL
		}
	case cfg.CloudFormationComponentType:
		if atmosConfig.Components.CloudFormation.Source != nil {
			sourceSpec.TTL = atmosConfig.Components.CloudFormation.Source.TTL
		}
	}
}

// extractSourceAndComponent extracts source spec and component name from config.
func extractSourceAndComponent(componentConfig map[string]any) (*schema.VendorComponentSource, string, error) {
	sourceSpec, err := ExtractSource(componentConfig)
	if err != nil {
		return nil, "", errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Invalid source configuration").
			Err()
	}
	if sourceSpec == nil {
		return nil, "", nil
	}

	component := extractComponentName(componentConfig)
	if component == "" {
		return nil, "", errUtils.Build(errUtils.ErrSourceProvision).
			WithExplanation("Component name not found in configuration").
			Err()
	}
	return sourceSpec, component, nil
}

// vendorTarget identifies the provisioning destination and progress output streams.
type vendorTarget struct {
	path            string
	component       string
	writers         provisioner.OutputWriters
	options         []VendorSourceOption
	preserveRuntime bool
}

// vendorToTarget provisions with progress reporting and removes a newly created target when downloading fails.
func vendorToTarget(ctx context.Context, atmosConfig *schema.AtmosConfiguration, sourceSpec *schema.VendorComponentSource, target vendorTarget) error {
	progressMsg := fmt.Sprintf("Auto-provisioning source for %q", target.component)
	completedMsg := fmt.Sprintf("Auto-provisioned source to %s", target.path)
	out := ui.New(target.writers.Stderr)

	operation := func() error {
		// Transactional Terraform refresh owns creation/publication under its target lock.
		// An outer "created target" cleanup can otherwise delete a concurrent winner.
		createdTarget, err := prepareAutoVendorTarget(target.path, target.preserveRuntime)
		if err != nil {
			return err
		}

		if err := VendorSource(ctx, atmosConfig, sourceSpec, target.path, target.options...); err != nil {
			if createdTarget {
				if rmErr := os.RemoveAll(target.path); rmErr != nil {
					out.Warningf("Failed to clean up target directory after failed provisioning: %s", rmErr)
				}
			}
			return provisionFailure(err, "Failed to auto-provision component source", map[string]string{
				"component": target.component,
				"source":    sourceSpec.Uri,
				"target":    target.path,
			}, "Verify source URI is accessible and credentials are valid")
		}

		return nil
	}
	if workdir.OutputSuppressed(ctx) {
		if err := operation(); err != nil {
			return err
		}
		if target.writers.Stderr != nil {
			out.Success(completedMsg)
		}
		return nil
	}
	return spinner.ExecWithSpinner(progressMsg, completedMsg, operation)
}

// wrapProvisionError wraps an error with provision context.
func wrapProvisionError(err error, explanation, component string) error {
	return provisionFailure(err, explanation, map[string]string{"component": component}, "")
}

// determineSourceTargetDirectory determines where to download the source.
// Returns the target directory path and whether it's a workdir path.
//
// Priority:
// 1. If workdir is enabled: .workdir/<componentType>/<stack>-<component>/.
// 2. Otherwise: components/<componentType>/<component>/.
func determineSourceTargetDirectory(
	atmosConfig *schema.AtmosConfiguration,
	componentType string,
	component string,
	componentConfig map[string]any,
) (string, bool, error) {
	// Check if workdir is enabled.
	if workdir.IsWorkdirEnabled(componentConfig) {
		// Get stack name for workdir path.
		stack, _ := componentConfig["atmos_stack"].(string)
		if stack == "" {
			return "", false, errUtils.Build(errUtils.ErrSourceProvision).
				WithExplanation("Stack name required when workdir is enabled").
				WithHint("The 'atmos_stack' field is required for workdir provisioning").
				Err()
		}

		basePath := atmosConfig.BasePath
		if basePath == "" {
			basePath = "."
		}

		workdirPath, err := workdir.BuildPath(basePath, componentType, component, stack, componentConfig)
		if err != nil {
			return "", false, err
		}
		workdirPath, err = sourceWorkdirOverride(atmosConfig, componentConfig, workdirPath)
		return workdirPath, true, err
	}

	// No workdir - use standard component path.
	targetDir, err := DetermineTargetDirectory(atmosConfig, componentType, component, componentConfig)
	if err != nil {
		return "", false, err
	}
	return targetDir, false, nil
}

// needsProvisioning checks if the target directory needs provisioning.
// Returns (true, reason) if:
//   - Directory doesn't exist or is empty.
//   - Version has changed from what's in metadata.
//   - URI has changed from what's in metadata.
//   - No metadata exists (fresh workdir).
//   - A remote source TTL expires (workdir or a shared directory with provenance).
//
// Returns (false, "") if the cached source can be reused.
func needsProvisioning(targetDir string, sourceSpec *schema.VendorComponentSource, isWorkdir bool) (bool, string) {
	// Check if directory exists and has content.
	if !isNonEmptyDir(targetDir) {
		return true, ""
	}

	// Shared directories retain their contents unless an owned remote source expires.
	if !isWorkdir {
		return sharedSourceExpired(targetDir, sourceSpec)
	}

	// Directory exists and has content - check metadata for version/URI changes.
	metadata, err := workdir.ReadMetadata(targetDir)
	if err != nil || metadata == nil {
		return true, "No metadata found, re-provisioning"
	}

	// Check for version/URI changes.
	changed, reason := checkMetadataChanges(metadata, sourceSpec)
	if changed {
		return changed, reason
	}

	// Check TTL-based cache expiration.
	if sourceSpec.TTL != "" {
		expired, ttlReason := isSourceCacheExpired(sourceSpec.TTL, metadata.UpdatedAt)
		if expired {
			return true, ttlReason
		}
	}

	return false, ""
}

// isNonEmptyDir checks if the path exists, is a directory, and contains at least one entry
// (excluding the .atmos metadata directory).
func isNonEmptyDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}

	// Count entries excluding .atmos metadata directory.
	for _, entry := range entries {
		if entry.Name() != workdir.AtmosDir {
			return true
		}
	}
	return false
}

// checkMetadataChanges compares metadata with source spec for version/URI changes.
// Returns (true, reason) if changes detected, (false, "") if up-to-date.
func checkMetadataChanges(metadata *workdir.WorkdirMetadata, sourceSpec *schema.VendorComponentSource) (bool, string) {
	// Check for version changes (including version removal).
	if sourceSpec.Version != metadata.SourceVersion {
		oldVer := metadata.SourceVersion
		newVer := sourceSpec.Version
		if oldVer == "" {
			oldVer = "(none)"
		}
		if newVer == "" {
			newVer = "(none)"
		}
		return true, fmt.Sprintf("Source version changed (%s → %s)", oldVer, newVer)
	}

	// SourceURI is a redacted provenance label; query strings may select a
	// different artifact. Compare the original cache source when it is available.
	previous := metadata.Source
	if previous == "" {
		previous = metadata.SourceURI
	}
	if sourceSpec.Uri != previous {
		return true, fmt.Sprintf("Source URI changed (%s → %s)", downloader.RedactSource(previous), downloader.RedactSource(sourceSpec.Uri))
	}

	return false, ""
}

// isSourceCacheExpired checks if the source cache has expired based on TTL.
// A TTL of "0" or "0s" means always expired (always re-pull). The expiry decision
// is delegated to the shared duration.IsExpired helper; this wrapper adds the
// source-provisioning-specific human-readable reason strings.
func isSourceCacheExpired(ttl string, updatedAt time.Time) (bool, string) {
	// Handle zero TTL explicitly (always expired) so we can tailor the reason.
	if isZeroTTL(ttl) {
		return true, fmt.Sprintf("Source cache expired (TTL: %s, always re-pull)", ttl)
	}

	expired, err := duration.IsExpired(updatedAt, ttl)
	if err != nil {
		return true, fmt.Sprintf("Invalid source TTL %q; forcing re-provision to avoid stale cache", ttl)
	}

	if expired {
		return true, fmt.Sprintf("Source cache expired (TTL: %s, last updated: %s)",
			ttl, updatedAt.Format(time.RFC3339))
	}
	return false, ""
}

// isZeroTTL checks if the TTL string represents a zero duration.
func isZeroTTL(ttl string) bool {
	return duration.IsZeroTTL(ttl)
}

// scpStyleHostPattern extracts an optional user and the host from an SCP-style Git URI, e.g.
// "git@ghe.example.com:org/repo.git", the userless "ghe.example.com:org/repo.git" for a dotted
// GHES host, or, for a single-label GHES host, "git@ghe:org/repo.git" (still requiring
// "user@" -- see isSCPStyleGHESRemote). Matches pkg/vendor's scpURLPattern and
// CustomGitDetector.rewriteSCPURL's SCP detection in pkg/downloader/.
var scpStyleHostPattern = regexp.MustCompile(`^(?:([\w.-]+)@)?([\w.-]+):`)

// isConfiguredGHESHost reports whether host matches the GitHub Enterprise Server host
// configured via RepoEndpoints (GITHUB_SERVER_URL). SCP-style Git URIs carry no port of their
// own -- the colon already separates host from path -- so this compares against the portless
// hostname (Endpoints.Hostname()) rather than IsHost, which would otherwise reject a match
// against a GHES host configured with a non-default port (RepoEndpoints().Host keeps that
// port; see its doc comment).
func isConfiguredGHESHost(host string) bool {
	return strings.EqualFold(host, github.RepoEndpoints().Hostname())
}

// isSCPStyleGHESRemote reports whether uri is an SCP-style Git URI ("[user@]host:path") naming
// the configured GitHub Enterprise Server host. A dotted host (e.g.
// "ghe.example.com:org/repo.git") is recognized as remote even without a "user@" prefix,
// matching pkg/vendor's scpURLPattern and rewriteSCPURL. A single-label host is genuinely
// ambiguous with a local relative path plus a colon-separated suffix (e.g. "dir:file"), so it
// still requires the "user@" prefix to be treated as remote -- the pattern this replaces already
// required it unconditionally, so this only loosens the dotted-host case.
func isSCPStyleGHESRemote(uri string) bool {
	m := scpStyleHostPattern.FindStringSubmatch(uri)
	if m == nil {
		return false
	}
	user, host := m[1], m[2]
	if user == "" && !strings.Contains(host, ".") {
		return false
	}
	return isConfiguredGHESHost(host)
}

// isLocalSource determines if a source URI refers to a local path.
// Local sources start with ".", absolute paths (OS-specific), or are relative paths without remote indicators.
func isLocalSource(uri string) bool {
	// Relative paths starting with . or ..
	if strings.HasPrefix(uri, ".") {
		return true
	}
	// Absolute paths (OS-specific, including Windows paths like C:\...).
	if filepath.IsAbs(uri) {
		return true
	}
	// File scheme.
	if strings.HasPrefix(uri, "file://") {
		return true
	}
	// SCP-style Git URI ([user@]host:org/repo.git) naming the configured GitHub Enterprise
	// Server host. Checked before the remoteIndicators loop below because SCP syntax has no
	// "://" separator, and the GHES host itself isn't in that literal list.
	if isSCPStyleGHESRemote(uri) {
		return false
	}
	// Remote indicators - if any of these are present, it's remote. Deliberately
	// github.com-only (see the equivalent knownHosts comment in pkg/stack/imports/uri.go): a
	// bare hostname can't guess a GitHub Enterprise Server host, and any URI naming one already
	// matches the "://" scheme indicator above.
	remoteIndicators := []string{
		"://",        // Any URL scheme (https://, git://, s3://, etc.).
		"github.com", // GitHub.
		"gitlab.com", // GitLab.
		"bitbucket.", // Bitbucket.
		"git::",      // Git getter prefix.
		"s3::",       // S3 getter prefix.
		"gcs::",      // GCS getter prefix.
	}
	for _, indicator := range remoteIndicators {
		if strings.Contains(uri, indicator) {
			return false
		}
	}
	// Default: paths without remote indicators are local.
	return true
}

// writeWorkdirMetadata writes workdir metadata based on source type (local or remote).
func writeWorkdirMetadata(workdirPath, component, stack string, sourceSpec *schema.VendorComponentSource, receipts ...*downloader.ResolvedArtifact) error {
	now := time.Now()

	// Read existing metadata to preserve CreatedAt if it exists.
	existingMetadata, _ := workdir.ReadMetadata(workdirPath)

	// Determine source type based on URI.
	sourceType := workdir.SourceTypeRemote
	if isLocalSource(sourceSpec.Uri) {
		sourceType = workdir.SourceTypeLocal
	}

	metadata := &workdir.WorkdirMetadata{
		Component:     component,
		Stack:         stack,
		SourceType:    sourceType,
		Source:        sourceSpec.Uri, // Keep source field for backward compatibility.
		SourceURI:     sourceSpec.Uri,
		SourceVersion: sourceSpec.Version,
		CreatedAt:     now,
		UpdatedAt:     now,
		LastAccessed:  now,
		ContentHash:   "", // Content hash is computed separately for local sources.
	}
	// Use the fetch receipt captured before Git metadata is stripped. Compatibility
	// callers without one get an honest content digest, never the enclosing repo HEAD.
	artifact := downloader.ResolvedArtifact{Declared: downloader.RedactSource(sourceSpec.Uri), Resolved: downloader.RedactSource(sourceSpec.Uri)}
	if len(receipts) > 0 && receipts[0] != nil {
		artifact = *receipts[0]
	} else if digest, err := downloader.TreeSHA256(workdirPath); err == nil {
		artifact.Identity = digest
	}
	metadata.SourceURI, metadata.SourceResolved, metadata.SourceIdentity = artifact.Declared, artifact.Resolved, artifact.Identity

	// Preserve original CreatedAt and ContentHash if metadata already existed.
	if existingMetadata != nil {
		metadata.CreatedAt = existingMetadata.CreatedAt
		// Preserve content hash for local sources (may have been computed by workdir provisioner).
		if sourceType == workdir.SourceTypeLocal && existingMetadata.ContentHash != "" {
			metadata.ContentHash = existingMetadata.ContentHash
		}
	}

	return workdir.WriteMetadata(workdirPath, metadata)
}

// extractComponentName extracts the component name from config.
// Priority: componentConfig["component"] > componentConfig["metadata"]["component"] > componentConfig["atmos_component"].
func extractComponentName(componentConfig map[string]any) string {
	// Try component field first (highest priority).
	if component, ok := componentConfig["component"].(string); ok && component != "" {
		return component
	}

	// Fall back to metadata.component.
	if metadata, ok := componentConfig["metadata"].(map[string]any); ok {
		if component, ok := metadata["component"].(string); ok && component != "" {
			return component
		}
	}

	// Fall back to atmos_component (instance name, set by atmos stack processing).
	// This handles components that have no base component override — the instance name
	// (e.g. "producer-from-source") is the canonical name for source resolution.
	if component, ok := componentConfig["atmos_component"].(string); ok && component != "" {
		return component
	}

	return ""
}

// setProvisionedWorkdir does not undo the runtime's already-resolved module subdirectory
// when a later hook reuses the same source provisioning result.
func setProvisionedWorkdir(config map[string]any, target string) {
	if existing, ok := config[workdir.WorkdirPathKey].(string); ok && existing != "" && isWithinBase(existing, target) {
		if info, err := os.Stat(existing); err == nil && info.IsDir() {
			return
		}
	}
	config[workdir.WorkdirPathKey] = target
}

func prepareAutoVendorTarget(path string, preserveRuntime bool) (bool, error) {
	if preserveRuntime {
		return false, nil
	}
	_, statErr := os.Stat(path)
	if err := os.MkdirAll(path, DirPermissions); err != nil {
		return false, errUtils.Build(errUtils.ErrSourceProvision).WithCause(err).WithExplanation("Failed to create target directory").WithContext("path", path).Err()
	}
	return os.IsNotExist(statErr), nil
}
