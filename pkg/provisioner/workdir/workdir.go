package workdir

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// HookEventBeforeTerraformInit is the hook event for before terraform init.
const HookEventBeforeTerraformInit = provisioner.HookEvent("before.terraform.init")

func init() {
	// Register workdir provisioner to run before terraform init.
	_ = provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "workdir",
		HookEvent: HookEventBeforeTerraformInit,
		Func:      ProvisionWorkdir,
	})
}

// Service coordinates workdir provisioning operations.
type Service struct {
	fs         FileSystem
	cache      Cache
	downloader Downloader
	hasher     Hasher
}

// NewService creates a new workdir service with default implementations.
func NewService() *Service {
	defer perf.Track(nil, "workdir.NewService")()

	return &Service{
		fs:         NewDefaultFileSystem(),
		cache:      NewDefaultCache(),
		downloader: NewDefaultDownloader(),
		hasher:     NewDefaultHasher(),
	}
}

// NewServiceWithDeps creates a new workdir service with injected dependencies.
func NewServiceWithDeps(fs FileSystem, cache Cache, downloader Downloader, hasher Hasher) *Service {
	defer perf.Track(nil, "workdir.NewServiceWithDeps")()

	return &Service{
		fs:         fs,
		cache:      cache,
		downloader: downloader,
		hasher:     hasher,
	}
}

// ProvisionWorkdir creates an isolated working directory and populates it with source files.
// This is the main provisioner function registered with the provisioner registry.
//
// Activation rules:
// - Runs if metadata.source is present (JIT vendoring)
// - Runs if metadata.workdir: true (explicit opt-in for local components)
// - Does nothing otherwise (terraform runs in original component directory).
func ProvisionWorkdir(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) error {
	defer perf.Track(atmosConfig, "workdir.ProvisionWorkdir")()

	service := NewService()
	return service.Provision(ctx, atmosConfig, componentConfig)
}

// Provision creates an isolated working directory and populates it with source files.
func (s *Service) Provision(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
) error {
	defer perf.Track(atmosConfig, "workdir.Service.Provision")()

	// Check activation conditions.
	sourceConfig := extractSourceConfig(componentConfig)
	workdirOptIn := isWorkdirEnabled(componentConfig)

	if sourceConfig == nil && !workdirOptIn {
		// No workdir needed - terraform runs in original directory.
		return nil
	}

	// Get component name.
	component, ok := componentConfig[ComponentKey].(string)
	if !ok {
		component = extractComponentName(componentConfig)
	}
	if component == "" {
		return errUtils.Build(errUtils.ErrWorkdirProvision).
			WithExplanation("component name not found in configuration").
			Err()
	}

	_ = ui.Info(fmt.Sprintf("Provisioning workdir for component '%s'", component))

	// 1. Create .workdir/terraform/<component>/ directory.
	workdirPath, err := s.createWorkdirDirectory(atmosConfig, component)
	if err != nil {
		return err
	}

	// 2. Populate workdir with source files.
	var metadata *WorkdirMetadata
	if sourceConfig != nil {
		// Download to XDG cache if needed, copy to workdir.
		metadata, err = s.downloadAndCopyToWorkdir(ctx, atmosConfig, sourceConfig, workdirPath, component)
		if err != nil {
			return err
		}
	} else {
		// Copy from local component to workdir.
		metadata, err = s.copyLocalToWorkdir(atmosConfig, componentConfig, workdirPath, component)
		if err != nil {
			return err
		}
	}

	// 3. Write workdir metadata.
	if err := s.writeMetadata(workdirPath, metadata); err != nil {
		return err
	}

	// 4. Store workdir path for terraform execution.
	componentConfig[WorkdirPathKey] = workdirPath

	_ = ui.Success(fmt.Sprintf("Workdir provisioned: %s", workdirPath))
	return nil
}

// createWorkdirDirectory creates the workdir directory structure.
func (s *Service) createWorkdirDirectory(atmosConfig *schema.AtmosConfiguration, component string) (string, error) {
	defer perf.Track(atmosConfig, "workdir.Service.createWorkdirDirectory")()

	basePath := atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}

	workdirPath := filepath.Join(basePath, WorkdirPath, "terraform", component)

	if err := s.fs.MkdirAll(workdirPath, DirPermissions); err != nil {
		return "", errUtils.Build(errUtils.ErrWorkdirCreation).
			WithCause(err).
			WithExplanation("failed to create workdir directory").
			WithContext("path", workdirPath).
			Err()
	}

	return workdirPath, nil
}

// downloadAndCopyToWorkdir downloads source to cache and copies to workdir.
func (s *Service) downloadAndCopyToWorkdir(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	sourceConfig *SourceConfig,
	workdirPath, component string,
) (*WorkdirMetadata, error) {
	defer perf.Track(atmosConfig, "workdir.Service.downloadAndCopyToWorkdir")()

	// Generate cache key.
	cacheKey := s.cache.GenerateKey(sourceConfig)

	// Check if already cached.
	cachedPath := s.cache.Path(cacheKey)
	if cachedPath == "" {
		// Download to cache.
		uri := buildFullURI(sourceConfig)
		_ = ui.Info(fmt.Sprintf("Downloading source: %s", uri))

		if err := s.downloadToCache(ctx, sourceConfig, cacheKey); err != nil {
			return nil, err
		}
		cachedPath = s.cache.Path(cacheKey)
	} else {
		_ = ui.Info(fmt.Sprintf("Using cached source: %s", cacheKey[:12]))
	}

	// Copy from cache to workdir.
	if err := s.fs.CopyDir(cachedPath, workdirPath); err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirSync).
			WithCause(err).
			WithExplanation("failed to copy from cache to workdir").
			WithContext("cache_path", cachedPath).
			WithContext("workdir_path", workdirPath).
			Err()
	}

	// Compute content hash.
	contentHash, _ := s.hasher.HashDir(workdirPath)

	now := time.Now()
	return &WorkdirMetadata{
		Component:     component,
		SourceType:    SourceTypeRemote,
		SourceURI:     sourceConfig.URI,
		SourceVersion: sourceConfig.Version,
		CacheKey:      cacheKey,
		CreatedAt:     now,
		UpdatedAt:     now,
		ContentHash:   contentHash,
	}, nil
}

// downloadToCache downloads the source to the XDG cache.
func (s *Service) downloadToCache(ctx context.Context, sourceConfig *SourceConfig, cacheKey string) error {
	defer perf.Track(nil, "workdir.Service.downloadToCache")()

	// Create temp directory for download.
	tempDir := filepath.Join(s.cache.Path(""), ".tmp", cacheKey)
	if err := s.fs.MkdirAll(tempDir, DirPermissions); err != nil {
		return errUtils.Build(errUtils.ErrSourceDownload).
			WithCause(err).
			WithExplanation("failed to create temp directory for download").
			Err()
	}
	defer func() {
		if err := s.fs.RemoveAll(tempDir); err != nil {
			// Log but don't fail on cleanup error.
			_ = ui.Warning(fmt.Sprintf("Failed to clean up temp directory: %s", err))
		}
	}()

	// Download source.
	uri := buildFullURI(sourceConfig)
	if err := s.downloader.Download(ctx, uri, tempDir); err != nil {
		return errUtils.Build(errUtils.ErrSourceDownload).
			WithCause(err).
			WithExplanation("failed to download component source").
			WithContext("uri", uri).
			WithHint("Check the source URI is valid and accessible").
			Err()
	}

	// Put in cache.
	entry := &CacheEntry{
		Key:            cacheKey,
		URI:            sourceConfig.URI,
		Version:        sourceConfig.Version,
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
	}

	// Set TTL based on cache policy.
	if s.cache.GetPolicy(sourceConfig) == CachePolicyTTL {
		entry.TTL = DefaultCacheTTL
	}

	if err := s.cache.Put(cacheKey, tempDir, entry); err != nil {
		return errUtils.Build(errUtils.ErrSourceCacheWrite).
			WithCause(err).
			WithExplanation("failed to store source in cache").
			Err()
	}

	return nil
}

// copyLocalToWorkdir copies local component files to workdir.
func (s *Service) copyLocalToWorkdir(
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	workdirPath, component string,
) (*WorkdirMetadata, error) {
	defer perf.Track(atmosConfig, "workdir.Service.copyLocalToWorkdir")()

	// Get component path.
	componentPath := extractComponentPath(atmosConfig, componentConfig, component)
	if componentPath == "" {
		return nil, errUtils.Build(errUtils.ErrWorkdirProvision).
			WithExplanation("cannot determine local component path").
			WithContext("component", component).
			Err()
	}

	// Verify source exists.
	if !s.fs.Exists(componentPath) {
		return nil, errUtils.Build(errUtils.ErrWorkdirProvision).
			WithExplanation("local component path does not exist").
			WithContext("path", componentPath).
			WithHint("Check that the component exists in components/terraform/").
			Err()
	}

	_ = ui.Info(fmt.Sprintf("Copying local component: %s", componentPath))

	// Copy to workdir.
	if err := s.fs.CopyDir(componentPath, workdirPath); err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirSync).
			WithCause(err).
			WithExplanation("failed to copy local component to workdir").
			WithContext("source", componentPath).
			WithContext("dest", workdirPath).
			Err()
	}

	// Compute content hash.
	contentHash, _ := s.hasher.HashDir(workdirPath)

	now := time.Now()
	return &WorkdirMetadata{
		Component:   component,
		SourceType:  SourceTypeLocal,
		LocalPath:   componentPath,
		CreatedAt:   now,
		UpdatedAt:   now,
		ContentHash: contentHash,
	}, nil
}

// writeMetadata writes workdir metadata to the metadata file.
func (s *Service) writeMetadata(workdirPath string, metadata *WorkdirMetadata) error {
	defer perf.Track(nil, "workdir.Service.writeMetadata")()

	metadataPath := filepath.Join(workdirPath, WorkdirMetadataFile)
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("failed to marshal workdir metadata").
			Err()
	}

	if err := s.fs.WriteFile(metadataPath, data, FilePermissionsStandard); err != nil {
		return errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("failed to write workdir metadata").
			WithContext("path", metadataPath).
			Err()
	}

	return nil
}

// extractSourceConfig extracts metadata.source from component config.
func extractSourceConfig(componentConfig map[string]any) *SourceConfig {
	defer perf.Track(nil, "workdir.extractSourceConfig")()

	metadata := getMetadata(componentConfig)
	if metadata == nil {
		return nil
	}

	source, ok := metadata["source"]
	if !ok {
		return nil
	}

	// Handle string form (just URI).
	if uri, ok := source.(string); ok {
		return &SourceConfig{URI: uri}
	}

	// Handle map form (structured config).
	if sourceMap, ok := source.(map[string]any); ok {
		return parseSourceMap(sourceMap)
	}

	return nil
}

// getMetadata extracts the metadata map from component config.
func getMetadata(componentConfig map[string]any) map[string]any {
	metadata, ok := componentConfig["metadata"].(map[string]any)
	if !ok {
		return nil
	}
	return metadata
}

// parseSourceMap parses a structured source configuration map.
func parseSourceMap(sourceMap map[string]any) *SourceConfig {
	config := &SourceConfig{}

	// Extract URI.
	if uri, ok := sourceMap["uri"].(string); ok {
		config.URI = uri
	}

	// Extract version.
	if version, ok := sourceMap["version"].(string); ok {
		config.Version = version
	}

	// Extract included paths.
	config.IncludedPaths = extractStringSlice(sourceMap, "included_paths")

	// Extract excluded paths.
	config.ExcludedPaths = extractStringSlice(sourceMap, "excluded_paths")

	// Only return config if URI is present.
	if config.URI == "" {
		return nil
	}

	return config
}

// extractStringSlice extracts a string slice from a map key.
func extractStringSlice(m map[string]any, key string) []string {
	var result []string

	slice, ok := m[key].([]any)
	if !ok {
		return result
	}

	for _, item := range slice {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}

	return result
}

// isWorkdirEnabled checks if metadata.workdir is set to true.
func isWorkdirEnabled(componentConfig map[string]any) bool {
	defer perf.Track(nil, "workdir.isWorkdirEnabled")()

	metadata, ok := componentConfig["metadata"].(map[string]any)
	if !ok {
		return false
	}

	workdir, ok := metadata["workdir"].(bool)
	return ok && workdir
}

// extractComponentName extracts the component name from config.
func extractComponentName(componentConfig map[string]any) string {
	defer perf.Track(nil, "workdir.extractComponentName")()

	// Try component field.
	if component, ok := componentConfig[ComponentKey].(string); ok && component != "" {
		return component
	}

	// Try metadata.component.
	if metadata, ok := componentConfig["metadata"].(map[string]any); ok {
		if component, ok := metadata[ComponentKey].(string); ok && component != "" {
			return component
		}
	}

	// Try vars.component as fallback.
	if vars, ok := componentConfig["vars"].(map[string]any); ok {
		if component, ok := vars[ComponentKey].(string); ok && component != "" {
			return component
		}
	}

	return ""
}

// extractComponentPath extracts the local component path.
func extractComponentPath(atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, component string) string {
	defer perf.Track(atmosConfig, "workdir.extractComponentPath")()

	basePath := atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}

	// Check for component_path in config.
	if componentPath, ok := componentConfig["component_path"].(string); ok && componentPath != "" {
		return componentPath
	}

	// Build default path.
	componentsBasePath := atmosConfig.Components.Terraform.BasePath
	if componentsBasePath == "" {
		componentsBasePath = "components/terraform"
	}

	return filepath.Join(basePath, componentsBasePath, component)
}

// buildFullURI constructs the full URI including version.
func buildFullURI(source *SourceConfig) string {
	defer perf.Track(nil, "workdir.buildFullURI")()

	if source.Version == "" {
		return source.URI
	}

	// Check if URI already has a ref.
	if containsRef(source.URI) {
		return source.URI
	}

	// Append version as ref.
	separator := "?"
	if containsQuery(source.URI) {
		separator = "&"
	}
	return source.URI + separator + "ref=" + source.Version
}

// containsRef checks if the URI already contains a ref parameter.
func containsRef(uri string) bool {
	return containsParam(uri, "ref=")
}

// containsQuery checks if the URI contains a query string.
func containsQuery(uri string) bool {
	for _, c := range uri {
		if c == '?' {
			return true
		}
	}
	return false
}

// containsParam checks if the URI contains a specific parameter.
func containsParam(uri, param string) bool {
	for i := 0; i <= len(uri)-len(param); i++ {
		if uri[i:i+len(param)] == param {
			return true
		}
	}
	return false
}
