package vendoring

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

// diffFlags holds flags specific to vendor diff command.
type diffFlags struct {
	Component string
	From      string
	To        string
	File      string
	Context   int
	Unified   bool
	NoColor   bool
}

// Diff executes the vendor diff operation with typed params.
func Diff(atmosConfig *schema.AtmosConfiguration, params *DiffParams) error {
	defer perf.Track(atmosConfig, "vendor.Diff")()

	// Convert params to internal diffFlags format.
	flags := &diffFlags{
		Component: params.Component,
		From:      params.From,
		To:        params.To,
		File:      params.File,
		Context:   params.Context,
		Unified:   params.Unified,
		NoColor:   params.NoColor,
	}

	// Validate component flag is provided.
	if flags.Component == "" {
		return errUtils.ErrComponentFlagRequired
	}

	// Execute vendor diff with the new Git operations interface.
	return executeVendorDiff(atmosConfig, flags)
}

// executeVendorDiff performs the vendor diff logic.
func executeVendorDiff(atmosConfig *schema.AtmosConfiguration, flags *diffFlags) error {
	defer perf.Track(atmosConfig, "vendor.executeVendorDiff")()

	return executeVendorDiffWithGitOps(atmosConfig, flags, NewGitOperations())
}

// executeVendorDiffWithGitOps performs the vendor diff logic with injectable Git operations.
// This function allows for testing with mocked Git operations.
func executeVendorDiffWithGitOps(atmosConfig *schema.AtmosConfiguration, flags *diffFlags, gitOps GitOperations) error {
	defer perf.Track(atmosConfig, "vendor.executeVendorDiffWithGitOps")()

	// Load and validate the component source.
	componentSource, _, err := loadComponentSourceForDiff(atmosConfig, flags)
	if err != nil {
		return err
	}
	if componentSource == nil {
		// Fallback to component vendor diff was attempted.
		return executeComponentVendorDiff(atmosConfig, flags)
	}

	// Verify it's a Git source.
	if !isGitSource(componentSource.Source) {
		return fmt.Errorf("%w: only Git sources are supported for diff", errUtils.ErrUnsupportedVendorSource)
	}

	// Extract Git URI and resolve refs.
	gitURI := version.ExtractGitURI(componentSource.Source)
	fromRef, toRef, err := resolveRefs(flags, componentSource, gitURI, gitOps)
	if err != nil {
		return err
	}

	// Generate and output the diff.
	return generateAndOutputDiff(&diffContext{
		atmosConfig: atmosConfig,
		gitOps:      gitOps,
		gitURI:      gitURI,
		fromRef:     fromRef,
		toRef:       toRef,
		flags:       flags,
	})
}

// loadComponentSourceForDiff loads the vendor config and finds the component source.
// Returns nil componentSource if no vendor config exists (caller should try component vendor diff).
func loadComponentSourceForDiff(
	atmosConfig *schema.AtmosConfiguration,
	flags *diffFlags,
) (*schema.AtmosVendorSource, string, error) {
	defer perf.Track(atmosConfig, "vendor.loadComponentSourceForDiff")()

	// Determine the vendor config file path.
	vendorConfigFileName := cfg.AtmosVendorConfigFileName
	if atmosConfig.Vendor.BasePath != "" {
		vendorConfigFileName = atmosConfig.Vendor.BasePath
	}

	// Read the main vendor config.
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(
		atmosConfig,
		vendorConfigFileName,
		true,
	)
	if err != nil {
		return nil, "", err
	}

	if !vendorConfigExists {
		return nil, "", nil
	}

	// Find the component in vendor sources.
	for i := range vendorConfig.Spec.Sources {
		if vendorConfig.Spec.Sources[i].Component == flags.Component {
			return &vendorConfig.Spec.Sources[i], foundVendorConfigFile, nil
		}
	}

	return nil, "", fmt.Errorf("%w: %s in %s", errUtils.ErrVendorComponentNotFound, flags.Component, foundVendorConfigFile)
}

// isGitSource checks if a source string represents a Git repository.
func isGitSource(source string) bool {
	return strings.HasPrefix(source, "git::") ||
		strings.HasPrefix(source, "github.com/") ||
		strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "git@")
}

// resolveRefs resolves the from and to refs for the diff operation.
func resolveRefs(
	flags *diffFlags,
	componentSource *schema.AtmosVendorSource,
	gitURI string,
	gitOps GitOperations,
) (fromRef, toRef string, err error) {
	defer perf.Track(nil, "vendor.resolveRefs")()

	// Determine from ref.
	fromRef = flags.From
	if fromRef == "" {
		fromRef = componentSource.Version
	}

	// Validate the fromRef.
	if fromRef == "" || strings.Contains(fromRef, "{{") {
		return "", "", fmt.Errorf("%w: invalid from ref %q for component %q", errUtils.ErrInvalidGitRef, fromRef, flags.Component)
	}

	// Determine to ref.
	toRef = flags.To
	if toRef == "" {
		toRef, err = resolveLatestVersion(gitURI, gitOps)
		if err != nil {
			return "", "", err
		}
	}

	return fromRef, toRef, nil
}

// resolveLatestVersion finds the latest version tag from remote.
func resolveLatestVersion(gitURI string, gitOps GitOperations) (string, error) {
	defer perf.Track(nil, "vendor.resolveLatestVersion")()

	tags, err := gitOps.GetRemoteTags(gitURI)
	if err != nil {
		return "", fmt.Errorf("failed to get remote tags: %w", err)
	}

	if len(tags) == 0 {
		return "", errUtils.ErrNoTagsFound
	}

	// Find latest semantic version.
	_, latestTag := version.FindLatestSemVerTag(tags)
	if latestTag == "" {
		return tags[0], nil
	}

	return latestTag, nil
}

// diffContext holds the context needed for diff generation.
type diffContext struct {
	atmosConfig *schema.AtmosConfiguration
	gitOps      GitOperations
	gitURI      string
	fromRef     string
	toRef       string
	flags       *diffFlags
}

// generateAndOutputDiff generates the diff and writes it to output.
func generateAndOutputDiff(ctx *diffContext) error {
	defer perf.Track(ctx.atmosConfig, "vendor.generateAndOutputDiff")()

	var diff []byte
	var err error
	if ctx.flags.File != "" {
		diff, err = ctx.gitOps.GetDiffBetweenRefsForFile(ctx.atmosConfig, ctx.gitURI, ctx.fromRef, ctx.toRef, ctx.flags.File, ctx.flags.Context, ctx.flags.NoColor)
	} else {
		diff, err = ctx.gitOps.GetDiffBetweenRefs(ctx.atmosConfig, ctx.gitURI, ctx.fromRef, ctx.toRef, ctx.flags.Context, ctx.flags.NoColor)
	}
	if err != nil {
		return err
	}

	if len(diff) == 0 {
		_ = ui.Infof("No differences between %s and %s", ctx.fromRef, ctx.toRef)
		return nil
	}

	return data.Write(string(diff))
}

// executeComponentVendorDiff handles vendor diff for component.yaml files.
func executeComponentVendorDiff(atmosConfig *schema.AtmosConfiguration, flags *diffFlags) error {
	defer perf.Track(atmosConfig, "vendor.executeComponentVendorDiff")()

	// TODO: Implement component vendor diff.
	// When implemented, this should:
	// 1. Read component.yaml from components/{type}/{component}/component.yaml.
	// 2. Extract version and source information.
	// 3. Call git diff operations similar to vendor.yaml handling.
	_ = ui.Warningf("Component vendor diff for component.yaml is not yet implemented for component %s", flags.Component)

	return errUtils.ErrNotImplemented
}
