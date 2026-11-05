// Package startup holds the configuration-free command startup shared by the root command and by commands
// that deliberately skip the root pre-run (such as doctor) because they must run on a broken atmos.yaml.
//
// The sequence is split in two phases because the root command loads atmos.yaml between them: flag values
// must reach Viper before the configuration is loaded, and the I/O context must be built after it.
package startup

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	profileFlagName  = "profile"
	identityFlagName = "identity"
	// The edition flag mirrors the atmos.yaml key of the same name.
	editionFlagName = "edition"
	maskFlagName    = "mask"
)

// Configure runs both startup phases back to back for commands that never load configuration.
// The order is: refine the color profile from parsed flags, synchronize global flags to Viper,
// initialize the I/O context, reconcile masking, rebuild the UI and data writers, then configure the
// logger from --logs-level, --logs-file, and ATMOS_LOGS_*. An unsupported log level or unusable log
// file is returned as an error with the usage exit code.
func Configure(cmd *cobra.Command) error {
	defer perf.Track(nil, "startup.Configure")()

	PrepareFlags(cmd)
	if err := InitializeIO(cmd, nil); err != nil {
		return err
	}
	handle, err := ConfigureLogger(cmd)
	if handle != nil {
		setLogFile(handle)
	}
	return err
}

var (
	logFileMu sync.Mutex
	logFile   *os.File
)

// setLogFile remembers the log file opened by Configure, closing one left over from an earlier call.
func setLogFile(handle *os.File) {
	logFileMu.Lock()
	defer logFileMu.Unlock()
	if logFile != nil {
		_ = logFile.Close()
	}
	logFile = handle
}

// CloseLogFile flushes and closes the log file opened by Configure. It is safe to call when none is open.
func CloseLogFile() {
	defer perf.Track(nil, "startup.CloseLogFile")()

	logFileMu.Lock()
	defer logFileMu.Unlock()
	if logFile == nil {
		return
	}
	_ = logFile.Sync()
	_ = logFile.Close()
	logFile = nil
}

// PrepareFlags applies parsed global flags before anything reads them: the early color profile
// (--no-color, --force-color, NO_COLOR) and the Viper synchronization of flags that configuration
// loading depends on. It does not read atmos.yaml.
func PrepareFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "startup.PrepareFlags")()

	// Configure lipgloss color profile early, before config loading.
	// This is critical because stack processing during config load may trigger
	// validation that uses theme styles. We need to set the color profile BEFORE
	// those styles are accessed to ensure NO_COLOR and other settings are respected.
	ConfigureEarlyColorProfile(cmd)

	// Sync global flags from Cobra to Viper before InitCliConfig.
	// This ensures flag values are immediately available in Viper for config loading.
	SyncGlobalFlagsToViper(cmd)
}

// InitializeIO rebuilds the I/O context and the global UI and data writers after flag parsing.
// This ensures flags like --no-color, --redirect-stderr, --mask are respected.
// Initialize() sets the global iolib.Data and iolib.UI writers with masking enabled.
// The loaded configuration, when there is one, supplies the masking literals and patterns.
func InitializeIO(cmd *cobra.Command, config *schema.AtmosConfiguration) error {
	defer perf.Track(nil, "startup.InitializeIO")()

	if err := iolib.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize I/O context: %w", err)
	}
	// The global masker may have been created before CLI flags were parsed (e.g. by early
	// output), so its enabled state reflects the `--mask` default, not the parsed flag.
	// Reconcile it now that flags are available so `--mask=false` reliably disables masking.
	ReconcileMasking(cmd, config)
	ioCtx := iolib.GetContext()
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)
	data.SetMarkdownRenderer(ui.Format) // Connect markdown rendering to data channel.
	return nil
}

// InitializeIOOrExit is InitializeIO for the root pre-run, where a failure to build the I/O context is fatal.
func InitializeIOOrExit(cmd *cobra.Command, config *schema.AtmosConfiguration) {
	defer perf.Track(nil, "startup.InitializeIOOrExit")()

	if err := InitializeIO(cmd, config); err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
}

// SyncGlobalFlagsToViper synchronizes global flags from Cobra's FlagSet to Viper.
// This is necessary because Viper's BindPFlag doesn't immediately sync values when flags are parsed.
// Call this after Cobra parses flags but before accessing flag values via Viper.
//
// Background: When using viper.BindPFlag(), the binding happens at initialization time,
// but the actual flag value isn't synced to Viper until you call viper.Get*().
// For some code paths (especially in InitCliConfig), we need the flag values
// available in Viper before config loading completes.
//
// This function explicitly syncs changed flags to Viper, making their values
// immediately available via viper.Get*() calls.
func SyncGlobalFlagsToViper(cmd *cobra.Command) {
	defer perf.Track(nil, "startup.SyncGlobalFlagsToViper")()

	v := viper.GetViper()

	// Sync profile flag if explicitly set.
	if cmd.Flags().Changed(profileFlagName) {
		if profiles, err := cmd.Flags().GetStringSlice(profileFlagName); err == nil {
			v.Set(profileFlagName, profiles)
		}
	}

	// Sync identity flag if explicitly set.
	if cmd.Flags().Changed(identityFlagName) {
		if identity, err := cmd.Flags().GetString(identityFlagName); err == nil {
			v.Set(identityFlagName, identity)
		}
	}

	// Sync edition flag if explicitly set, so LoadConfig sees the pin
	// before it applies edition default overrides.
	if cmd.Flags().Changed(editionFlagName) {
		if editionPin, err := cmd.Flags().GetString(editionFlagName); err == nil {
			v.Set(editionFlagName, editionPin)
		}
	}
}

// ConfigureEarlyColorProfile sets the lipgloss color profile based on environment variables and parsed flags.
// This is called early in the root pre-run, before config loading, to ensure that any
// theme styles accessed during stack processing respect NO_COLOR and other settings.
func ConfigureEarlyColorProfile(cmd *cobra.Command) {
	defer perf.Track(nil, "startup.ConfigureEarlyColorProfile")()

	// Check NO_COLOR environment variable (standard terminal env var).
	//nolint:forbidigo // Standard terminal env var, must use os.Getenv before config loads.
	if os.Getenv("NO_COLOR") != "" {
		// NO_COLOR is set - disable all colors.
		ui.SetColorProfile(termenv.Ascii)
		theme.InvalidateStyleCache() // Regenerate theme styles without colors.
		return
	}

	// Check --no-color flag (already parsed by cobra at this point).
	if noColor, _ := cmd.Flags().GetBool("no-color"); noColor {
		ui.SetColorProfile(termenv.Ascii)
		theme.InvalidateStyleCache()
		return
	}

	// Check --force-color flag.
	if forceColor, _ := cmd.Flags().GetBool("force-color"); forceColor {
		ui.SetColorProfile(termenv.TrueColor)
		theme.InvalidateStyleCache()
		return
	}

	// Note: Full color profile detection happens later in InitFormatter().
	// This early configuration just handles the most critical cases (NO_COLOR).
}

// ReconcileMasking applies the configured masking policy and then
// honors a command-local --mask flag when a subcommand shadows the root flag.
// Several component command groups register their own persistent common flags;
// Viper remains bound to the root flag, so the changed local value must win.
// A nil config skips registering configured literals and patterns.
func ReconcileMasking(cmd *cobra.Command, config *schema.AtmosConfiguration) {
	defer perf.Track(nil, "startup.ReconcileMasking")()

	iolib.ReconcileMasking()
	// Config discovery uses an invocation-local Viper instance. The global I/O
	// context may predate it, so register the resolved literals and patterns here
	// while retaining the already-reconciled flag/environment masking policy.
	if config != nil {
		iolib.ApplyMaskingConfig(&iolib.Config{AtmosConfig: *config, DisableMasking: !iolib.MaskingEnabled()})
	}
	if cmd == nil {
		return
	}
	for current := cmd; current != nil; current = current.Parent() {
		maskFlag := current.PersistentFlags().Lookup(maskFlagName)
		if maskFlag == nil || !maskFlag.Changed {
			continue
		}
		enabled, err := strconv.ParseBool(maskFlag.Value.String())
		if err != nil {
			return
		}
		iolib.GetContext().Masker().SetEnabled(enabled)
		return
	}
}
