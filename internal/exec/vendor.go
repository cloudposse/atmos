package exec

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
)

var (
	ErrNoVendorSourcesFound   = errors.New("no vendor.yaml found and no component.yaml manifests were discovered under any component type")
	ErrValidateComponentFlag  = errors.New("either '--component' or '--tags' flag can be provided, but not both")
	ErrValidateEverythingFlag = errors.New("'--everything' flag cannot be combined with '--component' or '--tags' flags")
	ErrMissingComponent       = errors.New("to vendor a component, the '--component' (shorthand '-c') flag needs to be specified.\n" +
		"Example: atmos vendor pull -c <component>")
	ErrInvalidLockEnforcement = errors.New("'--lock-enforcement' must be one of: strict, warn, silent")
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
	DryRun        bool
	Component     string
	Tags          []string
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

	// Vendor pull never needs full stack processing (imports/inheritance/deep-merge) - it
	// operates on vendor.yaml/component.yaml manifests directly.
	atmosConfig, err := cfg.InitCliConfig(info, false)
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
	if vendorFlags.Component, err = flags.GetString("component"); err != nil {
		return vendorFlags, err
	}
	if vendorFlags.Tags, err = parseVendorTagsFlag(flags); err != nil {
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

// parseVendorTagsFlag splits --tags' comma-separated value, returning nil for an empty/unset flag.
func parseVendorTagsFlag(flags *pflag.FlagSet) ([]string, error) {
	tagsCsv, err := flags.GetString("tags")
	if err != nil {
		return nil, err
	}
	if tagsCsv == "" {
		return nil, nil
	}
	return strings.Split(tagsCsv, ","), nil
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
		vendorFlags.Component == "" && len(vendorFlags.Tags) == 0 {
		vendorFlags.Everything = true
	}
}

func validateVendorFlags(flg *VendorFlags) error {
	if flg.Component != "" && len(flg.Tags) > 0 {
		return ErrValidateComponentFlag
	}

	if flg.Everything && (flg.Component != "" || len(flg.Tags) > 0) {
		return ErrValidateEverythingFlag
	}

	if flg.LockEnforcement != "" && !validLockEnforcementValues[flg.LockEnforcement] {
		return fmt.Errorf("%w: got %q", ErrInvalidLockEnforcement, flg.LockEnforcement)
	}

	return nil
}

func handleVendorConfig(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags, args []string) error {
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
