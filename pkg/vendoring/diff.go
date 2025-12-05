package vendoring

import (
	"fmt"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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
	return executeVendorDiffWithGitOps(atmosConfig, flags, NewGitOperations())
}

// executeVendorDiffWithGitOps performs the vendor diff logic with injectable Git operations.
// This function allows for testing with mocked Git operations.
//
//nolint:revive,nestif,cyclop,funlen // Complex vendor diff logic with conditional ref resolution.
func executeVendorDiffWithGitOps(atmosConfig *schema.AtmosConfiguration, flags *diffFlags, gitOps GitOperations) error {
	defer perf.Track(atmosConfig, "vendor.executeVendorDiffWithGitOps")()

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
		return err
	}

	if !vendorConfigExists {
		// Try component vendor config if no main vendor config.
		return executeComponentVendorDiff(atmosConfig, flags)
	}

	// Find the component in vendor sources.
	var componentSource *schema.AtmosVendorSource
	for i := range vendorConfig.Spec.Sources {
		if vendorConfig.Spec.Sources[i].Component == flags.Component {
			componentSource = &vendorConfig.Spec.Sources[i]
			break
		}
	}

	if componentSource == nil {
		return fmt.Errorf("%w: %s in %s", errUtils.ErrVendorComponentNotFound, flags.Component, foundVendorConfigFile)
	}

	// Verify it's a Git source.
	if !strings.HasPrefix(componentSource.Source, "git::") &&
		!strings.HasPrefix(componentSource.Source, "github.com/") &&
		!strings.HasPrefix(componentSource.Source, "https://") &&
		!strings.HasPrefix(componentSource.Source, "git@") {
		return fmt.Errorf("%w: only Git sources are supported for diff", errUtils.ErrUnsupportedVendorSource)
	}

	// Extract Git URI from source.
	gitURI := version.ExtractGitURI(componentSource.Source)

	// Determine from/to refs.
	fromRef := flags.From
	if fromRef == "" {
		// Default to current version.
		fromRef = componentSource.Version
	}

	toRef := flags.To
	if toRef == "" {
		// Default to latest version using injected Git operations.
		tags, err := gitOps.GetRemoteTags(gitURI)
		if err != nil {
			return fmt.Errorf("failed to get remote tags: %w", err)
		}

		if len(tags) == 0 {
			return errUtils.ErrNoTagsFound
		}

		// Find latest semantic version.
		_, latestTag := version.FindLatestSemVerTag(tags)
		if latestTag == "" {
			// No semantic versions found, use first tag.
			toRef = tags[0]
		} else {
			toRef = latestTag
		}
	}

	// Generate the diff using injected Git operations.
	diff, err := gitOps.GetDiffBetweenRefs(atmosConfig, gitURI, fromRef, toRef, flags.Context, flags.NoColor)
	if err != nil {
		return err
	}

	// Output the diff.
	if len(diff) == 0 {
		fmt.Fprintf(os.Stderr, "No differences between %s and %s\n", fromRef, toRef)
		return nil
	}

	_, err = os.Stdout.Write(diff)
	return err
}

// executeComponentVendorDiff handles vendor diff for component.yaml files.
func executeComponentVendorDiff(atmosConfig *schema.AtmosConfiguration, flags *diffFlags) error {
	defer perf.Track(atmosConfig, "vendor.executeComponentVendorDiff")()

	// TODO: Implement component vendor diff.
	// When implemented, this should:
	// 1. Read component.yaml from components/{type}/{component}/component.yaml.
	// 2. Extract version and source information.
	// 3. Call git diff operations similar to vendor.yaml handling.
	fmt.Fprintf(os.Stderr, "Component vendor diff for component.yaml is not yet implemented for component %s\n", flags.Component)

	return errUtils.ErrNotImplemented
}
