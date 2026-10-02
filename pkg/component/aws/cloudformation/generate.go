package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
)

// generationEnabled requires both the global opt-in and a nonempty component generate block.
func generationEnabled(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) bool {
	return atmosConfig.Components.CloudFormation.AutoGenerateFiles && len(tfgenerate.GetGenerateSectionFromComponent(info.ComponentSection)) > 0
}

// prepareGeneration normalizes isolation and validates the generated template reference.
func prepareGeneration(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	section, err := PrepareSourceComponentConfig(atmosConfig, info.ComponentSection)
	if err != nil {
		return err
	}
	if generationEnabled(atmosConfig, info) && !isTemplatePresent(section[config.TemplateSectionName]) {
		if path, _ := section[config.TemplatePathSectionName].(string); path == "" {
			return errUtils.Build(errUtils.ErrMissingAwsCloudFormationTemplate).
				WithHint("Set path to the template filename produced by generate.").Err()
		}
	}
	info.ComponentSection = section
	return nil
}

// PrepareSourceComponentConfig applies the same generation workdir policy to source
// management and execution. It neither generates files nor requires a template path.
// Inherited configuration maps remain unchanged.
func PrepareSourceComponentConfig(atmosConfig *schema.AtmosConfiguration, section map[string]any) (map[string]any, error) {
	defer perf.Track(atmosConfig, "cloudformation.PrepareSourceComponentConfig")()
	if !generationEnabled(atmosConfig, &schema.ConfigAndStacksInfo{ComponentSection: section}) {
		return section, nil
	}
	for _, name := range []string{config.MetadataSectionName, config.SettingsSectionName} {
		block, _ := section[name].(map[string]any)
		if override, _ := block["working_directory"].(string); override != "" {
			return nil, errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
				WithExplanationf("CloudFormation generate blocks cannot use %s.working_directory.", name).
				WithHint("Remove the working_directory override to use the component's isolated workdir.").Err()
		}
	}
	section = maps.Clone(section)
	provision, _ := section[config.ProvisionSectionName].(map[string]any)
	provision = cloneOrEmpty(provision)
	working, _ := provision["workdir"].(map[string]any)
	working = cloneOrEmpty(working)
	if enabled, ok := working["enabled"].(bool); ok && !enabled {
		return nil, errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
			WithExplanation("CloudFormation generate blocks require an isolated workdir.").
			WithHint("Remove provision.workdir.enabled: false or set it to true.").Err()
	}
	working["enabled"] = true
	provision["workdir"] = working
	section[config.ProvisionSectionName] = provision
	return section, nil
}

// cloneOrEmpty returns a writable shallow copy without mutating inherited configuration.
func cloneOrEmpty(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return maps.Clone(value)
}

// prepareComponentFiles provisions source/workdir before generating template and policy files.
func prepareComponentFiles(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	if err := prepareGeneration(atmosConfig, info); err != nil {
		return "", err
	}
	path, err := resolveComponentPath(atmosConfig, info)
	if err != nil {
		return "", err
	}
	ctx = downloader.WithAWSAuthResolver(ctx, func(context.Context) (*schema.AWSAuthContext, error) {
		return resolveSourceAWSAuth(atmosConfig, info)
	})
	path, _, err = provisionAndResolveComponentPath(ctx, provisioner.OutputWriters{}, atmosConfig, info, config.CloudFormationComponentType, path)
	if err != nil {
		return "", err
	}
	if err := generateComponentFiles(atmosConfig, info, path); err != nil {
		return "", err
	}
	return path, nil
}

// resolveSourceAWSAuth runs only when the S3 downloader needs credentials. Local
// templates, other source protocols and a warm source cache never authenticate.
func resolveSourceAWSAuth(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.AWSAuthContext, error) {
	defer perf.Track(atmosConfig, "cloudformation.resolveSourceAWSAuth")()
	if info.DryRun || info.AuthDisabled || authdeferred.AuthDisabled(atmosConfig.AuthManager) {
		return nil, nil
	}
	if info.AuthContext != nil {
		return info.AuthContext.AWS, nil
	}
	identity := config.NormalizeIdentityValue(info.Identity)
	if identity == config.IdentityFlagDisabledValue {
		return nil, nil
	}
	if identity == config.IdentityFlagSelectValue {
		identity = ""
	}
	resolved, err := authdeferred.Credentials(atmosConfig, info, identity).Resolve()
	if err != nil {
		return nil, err
	}
	info.AuthManager, info.AuthContext = resolved.AuthManager, resolved.AuthContext
	if info.AuthContext == nil {
		return nil, nil
	}
	return info.AuthContext.AWS, nil
}

// generateComponentFiles writes into the isolated workdir and reports both batch and per-file failures.
func generateComponentFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, path string) error {
	defer perf.Track(atmosConfig, "cloudformation.generateComponentFiles")()
	if info.DryRun || !generationEnabled(atmosConfig, info) {
		return nil
	}
	root, err := workdir.BuildPath(atmosConfig.BasePath, config.CloudFormationComponentType, info.FinalComponent, info.Stack, info.ComponentSection)
	if err != nil {
		return err
	}
	if err := requireGeneratedPath(root, path); err != nil {
		return err
	}
	section := tfgenerate.GetGenerateSectionFromComponent(info.ComponentSection)
	if err := prepareGeneratedDirectories(path, section); err != nil {
		return err
	}
	results, err := tfgenerate.GenerateFiles(section, path, tfgenerate.BuildTemplateContext(info), tfgenerate.GenerateConfig{})
	for _, result := range results {
		if result.Error != nil {
			err = errors.Join(err, fmt.Errorf("generate %s: %w", result.Filename, result.Error))
		}
	}
	if err != nil {
		return errors.Join(errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}
	return nil
}

// prepareGeneratedDirectories validates every destination before creating any parent directories.
func prepareGeneratedDirectories(path string, section map[string]any) error {
	for name := range section {
		if filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
			return invalidGeneratedPath(name)
		}
		if err := requireGeneratedPath(path, filepath.Join(path, name)); err != nil {
			return err
		}
	}
	for name := range section {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(path, name)), workdir.DirPermissions); err != nil {
			return errors.Join(errUtils.ErrInvalidAwsCloudFormationSettings, err)
		}
	}
	return nil
}

// requireGeneratedPath rejects lexical and symlink escapes from the workdir, including missing leaves.
func requireGeneratedPath(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return invalidGeneratedPath(path)
	}
	resolvedRoot, err := resolveExistingAncestor(root)
	if err != nil {
		return errors.Join(errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}
	resolvedPath, err := resolveExistingAncestor(path)
	if err != nil {
		return errors.Join(errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}
	relative, err = filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return invalidGeneratedPath(path)
	}
	return nil
}

// resolveExistingAncestor resolves symlinks even when the generated leaf does not exist yet.
func resolveExistingAncestor(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(absolute); err == nil {
		return filepath.EvalSymlinks(absolute)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent, err := resolveExistingAncestor(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

// invalidGeneratedPath explains how to keep generated files inside the component workdir.
func invalidGeneratedPath(path string) error {
	return errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
		WithExplanationf("Generated path %q must remain inside the component's isolated workdir.", path).
		WithHint("Use relative generate filenames and remove shared working_directory overrides.").Err()
}
