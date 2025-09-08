package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
}

// extractStreamConfig extracts all configuration from the command.
func extractStreamConfig(cmd *cobra.Command, args []string, logger *log.Logger) (*StreamConfig, error) {
	config := &StreamConfig{}

	// Parse test path
	config.TestPath = "./..."
	if len(args) > 0 {
		config.TestPath = args[0]
		logger.Debug("Test path specified", "path", config.TestPath)
	}

	// Extract test arguments
	config.TestArgs = extractTestArguments(cmd)

	// Get filter flags
	config.ShowFilter, _ = cmd.Flags().GetString("show")
	config.IncludePatterns, _ = cmd.Flags().GetString("include")
	config.ExcludePatterns, _ = cmd.Flags().GetString("exclude")

	// Get output settings
	config.Format, _ = cmd.Flags().GetString("format")
	config.OutputFile, _ = cmd.Flags().GetString("output")
	config.Alert, _ = cmd.Flags().GetBool("alert")
	config.VerbosityLevel, _ = cmd.Flags().GetString("verbosity")

	// Get coverage settings
	config.Cover, _ = cmd.Flags().GetBool("cover")
	config.CoverProfile, _ = cmd.Flags().GetString("coverprofile")
	config.CoverPkg, _ = cmd.Flags().GetString("coverpkg")

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
	_ = viper.BindPFlag("post-comment", cmd.Flags().Lookup("post-comment"))
	_ = viper.BindEnv("post-comment", "GOTCHA_POST_COMMENT", "POST_COMMENT")
	config.PostStrategy = viper.GetString("post-comment")
	config.PostFlagPresent = cmd.Flags().Changed("post-comment") || viper.IsSet("post-comment")

	// Bind other viper settings
	_ = viper.BindPFlag("github-token", cmd.Flags().Lookup("github-token"))
	_ = viper.BindEnv("github-token", "GITHUB_TOKEN")

	_ = viper.BindPFlag("exclude-mocks", cmd.Flags().Lookup("exclude-mocks"))
	config.ExcludeMocks = viper.GetBool("exclude-mocks")

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
		config.OutputFile = "test-output.json"
		if config.Format == "markdown" {
			config.OutputFile = "test-output.md"
		}
	}

	return config, nil
}

// detectCIMode auto-detects CI environment.
func (c *StreamConfig) detectCIMode(logger *log.Logger) {
	if !c.CIMode {
		// Using viper for environment detection
		if viper.GetBool("ci") || viper.GetBool("github.actions") {
			c.CIMode = true
			logger.Debug("CI mode auto-detected",
				"CI", viper.GetBool("ci"),
				"GITHUB_ACTIONS", viper.GetBool("github.actions"))
		}
	}
}

// adjustFormatForCI adjusts output format for CI environments.
func (c *StreamConfig) adjustFormatForCI(cmd *cobra.Command, logger *log.Logger) {
	if c.CIMode && c.Format == "terminal" {
		// Don't override if user explicitly set format
		if !cmd.Flags().Changed("format") {
			c.Format = "markdown"
			logger.Debug("Switching to markdown format for CI mode")
		}
	}
}

// adjustShowFilterForVerbosity adjusts show filter based on verbosity level.
func (c *StreamConfig) adjustShowFilterForVerbosity(cmd *cobra.Command) {
	if cmd.Flags().Changed("verbosity") {
		switch c.VerbosityLevel {
		case "minimal":
			c.ShowFilter = "failed"
		case "verbose":
			c.ShowFilter = "all"
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
		if arg == "--" {
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
		testArgs = append(testArgs, "-run", run)
	}
	if timeout, _ := cmd.Flags().GetString("timeout"); timeout != "" && timeout != "10m" {
		testArgs = append(testArgs, "-timeout", timeout)
	}
	if short, _ := cmd.Flags().GetBool("short"); short {
		testArgs = append(testArgs, "-short")
	}
	if race, _ := cmd.Flags().GetBool("race"); race {
		testArgs = append(testArgs, "-race")
	}
	if count, _ := cmd.Flags().GetInt("count"); count > 1 {
		testArgs = append(testArgs, "-count", fmt.Sprintf("%d", count))
	}
	if shuffle, _ := cmd.Flags().GetBool("shuffle"); shuffle {
		testArgs = append(testArgs, "-shuffle", "on")
	}

	return testArgs
}

// parseTestPackages determines the test packages from the test path.
func (c *StreamConfig) parseTestPackages() {
	var testPackages []string

	switch {
	case c.TestPath == "./..." || c.TestPath == "...":
		testPackages = append(testPackages, "./...")
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
