package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"

	semverlib "github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	ErrVendorConfigNotExist       = errors.New("the '--everything' flag is set, but vendor config file does not exist")
	ErrValidateComponentFlag      = errors.New("either '--component' or '--tags' flag can be provided, but not both")
	ErrValidateComponentStackFlag = errors.New("either '--component' or '--stack' flag can be provided, but not both")
	ErrValidateEverythingFlag     = errors.New("'--everything' flag cannot be combined with '--component', '--stack', or '--tags' flags")
	ErrMissingComponent           = errors.New("to vendor a component, the '--component' (shorthand '-c') flag needs to be specified.\n" +
		"Example: atmos vendor pull -c <component>")

	commitHashRegex = regexp.MustCompile(`^[a-fA-F0-9]{7,40}$`)
)

const (
	gitLsRemoteCmd = "ls-remote"
)

// ExecuteVendorPullCmd executes `vendor pull` commands.
func ExecuteVendorPullCmd(cmd *cobra.Command, args []string) error {
	return ExecuteVendorPullCommand(cmd, args)
}

// ExecuteVendorPullCommand executes `atmos vendor` commands.
func ExecuteVendorPullCommand(cmd *cobra.Command, args []string) error {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	flags := cmd.Flags()
	processStacks := flags.Changed("stack")

	atmosConfig, err := cfg.InitCliConfig(info, processStacks)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI config: %w", err)
	}

	vendorFlags, err := parseVendorFlags(flags)
	if err != nil {
		return err
	}

	if err := validateVendorFlags(&vendorFlags); err != nil {
		return err
	}

	if vendorFlags.Stack != "" {
		return ExecuteStackVendorInternal(vendorFlags.Stack, vendorFlags.DryRun)
	}

	return handleVendorConfig(&atmosConfig, &vendorFlags, args)
}

// ExecuteVendorDiffCmd executes `vendor diff` commands.
func ExecuteVendorDiffCmd(cmd *cobra.Command, args []string) error {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	flags := cmd.Flags()
	processStacks := false // stack flag is not used in vendor diff

	atmosConfig, err := cfg.InitCliConfig(info, processStacks)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI config: %w", err)
	}

	vendorFlags, err := parseVendorFlags(flags)
	if err != nil {
		return err
	}

	// For vendor diff, always dry-run unless --update is set
	vendorFlags.DryRun = !vendorFlags.Update

	if err := validateVendorFlags(&vendorFlags); err != nil {
		return err
	}

	return executeVendorDiff(&atmosConfig, &vendorFlags)
}

type VendorFlags struct {
	DryRun        bool
	Component     string
	Stack         string
	Tags          []string
	Everything    bool
	ComponentType string
	Update        bool // New flag to update vendor file with latest versions
	Outdated      bool // New flag to show only outdated components
}

func parseVendorFlags(flags *pflag.FlagSet) (VendorFlags, error) {
	vendorFlags := VendorFlags{}
	var err error

	// Handle dry-run flag only if it exists (vendor pull has it, vendor diff doesn't)
	if flags.Lookup("dry-run") != nil {
		if vendorFlags.DryRun, err = flags.GetBool("dry-run"); err != nil {
			return vendorFlags, err
		}
	}

	if vendorFlags.Component, err = flags.GetString("component"); err != nil {
		return vendorFlags, err
	}

	if vendorFlags.Stack, err = flags.GetString("stack"); err != nil {
		return vendorFlags, err
	}

	tagsCsv, err := flags.GetString("tags")
	if err != nil {
		return vendorFlags, err
	}
	if tagsCsv != "" {
		vendorFlags.Tags = strings.Split(tagsCsv, ",")
	}

	if vendorFlags.Everything, err = flags.GetBool("everything"); err != nil {
		return vendorFlags, err
	}

	if flags.Lookup("update") != nil {
		if vendorFlags.Update, err = flags.GetBool("update"); err != nil {
			return vendorFlags, err
		}
	}

	if flags.Lookup("outdated") != nil {
		if vendorFlags.Outdated, err = flags.GetBool("outdated"); err != nil {
			return vendorFlags, err
		}
	}

	// Set default for 'everything' if no specific flags are provided
	setDefaultEverythingFlag(flags, &vendorFlags)

	// Handle 'type' flag only if it exists
	if flags.Lookup("type") != nil {
		if vendorFlags.ComponentType, err = flags.GetString("type"); err != nil {
			return vendorFlags, err
		}
	}

	return vendorFlags, nil
}

// Helper function to set the default for 'everything' if no specific flags are provided.
func setDefaultEverythingFlag(flags *pflag.FlagSet, vendorFlags *VendorFlags) {
	if !vendorFlags.Everything && !flags.Changed("everything") &&
		vendorFlags.Component == "" && vendorFlags.Stack == "" && len(vendorFlags.Tags) == 0 {
		vendorFlags.Everything = true
	}
}

func validateVendorFlags(flg *VendorFlags) error {
	if flg.Component != "" && flg.Stack != "" {
		return ErrValidateComponentStackFlag
	}

	if flg.Component != "" && len(flg.Tags) > 0 {
		return ErrValidateComponentFlag
	}

	if flg.Everything && (flg.Component != "" || flg.Stack != "" || len(flg.Tags) > 0) {
		return ErrValidateEverythingFlag
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
		return fmt.Errorf("%w: %s", ErrVendorConfigNotExist, cfg.AtmosVendorConfigFileName)
	}
	if vendorConfigExists {
		return ExecuteAtmosVendorInternal(&executeVendorOptions{
			vendorConfigFileName: foundVendorConfigFile,
			dryRun:               flg.DryRun,
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
		flg.DryRun,
	)
}

func executeVendorDiff(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags) error {
	// Determine the vendor config file path based on atmos configuration
	vendorConfigFileName := cfg.AtmosVendorConfigFileName // Default to "vendor.yaml"

	// If vendor.base_path is configured in atmos.yaml, use that instead
	if atmosConfig.Vendor.BasePath != "" {
		vendorConfigFileName = atmosConfig.Vendor.BasePath
	}

	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(
		atmosConfig,
		vendorConfigFileName,
		true,
	)
	if err != nil {
		return err
	}

	if !vendorConfigExists {
		return fmt.Errorf("%w: %s", errUtils.ErrVendorConfigFileNotFound, vendorConfigFileName)
	}

	// Process imports and get all sources
	sources, _, err := processVendorImports(
		atmosConfig,
		foundVendorConfigFile,
		vendorConfig.Spec.Imports,
		vendorConfig.Spec.Sources,
		[]string{foundVendorConfigFile},
	)
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		fmt.Println("No vendor sources configured")
		return nil
	}

	// Filter sources based on component and tags
	filteredSources := filterSources(sources, flg.Component, flg.Tags)

	if len(filteredSources) == 0 {
		switch {
		case flg.Component != "":
			fmt.Printf("No vendor sources found for component: %s\n", flg.Component)
		case len(flg.Tags) > 0:
			fmt.Printf("No vendor sources found for tags: %v\n", flg.Tags)
		default:
			fmt.Println("No vendor sources found")
		}
		return nil
	}

	// Compare versions and display differences - pass the main vendor config file path
	return compareAndDisplayVendorDiffs(filteredSources, flg.Update, flg.Outdated, foundVendorConfigFile)
}

// extractComponentNameFromSource extracts a component name from a source URL.
func extractComponentNameFromSource(source string) string {
	// Extract the last part of the URL path as component name
	parts := strings.Split(strings.TrimSuffix(source, "/"), "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		// Remove common Git suffixes
		name = strings.TrimSuffix(name, ".git")
		return name
	}
	return source
}

// checkForVendorUpdates checks if updates are available for a vendor source.
func checkForVendorUpdates(source *schema.AtmosVendorSource, _ bool) (bool, string, error) {
	// Process the source URI template just like vendor pull does
	tmplData := struct {
		Component string
		Version   string
	}{source.Component, source.Version}

	uri, err := ProcessTmpl(nil, "version-check", source.Source, tmplData, false)
	if err != nil {
		return false, "", fmt.Errorf("failed to process source template: %w", err)
	}

	// Validate URI using the same utility as vendor pull
	if err := u.ValidateURI(uri); err != nil {
		return false, "", fmt.Errorf("invalid URI: %w", err)
	}

	// Determine source type using the same logic as vendor pull
	useOciScheme, useLocalFileSystem, sourceIsLocalFile, err := determineSourceType(&uri, "")
	if err != nil {
		return false, "", fmt.Errorf("failed to determine source type: %w", err)
	}

	currentVersion := source.Version
	if currentVersion == "" {
		currentVersion = "latest"
	}

	// Check for updates based on source type using go-getter patterns
	switch {
	case useOciScheme:
		return checkOciUpdates(uri, currentVersion)
	case useLocalFileSystem:
		return checkLocalUpdates(uri, sourceIsLocalFile, currentVersion)
	default:
		// For remote sources (Git, HTTP, etc.), try Git-based version checking
		// This covers most remote repositories that have version tags
		return checkRemoteUpdates(uri, currentVersion)
	}
}

// checkRemoteUpdates checks for updates in remote sources (Git, HTTP, etc.) using go-getter patterns.
func checkRemoteUpdates(uri, currentVersion string) (bool, string, error) {
	// Check if this is a direct file download (not a Git repository)
	if isDirectHTTPDownload(uri) {
		return false, currentVersion, errUtils.ErrVersionCheckingNotSupported
	}

	// Extract the clean Git URL using the same patterns as vendor pull
	gitURL := extractCleanGitURL(uri)

	// Try to get the latest tag first
	latestTag, err := getLatestGitTag(gitURL)
	if err != nil {
		// Fall back to commit checking if tags aren't available
		return checkCommitUpdates(gitURL, currentVersion, err)
	}

	// Check if we have a newer version
	return checkTagUpdate(latestTag, currentVersion)
}

// isDirectHTTPDownload checks if a URI is a direct file download rather than a Git repository.
func isDirectHTTPDownload(uri string) bool {
	if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		return false
	}

	// Check if this is a direct file download (like raw.githubusercontent.com files)
	if strings.Contains(uri, "raw.githubusercontent.com") {
		return true
	}

	// Check if it's not a known Git hosting service
	isGitService := strings.Contains(uri, ".git") ||
		strings.Contains(uri, "github.com") ||
		strings.Contains(uri, "gitlab.com") ||
		strings.Contains(uri, "bitbucket.org")

	return !isGitService
}

// checkTagUpdate compares a latest tag with the current version.
func checkTagUpdate(latestTag, currentVersion string) (bool, string, error) {
	if latestTag == "" || latestTag == currentVersion {
		return false, currentVersion, nil
	}

	if isVersionNewer(currentVersion, latestTag) {
		return true, latestTag, nil
	}

	return false, currentVersion, nil
}

// checkOciUpdates checks for updates in OCI registries.
func checkOciUpdates(_ string, currentVersion string) (bool, string, error) {
	// OCI version checking would require implementing OCI registry API calls
	// This is a future enhancement - for now, assume no updates are available
	return false, currentVersion, errUtils.ErrOCIVersionCheckingNotImplemented
}

// checkLocalUpdates checks for updates in local filesystem sources.
func checkLocalUpdates(uri string, _ bool, currentVersion string) (bool, string, error) {
	// Use the same file validation patterns as vendor pull
	if !u.FileExists(uri) {
		return false, currentVersion, fmt.Errorf("%w: %s", errUtils.ErrLocalSourceDoesNotExist, uri)
	}

	// Get file/directory info using the same patterns
	info, err := os.Stat(uri)
	if err != nil {
		return false, currentVersion, fmt.Errorf("failed to stat local source: %w", err)
	}

	// For local sources, use modification time as version indicator
	modTime := info.ModTime().Format("2006-01-02_15:04:05")

	if currentVersion != "latest" && currentVersion != modTime {
		return true, modTime, nil
	}

	return false, modTime, nil
}

// extractCleanGitURL extracts the actual Git repository URL using the same patterns as vendor pull.
func extractCleanGitURL(uri string) string {
	// Handle git:: prefixed URLs like the vendor pull implementation
	if strings.HasPrefix(uri, "git::") {
		// Remove git:: prefix
		gitURL := strings.TrimPrefix(uri, "git::")

		// Remove query parameters (everything after ?)
		if idx := strings.Index(gitURL, "?"); idx != -1 {
			gitURL = gitURL[:idx]
		}

		// Remove submodule paths (everything after //)
		if idx := strings.Index(gitURL, "//"); idx != -1 {
			gitURL = gitURL[:idx]
		}

		return gitURL
	}

	// Handle other Git URL formats
	if strings.Contains(uri, "?") {
		if idx := strings.Index(uri, "?"); idx != -1 {
			uri = uri[:idx]
		}
	}

	// Remove submodule paths
	if strings.Contains(uri, "//") {
		if idx := strings.Index(uri, "//"); idx != -1 {
			uri = uri[:idx]
		}
	}

	return uri
}

// getLatestGitTag gets the latest stable tag from a Git repository.
func getLatestGitTag(gitURL string) (string, error) {
	ctx := context.Background()

	// Use git ls-remote with proper error handling
	cmd := exec.CommandContext(ctx, gitCommand, gitLsRemoteCmd, "--tags", "--sort=-version:refname", gitURL)
	cmd.Env = append(os.Environ(),
		"GIT_SSH_COMMAND=ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -o ConnectTimeout=10",
		gitTerminalPrompt0)

	output, err := cmd.Output()
	if err != nil {
		// Try HTTPS fallback
		httpsURL := convertSSHToHTTPS(gitURL)
		if httpsURL == gitURL {
			return "", fmt.Errorf("failed to execute git ls-remote: %w", err)
		}
		cmd = exec.CommandContext(ctx, gitCommand, gitLsRemoteCmd, "--tags", "--sort=-version:refname", httpsURL)
		cmd.Env = append(os.Environ(), gitTerminalPrompt0)
		output, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to execute git ls-remote (both SSH and HTTPS): %w", err)
		}
	}

	return parseLatestStableTag(string(output))
}

// getLatestGitCommit gets the latest commit from a Git repository.
func getLatestGitCommit(gitURL string) (string, error) {
	ctx := context.Background()

	cmd := exec.CommandContext(ctx, gitCommand, gitLsRemoteCmd, gitURL, "HEAD")
	cmd.Env = append(os.Environ(),
		"GIT_SSH_COMMAND=ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -o ConnectTimeout=10",
		gitTerminalPrompt0)

	output, err := cmd.Output()
	if err != nil {
		// Try HTTPS fallback
		httpsURL := convertSSHToHTTPS(gitURL)
		if httpsURL == gitURL {
			return "", fmt.Errorf("failed to execute git ls-remote: %w", err)
		}
		cmd = exec.CommandContext(ctx, gitCommand, gitLsRemoteCmd, httpsURL, "HEAD")
		cmd.Env = append(os.Environ(), gitTerminalPrompt0)
		output, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to execute git ls-remote (both SSH and HTTPS): %w", err)
		}
	}

	return parseLatestCommit(string(output))
}

// parseLatestStableTag parses git ls-remote output to find the latest stable tag.
func parseLatestStableTag(output string) (string, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", errUtils.ErrNoTagsFound
	}

	// Parse all lines to find the latest stable tag
	var latestTag string
	var latestVersion semverlib.Version

	for _, line := range lines {
		tag, version := parseGitTagLine(line)
		if tag == "" || version == nil {
			continue
		}

		if shouldUpdateLatestTag(latestTag, &latestVersion, tag, version) {
			latestTag = tag
			latestVersion = *version
		}
	}

	if latestTag == "" {
		return "", errUtils.ErrNoStableReleaseTags
	}

	return latestTag, nil
}

// parseGitTagLine parses a single line from git ls-remote output.
func parseGitTagLine(line string) (string, *semverlib.Version) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", nil
	}

	// Extract tag name from refs/tags/tagname format
	tagRef := parts[1]
	tag := strings.TrimPrefix(tagRef, "refs/tags/")
	tag = strings.TrimSuffix(tag, "^{}")

	// Skip empty tags or pre-release tags
	if tag == "" || isPreReleaseTag(tag) {
		return "", nil
	}

	// Parse version for comparison
	version, err := semverlib.NewVersion(tag)
	if err != nil {
		return "", nil
	}

	return tag, version
}

// shouldUpdateLatestTag determines if a new tag should become the latest.
func shouldUpdateLatestTag(currentLatest string, currentVersion *semverlib.Version, _ string, newVersion *semverlib.Version) bool {
	return currentLatest == "" || newVersion.GreaterThan(currentVersion)
}

// parseLatestCommit parses git ls-remote output to extract the latest commit.
func parseLatestCommit(output string) (string, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", errUtils.ErrNoCommitsFound
	}

	parts := strings.Fields(lines[0])
	if len(parts) < 1 {
		return "", errUtils.ErrInvalidGitLsRemoteOutput
	}

	commit := parts[0]
	if commit == "" {
		return "", errUtils.ErrNoValidCommitsFound
	}

	return commit, nil
}

// convertSSHToHTTPS converts SSH Git URLs to HTTPS URLs.
func convertSSHToHTTPS(gitURL string) string {
	// Convert git@github.com:user/repo.git to https://github.com/user/repo.git
	if strings.HasPrefix(gitURL, "git@github.com:") {
		path := strings.TrimPrefix(gitURL, "git@github.com:")
		return "https://github.com/" + path
	}

	// Convert git@gitlab.com:user/repo.git to https://gitlab.com/user/repo.git
	if strings.HasPrefix(gitURL, "git@gitlab.com:") {
		path := strings.TrimPrefix(gitURL, "git@gitlab.com:")
		return "https://gitlab.com/" + path
	}

	return gitURL
}

// isVersionNewer uses Masterminds/semver for semantic version comparison.
func isVersionNewer(current, latest string) bool {
	if current == "latest" {
		return false
	}

	// Remove 'v' prefix for compatibility
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	currentVer, err1 := semverlib.NewVersion(current)
	latestVer, err2 := semverlib.NewVersion(latest)
	if err1 != nil || err2 != nil {
		return false
	}
	return latestVer.GreaterThan(currentVer)
}

// isCommitHash checks if a version string looks like a Git commit hash using regex.
func isCommitHash(version string) bool {
	return commitHashRegex.MatchString(version)
}

// isPreReleaseTag checks if a tag is a pre-release (contains alpha, beta, rc, etc.)
func isPreReleaseTag(tag string) bool {
	lowerTag := strings.ToLower(tag)
	preReleasePrefixes := []string{"-rc", "-alpha", "-beta", "-pre", "-dev", "-snapshot"}

	for _, prefix := range preReleasePrefixes {
		if strings.Contains(lowerTag, prefix) {
			return true
		}
	}

	return false
}

// checkCommitUpdates checks for updates using Git commit hashes when tags are not available.
func checkCommitUpdates(gitURL, currentVersion string, tagErr error) (bool, string, error) {
	latestCommit, commitErr := getLatestGitCommit(gitURL)
	if commitErr != nil {
		return false, "", fmt.Errorf("failed to check Git updates: %w (tag check also failed: %v)", commitErr, tagErr)
	}

	// If current version looks like a commit hash, compare
	if isCommitHash(currentVersion) && latestCommit != "" {
		if currentVersion != latestCommit[:min(len(currentVersion), len(latestCommit))] {
			return true, latestCommit[:gitShortHashLength], nil // Show short commit hash
		}
	} else if latestCommit != "" {
		return true, latestCommit[:gitShortHashLength], nil
	}

	return false, currentVersion, nil
}
