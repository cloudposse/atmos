package exec

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteVendorDiffCmd executes `vendor diff` commands.
func ExecuteVendorDiffCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteVendorDiffCmd")()

	// Initialize Atmos configuration
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	// Vendor diff doesn't use stack flag
	processStacks := false

	atmosConfig, err := cfg.InitCliConfig(info, processStacks)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI config: %w", err)
	}

	// Parse vendor diff flags
	diffFlags, err := parseVendorDiffFlags(cmd)
	if err != nil {
		return err
	}

	// Validate component flag is provided
	if diffFlags.Component == "" {
		return errUtils.ErrComponentFlagRequired
	}

	// Execute vendor diff
	return executeVendorDiff(&atmosConfig, diffFlags)
}

// VendorDiffFlags holds flags specific to vendor diff command.
type VendorDiffFlags struct {
	Component string
	From      string
	To        string
	File      string
	Context   int
	Unified   bool
	NoColor   bool
}

// parseVendorDiffFlags parses flags from the vendor diff command.
func parseVendorDiffFlags(cmd *cobra.Command) (*VendorDiffFlags, error) {
	flags := cmd.Flags()

	component, err := flags.GetString("component")
	if err != nil {
		return nil, err
	}

	from, err := flags.GetString("from")
	if err != nil {
		return nil, err
	}

	to, err := flags.GetString("to")
	if err != nil {
		return nil, err
	}

	file, err := flags.GetString("file")
	if err != nil {
		return nil, err
	}

	context, err := flags.GetInt("context")
	if err != nil {
		return nil, err
	}

	unified, err := flags.GetBool("unified")
	if err != nil {
		return nil, err
	}

	// Check for no-color flag (may not exist yet in root command)
	noColor := false
	if flags.Lookup("no-color") != nil {
		noColor, err = flags.GetBool("no-color")
		if err != nil {
			return nil, err
		}
	}

	return &VendorDiffFlags{
		Component: component,
		From:      from,
		To:        to,
		File:      file,
		Context:   context,
		Unified:   unified,
		NoColor:   noColor,
	}, nil
}

// executeVendorDiff performs the vendor diff logic.
func executeVendorDiff(atmosConfig *schema.AtmosConfiguration, flags *VendorDiffFlags) error {
	return executeVendorDiffWithGitOps(atmosConfig, flags, NewGitOperations())
}

// executeVendorDiffWithGitOps performs the vendor diff logic with injectable Git operations.
// This function allows for testing with mocked Git operations.
//
//nolint:revive,nestif,cyclop,funlen // Complex vendor diff logic with conditional ref resolution.
func executeVendorDiffWithGitOps(atmosConfig *schema.AtmosConfiguration, flags *VendorDiffFlags, gitOps GitOperations) error {
	defer perf.Track(atmosConfig, "exec.executeVendorDiffWithGitOps")()

	// Determine the vendor config file path
	vendorConfigFileName := cfg.AtmosVendorConfigFileName
	if atmosConfig.Vendor.BasePath != "" {
		vendorConfigFileName = atmosConfig.Vendor.BasePath
	}

	// Read the main vendor config
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(
		atmosConfig,
		vendorConfigFileName,
		true,
	)
	if err != nil {
		return err
	}

	if !vendorConfigExists {
		// Try component vendor config if no main vendor config
		return executeComponentVendorDiff(atmosConfig, flags)
	}

	// Find the component in vendor sources
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

	// Verify it's a Git source
	if !strings.HasPrefix(componentSource.Source, "git::") &&
		!strings.HasPrefix(componentSource.Source, "github.com/") &&
		!strings.HasPrefix(componentSource.Source, "https://") &&
		!strings.HasPrefix(componentSource.Source, "git@") {
		return fmt.Errorf("%w: only Git sources are supported for diff", errUtils.ErrUnsupportedVendorSource)
	}

	// Extract Git URI from source
	gitURI := extractGitURI(componentSource.Source)

	// Determine from/to refs
	fromRef := flags.From
	if fromRef == "" {
		// Default to current version
		fromRef = componentSource.Version
	}

	toRef := flags.To
	if toRef == "" {
		// Default to latest version using injected Git operations
		tags, err := gitOps.GetRemoteTags(gitURI)
		if err != nil {
			return fmt.Errorf("failed to get remote tags: %w", err)
		}

		if len(tags) == 0 {
			return errUtils.ErrNoTagsFound
		}

		// Find latest semantic version
		_, latestTag := findLatestSemVerTag(tags)
		if latestTag == "" {
			// No semantic versions found, use first tag
			toRef = tags[0]
		} else {
			toRef = latestTag
		}
	}

	// Generate the diff using injected Git operations
	diff, err := gitOps.GetDiffBetweenRefs(atmosConfig, gitURI, fromRef, toRef, flags.Context, flags.NoColor)
	if err != nil {
		return err
	}

	// Output the diff
	if len(diff) == 0 {
		fmt.Fprintf(os.Stderr, "No differences between %s and %s\n", fromRef, toRef)
		return nil
	}

	_, err = os.Stdout.Write(diff)
	return err
}

// executeComponentVendorDiff handles vendor diff for component.yaml files.
func executeComponentVendorDiff(atmosConfig *schema.AtmosConfiguration, flags *VendorDiffFlags) error {
	defer perf.Track(atmosConfig, "exec.executeComponentVendorDiff")()

	// TODO: Implement component vendor diff
	// When implemented, this should:
	// 1. Read component.yaml from components/{type}/{component}/component.yaml
	// 2. Extract version and source information
	// 3. Call git diff operations similar to vendor.yaml handling
	fmt.Fprintf(os.Stderr, "Component vendor diff for component.yaml is not yet implemented for component %s\n", flags.Component)

	return errUtils.ErrNotImplemented
}

// extractGitURI extracts a clean Git URI from various vendor source formats.
func extractGitURI(source string) string {
	// Handle git:: prefix
	source = strings.TrimPrefix(source, "git::")

	// Handle github.com/ shorthand
	if strings.HasPrefix(source, "github.com/") {
		source = "https://" + source
	}

	// Remove query parameters and fragments (like ?ref=xxx)
	if idx := strings.Index(source, "?"); idx != -1 {
		source = source[:idx]
	}

	// Clean up .git suffix if present
	source = strings.TrimSuffix(source, ".git")

	return source
}
