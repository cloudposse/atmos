package exec

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
)

var (
	ErrNoVendorSourcesFound   = errors.New("no vendor.yaml found and no component.yaml manifests were discovered under any component type")
	ErrValidateEverythingFlag = errors.New("'--everything' flag cannot be combined with '--component', '--stack', '--tags', or '--labels' flags")
	// ErrValidateComponentStackFlag guards the stack-based vendoring path (handleStackVendor):
	// --stack resolves and vendors every component declared in the stack, so a single
	// --component target doesn't compose with it.
	ErrValidateComponentStackFlag = errors.New("either '--component' or '--stack' flag can be provided, but not both")
	// ErrValidateComponentLabelsFlag guards the same stack-resolution path as
	// ErrValidateComponentStackFlag: --labels resolves a set of stack-declared components (like
	// --stack), so a single --component target doesn't compose with it either.
	ErrValidateComponentLabelsFlag = errors.New("either '--component' or '--labels' flag can be provided, but not both")
	ErrMissingComponent            = errors.New("to vendor a component, the '--component' (shorthand '-c') flag needs to be specified.\n" +
		"Example: atmos vendor pull -c <component>")
	ErrInvalidLockEnforcement = errors.New("'--lock-enforcement' must be one of: strict, warn, silent")
	// ErrSingleComponentRequired guards the 'vendor update --pull' delegation path, where
	// --component is a repeatable slice but a pull can only target one component at a time.
	ErrSingleComponentRequired = errors.New("vendor pull accepts a single '--component' value")
)

// validLockEnforcementValues are the only values parseVendorFlags/validateVendorFlags accept for
// --lock-enforcement or vendor.lock.enforcement, matching schema.VendorLockConfig's own
// `validate:"omitempty,oneof=strict warn silent"` tag.
var validLockEnforcementValues = map[string]bool{
	install.LockEnforcementStrict: true,
	install.LockEnforcementWarn:   true,
	install.LockEnforcementSilent: true,
}

// DefaultLockEnforcement resolves vendor.lock.enforcement's effective value from atmosConfig,
// defaulting to install.LockEnforcementWarn when unset. Used directly by call paths with no
// --lock-enforcement flag of their own (e.g. cmd/vendor/update.go's pullBatchedComponentManifests),
// and by parseVendorFlags as the fallback when --lock-enforcement itself was not explicitly passed.
func DefaultLockEnforcement(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig != nil && atmosConfig.Vendor.Lock.Enforcement != "" {
		return atmosConfig.Vendor.Lock.Enforcement
	}
	return install.LockEnforcementWarn
}

// ExecuteVendorPullCmd executes `vendor pull` commands.
func ExecuteVendorPullCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteVendorPullCmd")()

	return ExecuteVendorPullCommand(cmd, args)
}

type VendorFlags struct {
	DryRun    bool
	Component string
	// Stack, when set, vendors every component declared in the stack that has its own
	// component.yaml -- see handleStackVendor. Bypasses vendor.yaml entirely, even when one
	// exists (checked ahead of the vendor.yaml lookup in handleVendorConfig).
	Stack string
	Tags  []string
	// Labels filters the stack-resolved component set (the same resolution --stack performs) by
	// metadata.labels, matching ALL key=value pairs (AND). Composes with --stack (narrows further)
	// and with Tags (a distinct, independent filter over vendor.yaml-declared source tags).
	Labels        map[string]string
	Everything    bool
	ComponentType string
	RefreshLock   bool
	// TypeChanged is true only when --type was explicitly passed, distinguishing "sweep only this
	// type" from "no --type given, sweep every component type" for handleVendorPullSweep.
	TypeChanged bool
	// LockEnforcement is one of install.LockEnforcementSilent/Warn/Strict, resolved from
	// --lock-enforcement (when explicitly passed) or else atmosConfig.Vendor.Lock.Enforcement
	// (when non-empty) or else install.LockEnforcementWarn -- see DefaultLockEnforcement.
	LockEnforcement string
}

// ExecuteVendorPullCommand executes `atmos vendor` commands.
func ExecuteVendorPullCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteVendorPullCommand")()

	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	// --component/--tags/--everything never need full stack processing (imports/inheritance/
	// deep-merge) - they operate on vendor.yaml/component.yaml manifests directly. --stack/
	// --labels are the exception: handleStackVendor resolves components via ExecuteDescribeStacksScoped,
	// which requires atmosConfig.StackConfigFilesAbsolutePaths to be populated, so those two flags
	// need processStacks=true. A cheap presence check (not the authoritative parse, which
	// parseVendorFlags still does below) is enough to decide.
	stackFlagVal, _ := flags.GetString("stack")
	labelsFlagVal, _ := flags.GetString("labels")
	needsStackProcessing := stackFlagVal != "" || labelsFlagVal != ""

	atmosConfig, err := cfg.InitCliConfig(info, needsStackProcessing)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI config: %w", err)
	}

	vendorFlags, err := parseVendorFlags(flags, &atmosConfig)
	if err != nil {
		return err
	}

	if err := validateVendorFlags(&vendorFlags); err != nil {
		return err
	}

	return handleVendorConfig(&atmosConfig, &vendorFlags, args)
}

func parseVendorFlags(flags *pflag.FlagSet, atmosConfig *schema.AtmosConfiguration) (VendorFlags, error) {
	vendorFlags := VendorFlags{}
	var err error

	if vendorFlags.DryRun, err = flags.GetBool("dry-run"); err != nil {
		return vendorFlags, err
	}
	if vendorFlags.Component, err = parseVendorComponentFlag(flags); err != nil {
		return vendorFlags, err
	}
	if vendorFlags.Stack, err = parseOptionalStringFlag(flags, "stack"); err != nil {
		return vendorFlags, err
	}
	if vendorFlags.Tags, err = parseVendorTagsFlag(flags); err != nil {
		return vendorFlags, err
	}
	if vendorFlags.Labels, err = parseOptionalLabelsFlag(flags); err != nil {
		return vendorFlags, err
	}
	if vendorFlags.Everything, err = flags.GetBool("everything"); err != nil {
		return vendorFlags, err
	}
	if vendorFlags.RefreshLock, err = parseOptionalBoolFlag(flags, "refresh-lock"); err != nil {
		return vendorFlags, err
	}
	if vendorFlags.LockEnforcement, err = resolveLockEnforcementFlag(flags, atmosConfig); err != nil {
		return vendorFlags, err
	}

	// Set default for 'everything' if no specific flags are provided.
	setDefaultEverythingFlag(flags, &vendorFlags)

	if err := parseVendorTypeFlag(flags, &vendorFlags); err != nil {
		return vendorFlags, err
	}

	return vendorFlags, nil
}

// parseVendorComponentFlag reads --component regardless of how the calling command registered it.
// 'vendor pull' registers it as a plain string, but 'vendor update --pull' delegates here with
// vendorUpdateCmd's FlagSet, where --component is a repeatable string slice; pflag's GetString
// hard-errors on that type mismatch. Pull operates on a single component, so a multi-element
// slice is rejected explicitly rather than silently pulling only the first entry.
func parseVendorComponentFlag(flags *pflag.FlagSet) (string, error) {
	flag := flags.Lookup("component")
	if flag == nil {
		return "", nil
	}
	if flag.Value.Type() != "stringSlice" {
		return flags.GetString("component")
	}
	components, err := flags.GetStringSlice("component")
	if err != nil {
		return "", err
	}
	switch len(components) {
	case 0:
		return "", nil
	case 1:
		return components[0], nil
	default:
		return "", fmt.Errorf("%w: got %d components", ErrSingleComponentRequired, len(components))
	}
}

// parseVendorTagsFlag splits --tags' comma-separated value, trimming whitespace around each tag
// and dropping empty segments (e.g. "networking, database" or "prod,,staging"), returning nil for
// an empty/unset flag. Mirrors cmd/vendor's splitTags (a different package, so duplicated rather
// than shared for a five-line helper).
func parseVendorTagsFlag(flags *pflag.FlagSet) ([]string, error) {
	tagsCsv, err := flags.GetString("tags")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(tagsCsv) == "" {
		return nil, nil
	}
	var tags []string
	for _, tag := range strings.Split(tagsCsv, ",") {
		if trimmed := strings.TrimSpace(tag); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags, nil
}

// parseOptionalLabelsFlag reads --labels (a comma-separated key=value/key:value list, parsed via
// pkg/tags.ParseLabelsFlag -- the same parser terraform bulk selection and list commands use) for
// callers that don't all register the flag (e.g. 'vendor update --pull' delegates to
// parseVendorFlags with its own FlagSet), returning nil without error when the flag is absent.
func parseOptionalLabelsFlag(flags *pflag.FlagSet) (map[string]string, error) {
	if flags.Lookup("labels") == nil {
		return nil, nil
	}
	labelsCsv, err := flags.GetString("labels")
	if err != nil {
		return nil, err
	}
	return tags.ParseLabelsFlag(labelsCsv)
}

// parseOptionalBoolFlag reads a bool flag that isn't registered on every cmd.Flags() this is
// called with (e.g. some callers share a cmd that doesn't define "refresh-lock"), returning false
// without error when the flag itself is absent.
func parseOptionalBoolFlag(flags *pflag.FlagSet, name string) (bool, error) {
	if flags.Lookup(name) == nil {
		return false, nil
	}
	return flags.GetBool(name)
}

// parseOptionalStringFlag reads a string flag that isn't registered on every cmd.Flags() this is
// called with (e.g. 'vendor update --pull' delegates to parseVendorFlags with its own FlagSet,
// which doesn't register "stack"), returning "" without error when the flag itself is absent.
func parseOptionalStringFlag(flags *pflag.FlagSet, name string) (string, error) {
	if flags.Lookup(name) == nil {
		return "", nil
	}
	return flags.GetString(name)
}

// resolveLockEnforcementFlag resolves --lock-enforcement's effective value: the flag's own value
// when explicitly passed, else vendor.lock.enforcement's configured default (DefaultLockEnforcement).
// A nil atmosConfig (some callers construct VendorFlags without a loaded config in tests) falls
// back to install.LockEnforcementWarn via DefaultLockEnforcement.
func resolveLockEnforcementFlag(flags *pflag.FlagSet, atmosConfig *schema.AtmosConfiguration) (string, error) {
	if flags.Lookup("lock-enforcement") != nil && flags.Changed("lock-enforcement") {
		return flags.GetString("lock-enforcement")
	}
	return DefaultLockEnforcement(atmosConfig), nil
}

// parseVendorTypeFlag reads --type only when it's registered on flags (not every caller registers it).
func parseVendorTypeFlag(flags *pflag.FlagSet, vendorFlags *VendorFlags) error {
	if flags.Lookup("type") == nil {
		return nil
	}
	var err error
	if vendorFlags.ComponentType, err = flags.GetString("type"); err != nil {
		return err
	}
	vendorFlags.TypeChanged = flags.Changed("type")
	return nil
}

// Helper function to set the default for 'everything' if no specific flags are provided.
func setDefaultEverythingFlag(flags *pflag.FlagSet, vendorFlags *VendorFlags) {
	if !vendorFlags.Everything && !flags.Changed("everything") &&
		vendorFlags.Component == "" && vendorFlags.Stack == "" && len(vendorFlags.Tags) == 0 && len(vendorFlags.Labels) == 0 {
		vendorFlags.Everything = true
	}
}

// validateVendorFlags rejects --component combined with --stack/--labels (both resolve a
// stack-declared component set that a single explicit name doesn't compose with) and --everything
// combined with any selector (it means "skip selection entirely"). --tags is deliberately not
// checked against anything here: it's an independent filter, not a fourth mutually exclusive
// selector "mode", so it composes with --component (handleComponentVendor/ExecuteAtmosVendorInternal
// already AND it via shouldSkipSource), and with --stack/--labels (handleStackVendor narrows its
// resolved set via vendoring.FilterComponentsByDeclaredTags).
func validateVendorFlags(flg *VendorFlags) error {
	if flg.Component != "" && flg.Stack != "" {
		return ErrValidateComponentStackFlag
	}

	if flg.Component != "" && len(flg.Labels) > 0 {
		return ErrValidateComponentLabelsFlag
	}

	if flg.Everything && (flg.Component != "" || flg.Stack != "" || len(flg.Tags) > 0 || len(flg.Labels) > 0) {
		return ErrValidateEverythingFlag
	}

	if flg.LockEnforcement != "" && !validLockEnforcementValues[flg.LockEnforcement] {
		return fmt.Errorf("%w: got %q", ErrInvalidLockEnforcement, flg.LockEnforcement)
	}

	return nil
}

func handleVendorConfig(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags, args []string) error {
	// --stack (and/or --labels, which resolves the same stack-declared component set, scoped
	// across all stacks when --stack is omitted) takes precedence over vendor.yaml -- it vendors
	// the resolved components (each via its own component.yaml) regardless of whether a repo-wide
	// vendor.yaml also exists.
	if flg.Stack != "" || len(flg.Labels) > 0 {
		return handleStackVendor(atmosConfig, flg)
	}

	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(
		atmosConfig,
		cfg.AtmosVendorConfigFileName,
		true,
	)
	if err != nil {
		return err
	}
	if !vendorConfigExists && flg.Everything {
		return handleVendorPullSweep(atmosConfig, flg)
	}
	if vendorConfigExists {
		return ExecuteAtmosVendorInternal(&executeVendorOptions{
			vendorConfigFileName: foundVendorConfigFile,
			dryRun:               flg.DryRun,
			refreshLock:          flg.RefreshLock,
			lockEnforcement:      flg.LockEnforcement,
			atmosConfig:          atmosConfig,
			atmosVendorSpec:      vendorConfig.Spec,
			component:            flg.Component,
			tags:                 flg.Tags,
		})
	}

	if flg.Component != "" {
		// No vendor.yaml exists, so flg.Component resolves via its own component.yaml, which has
		// no tags concept at all -- a non-empty --tags filter can never match it. Consistent with
		// --tags composing as an independent filter everywhere else in this feature: a candidate
		// with no declared tags is excluded by any non-empty tags filter, not a special case.
		if len(flg.Tags) > 0 {
			return errUtils.Build(errUtils.ErrInvalidArgumentError).
				WithExplanation("No components matched the given selector.").
				WithHint("--component resolved via its own component.yaml, which has no tags to match against --tags.").
				Err()
		}
		return handleComponentVendor(atmosConfig, flg)
	}

	if len(args) > 0 {
		q := fmt.Sprintf("Did you mean 'atmos vendor pull -c %s'?", args[0])
		return fmt.Errorf("%w\n%s", ErrMissingComponent, q)
	}
	return ErrMissingComponent
}

func handleComponentVendor(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags) error {
	componentType := flg.ComponentType
	if componentType == "" {
		componentType = "terraform"
	}

	config, path, err := ReadAndProcessComponentVendorConfigFile(
		atmosConfig,
		flg.Component,
		componentType,
	)
	if err != nil {
		return err
	}

	return ExecuteComponentVendorInternal(
		atmosConfig,
		&config.Spec,
		flg.Component,
		path,
		install.InstallOptions{DryRun: flg.DryRun, RefreshLock: flg.RefreshLock, LockEnforcement: flg.LockEnforcement},
	)
}
