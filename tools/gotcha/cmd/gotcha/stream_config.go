package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/output"
)

// Configuration field names.
const (
	// Viper configuration keys.
	ViperKeyShow            = "show"
	ViperKeyFormat          = "format"
	ViperKeyOutput          = "output"
	ViperKeyAlert           = "alert"
	ViperKeyCover           = "cover"
	ViperKeyCoverageEnabled = "coverage.enabled"
	ViperKeyCoverProfile    = "coverprofile"
	ViperKeyCoverageProfile = "coverage.profile"
	ViperKeyCoverPkg        = "coverpkg"
	ViperKeyPostComment     = "post-comment"
	ViperKeyGitHubToken     = "github-token"
	ViperKeyExcludeMocks    = "exclude-mocks"

	// Environment variables.
	EnvGotchaPostComment = "GOTCHA_POST_COMMENT"
	EnvPostComment       = "POST_COMMENT"
	EnvGitHubToken       = "GITHUB_TOKEN"

	// Default values.
	DefaultShowFilter = "all"
	DefaultTestPath   = "./..."
	DefaultTimeout    = "10m"
	DefaultOutputMD   = "test-output.md"
	DefaultOutputJSON = "test-output.json"

	// Verbosity levels.
	VerbosityMinimal  = "minimal"
	VerbosityVerbose  = "verbose"
	VerbosityStandard = "standard"

	// Show filter values.
	ShowFilterFailed = "failed"
	ShowFilterAll    = "all"


	// Test argument flags.
	FlagRun       = "-run"
	FlagTimeout   = "-timeout"
	FlagShort     = "-short"
	FlagRace      = "-race"
	FlagTestCount = "-count"
	FlagShuffle   = "-shuffle"

	// Separator.
	DashSeparator = "--"

	// Shuffle option.
	ShuffleOn = "on"
)

// StreamConfig holds all configuration for the stream command.
type StreamConfig struct {
	// Test execution settings
	TestPath     string
	TestPackages []string
	TestArgs     []string

	// Filter settings
	ShowFilter      string
	IncludePatterns string
	ExcludePatterns string

	// Output settings
	Format         string
	OutputFile     string
	Alert          bool
	VerbosityLevel string

	// Coverage settings
	Cover        bool
	CoverProfile string
	CoverPkg     string

	// CI settings
	CIMode          bool
	PostStrategy    string
	PostFlagPresent bool

	// Cache settings
	EstimatedTestCount int

	// Derived settings
	ExcludeMocks bool

	// Output writer for unified output handling
	Writer *output.Writer
}

// extractStreamConfig extracts all configuration from the command.
//
// - Command-line flag processing
// - Environment variable fallbacks
// - Default value assignments
// - CI mode detection and adaptation
// - Format validation and normalization
// - Coverage configuration handling
// Each condition handles a specific configuration aspect.
//
//nolint:nestif,gocognit // Configuration extraction involves many conditional checks:
func extractStreamConfig(cmd *cobra.Command, args []string, logger *log.Logger) (*StreamConfig, error) {
	config := &StreamConfig{}

	// Parse test path
	config.TestPath = DefaultTestPath
	if len(args) > 0 {
		config.TestPath = args[0]
		logger.Debug("Test path specified", "path", config.TestPath)
	}

	// Extract test arguments
	config.TestArgs = extractTestArguments(cmd)

	// Get filter flags - use viper for show filter to respect config file
	// Only use flag value if it was explicitly set, otherwise use viper
	if cmd.Flags().Changed(ViperKeyShow) {
		config.ShowFilter, _ = cmd.Flags().GetString(ViperKeyShow)
	} else {
		config.ShowFilter = viper.GetString(ViperKeyShow)
		// Default to "all" if not set anywhere
		if config.ShowFilter == "" {
			config.ShowFilter = DefaultShowFilter
		}
	}

	// Log the show filter value for debugging config issues
	logger.Debug("ShowFilter configuration",
		"showFilter", config.ShowFilter,
		"viperValue", viper.GetString(ViperKeyShow),
		"flagChanged", cmd.Flags().Changed(ViperKeyShow),
		"configFile", viper.ConfigFileUsed())

	// Get other filter patterns from flags
	config.IncludePatterns, _ = cmd.Flags().GetString("include")
	config.ExcludePatterns, _ = cmd.Flags().GetString("exclude")

	// Get output settings - use flag value only if explicitly set, otherwise use viper
	if cmd.Flags().Changed(ViperKeyFormat) {
		config.Format, _ = cmd.Flags().GetString(ViperKeyFormat)
		logger.Debug("Format from flag", "format", config.Format)
	} else {
		config.Format = viper.GetString(ViperKeyFormat)
		logger.Debug("Format from viper", "format", config.Format, "configFile", viper.ConfigFileUsed())
		if config.Format == "" {
			config.Format = FormatTerminal
			logger.Debug("Format defaulted to terminal")
		}
	}

	if cmd.Flags().Changed(ViperKeyOutput) {
		config.OutputFile, _ = cmd.Flags().GetString(ViperKeyOutput)
	} else {
		config.OutputFile = viper.GetString(ViperKeyOutput)
	}

	if cmd.Flags().Changed(ViperKeyAlert) {
		config.Alert, _ = cmd.Flags().GetBool(ViperKeyAlert)
	} else {
		config.Alert = viper.GetBool(ViperKeyAlert)
	}

	config.VerbosityLevel, _ = cmd.Flags().GetString("verbosity")

	// Get coverage settings - use flag value only if explicitly set, otherwise use viper
	if cmd.Flags().Changed(ViperKeyCover) {
		config.Cover, _ = cmd.Flags().GetBool(ViperKeyCover)
	} else {
		// Check both locations for backward compatibility
		config.Cover = viper.GetBool(ViperKeyCover)
		if !config.Cover {
			// Try the new structured location
			config.Cover = viper.GetBool(ViperKeyCoverageEnabled)
		}
	}

	if cmd.Flags().Changed(ViperKeyCoverProfile) {
		config.CoverProfile, _ = cmd.Flags().GetString(ViperKeyCoverProfile)
	} else {
		// Check both locations for backward compatibility
		config.CoverProfile = viper.GetString(ViperKeyCoverProfile)
		if config.CoverProfile == "" {
			// Try the new structured location
			config.CoverProfile = viper.GetString(ViperKeyCoverageProfile)
		}
	}

	if cmd.Flags().Changed(ViperKeyCoverPkg) {
		config.CoverPkg, _ = cmd.Flags().GetString(ViperKeyCoverPkg)
	} else {
		config.CoverPkg = viper.GetString(ViperKeyCoverPkg)
	}

	// Handle coverage flags
	if config.Cover && config.CoverProfile == "" {
		config.CoverProfile = fmt.Sprintf("coverage-%s.out", time.Now().Format("20060102-150405"))
	}
	if config.CoverProfile != "" {
		config.Cover = true
	}

	// Get CI settings
	config.CIMode, _ = cmd.Flags().GetBool("ci")

	// Bind and get posting strategy
	_ = viper.BindPFlag(ViperKeyPostComment, cmd.Flags().Lookup(ViperKeyPostComment))
	_ = viper.BindEnv(ViperKeyPostComment, EnvGotchaPostComment, EnvPostComment)
	config.PostStrategy = viper.GetString(ViperKeyPostComment)
	config.PostFlagPresent = cmd.Flags().Changed(ViperKeyPostComment) || viper.IsSet(ViperKeyPostComment)

	// Bind other viper settings
	_ = viper.BindPFlag(ViperKeyGitHubToken, cmd.Flags().Lookup(ViperKeyGitHubToken))
	_ = viper.BindEnv(ViperKeyGitHubToken, EnvGitHubToken)

	_ = viper.BindPFlag(ViperKeyExcludeMocks, cmd.Flags().Lookup(ViperKeyExcludeMocks))
	config.ExcludeMocks = viper.GetBool(ViperKeyExcludeMocks)

	// Auto-detect CI mode
	config.detectCIMode(logger)

	// Adjust format for CI if needed
	config.adjustFormatForCI(cmd, logger)

	// Override show filter based on verbosity
	config.adjustShowFilterForVerbosity(cmd)

	// Normalize posting strategy
	config.PostStrategy = normalizePostingStrategy(config.PostStrategy, config.PostFlagPresent)

	// Determine output file if not specified
	if config.OutputFile == "" {
		switch config.Format {
		case FormatMarkdown, FormatGitHub:
			config.OutputFile = DefaultOutputMD
		default:
			config.OutputFile = DefaultOutputJSON
		}
	}

	return config, nil
}

// detectCIMode auto-detects CI environment.
func (c *StreamConfig) detectCIMode(logger *log.Logger) {
	if !c.CIMode {
		// Use config package for proper runtime detection
		inCI := config.IsCI()             // Checks actual runtime environment
		ciEnabled := config.IsCIEnabled() // Checks config setting

		inGitHubActions := config.IsGitHubActions()             // Checks actual runtime
		githubActionsEnabled := config.IsGitHubActionsEnabled() // Checks config

		// Only enable CI mode if we're actually in CI
		// For generic CI: just being in CI is enough
		// For GitHub Actions: we check runtime, config is for features
		if inCI || inGitHubActions {
			c.CIMode = true
			logger.Debug("CI mode detected",
				"inCI", inCI,
				"ciEnabled", ciEnabled,
				"inGitHubActions", inGitHubActions,
				"githubActionsEnabled", githubActionsEnabled)
		}
	}
}

// adjustFormatForCI adjusts output format for CI environments.
func (c *StreamConfig) adjustFormatForCI(cmd *cobra.Command, logger *log.Logger) {
	if c.CIMode && c.Format == FormatTerminal {
		// Don't override if user explicitly set format
		if !cmd.Flags().Changed(ViperKeyFormat) {
			c.Format = FormatMarkdown
			logger.Debug("Switching to markdown format for CI mode")
		}
	}
}

// adjustShowFilterForVerbosity adjusts show filter based on verbosity level.
func (c *StreamConfig) adjustShowFilterForVerbosity(cmd *cobra.Command) {
	if cmd.Flags().Changed("verbosity") {
		switch c.VerbosityLevel {
		case VerbosityMinimal:
			c.ShowFilter = ShowFilterFailed
		case VerbosityVerbose:
			c.ShowFilter = ShowFilterAll
			// "standard" and "with-output" keep existing filter
		}
	}
}

// extractTestArguments builds the test arguments from command flags.
func extractTestArguments(cmd *cobra.Command) []string {
	var testArgs []string

	// Check for -- separator to allow raw go test args
	dashIndex := -1
	for i, arg := range os.Args {
		if arg == DashSeparator {
			dashIndex = i
			break
		}
	}

	// If we have raw args after --, use them
	if dashIndex >= 0 && dashIndex < len(os.Args)-1 {
		testArgs = os.Args[dashIndex+1:]
		return testArgs
	}

	// Build args from flags
	if run, _ := cmd.Flags().GetString("run"); run != "" {
		testArgs = append(testArgs, FlagRun, run)
	}
	if timeout, _ := cmd.Flags().GetString("timeout"); timeout != "" && timeout != DefaultTimeout {
		testArgs = append(testArgs, FlagTimeout, timeout)
	}
	if short, _ := cmd.Flags().GetBool("short"); short {
		testArgs = append(testArgs, FlagShort)
	}
	if race, _ := cmd.Flags().GetBool("race"); race {
		testArgs = append(testArgs, FlagRace)
	}
	if count, _ := cmd.Flags().GetInt("count"); count > 1 {
		testArgs = append(testArgs, FlagTestCount, fmt.Sprintf("%d", count))
	}
	if shuffle, _ := cmd.Flags().GetBool("shuffle"); shuffle {
		testArgs = append(testArgs, FlagShuffle, ShuffleOn)
	}

	return testArgs
}

// parseTestPackages determines the test packages from the test path.
func (c *StreamConfig) parseTestPackages() {
	var testPackages []string

	switch {
	case c.TestPath == DefaultTestPath || c.TestPath == "...":
		testPackages = append(testPackages, DefaultTestPath)
	case strings.HasSuffix(c.TestPath, "/..."):
		testPackages = append(testPackages, c.TestPath)
	case strings.Contains(c.TestPath, ","):
		for _, pkg := range strings.Split(c.TestPath, ",") {
			testPackages = append(testPackages, strings.TrimSpace(pkg))
		}
	default:
		testPackages = append(testPackages, c.TestPath)
	}

	c.TestPackages = testPackages
}
