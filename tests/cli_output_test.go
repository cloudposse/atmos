package tests

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"
	"github.com/muesli/termenv"

	log "github.com/cloudposse/atmos/pkg/logger"
)

func init() {
	// Initialize logger with default settings.
	// Verbosity level will be configured in TestMain based on -v flag.
	logger = log.New()
	logger.SetOutput(os.Stderr)

	// Ensure that Lipgloss uses terminal colors for tests
	lipgloss.SetColorProfile(termenv.TrueColor)

	styles := log.DefaultStyles()
	styles.Levels[log.ErrorLevel] = lipgloss.NewStyle().
		SetString("ERROR").
		Padding(0, 0, 0, 0).
		Background(lipgloss.Color("204")).
		Foreground(lipgloss.Color("0"))
	styles.Levels[log.FatalLevel] = lipgloss.NewStyle().
		SetString("FATAL").
		Padding(0, 0, 0, 0).
		Background(lipgloss.Color("204")).
		Foreground(lipgloss.Color("0"))
	// Add a custom style for key `err`
	styles.Keys["err"] = lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
	styles.Values["err"] = lipgloss.NewStyle().Bold(true)
	logger.SetStyles(styles)
	logger.SetColorProfile(termenv.TrueColor)
}

// Determine if running in a CI environment.
func isCIEnvironment() bool {
	// Check for common CI environment variables
	// Note, that the CI variable has many possible truthy values, so we check for any non-empty value that is not "false".
	return (os.Getenv("CI") != "" && os.Getenv("CI") != "false") || os.Getenv("GITHUB_ACTIONS") == "true"
}

// collapseExtraSlashes replaces multiple consecutive slashes with a single slash.
func collapseExtraSlashes(s string) string {
	// Normalize the protocol to have exactly two slashes after http: or https:
	protocolRegex := regexp.MustCompile(`(?i)(https?):/*`)
	s = protocolRegex.ReplaceAllString(s, "$1://")

	// Split into protocol and the rest of the URL
	parts := regexp.MustCompile(`(?i)^(https?://)(.*)$`).FindStringSubmatch(s)
	if len(parts) == 3 {
		protocol := parts[1]
		rest := parts[2]
		// Collapse multiple slashes in the rest part
		rest = regexp.MustCompile(`/+`).ReplaceAllString(rest, "/")
		// Remove any leading slashes after the protocol to avoid triple slashes
		rest = strings.TrimLeft(rest, "/")
		return protocol + rest
	}

	// If no protocol, collapse all slashes
	return regexp.MustCompile(`/+`).ReplaceAllString(s, "/")
}

// sanitizeOption is a functional option for customizing output sanitization.
type sanitizeOption func(*sanitizeConfig)

// sanitizeConfig holds configuration for sanitization.
type sanitizeConfig struct {
	customReplacements map[string]string // pattern -> replacement (applied as regexp.ReplaceAllString)
}

// WithCustomReplacements adds custom pattern replacements to the sanitization process.
// The patterns are treated as regular expressions and applied after all standard sanitization steps.
// This is useful for one-off cases specific to certain tests that don't need global sanitization.
//
// Example:
//
//	sanitizeOutput(output, WithCustomReplacements(map[string]string{
//	    `session-[0-9]+`: "session-12345",
//	    `temp_[a-z]+`:    "temp_xyz",
//	}))
func WithCustomReplacements(replacements map[string]string) sanitizeOption {
	return func(c *sanitizeConfig) {
		if c.customReplacements == nil {
			c.customReplacements = make(map[string]string)
		}
		for pattern, replacement := range replacements {
			c.customReplacements[pattern] = replacement
		}
	}
}

// sanitizeOutput replaces occurrences of the repository's absolute path in the output
// with the placeholder "/absolute/path/to/repo". It first normalizes both the repository root
// and the output to use forward slashes, ensuring that the replacement works reliably.
// An error is returned if the repository root cannot be determined.
// Convert something like:
//
//	D:\\a\atmos\atmos\examples\demo-stacks\stacks\deploy\**\*
//	   --> /absolute/path/to/repo/examples/demo-stacks/stacks/deploy/**/*
//	/home/runner/work/atmos/atmos/examples/demo-stacks/stacks/deploy/**/*
//	   --> /absolute/path/to/repo/examples/demo-stacks/stacks/deploy/**/*
//
// Custom replacements can be provided via WithCustomReplacements option for test-specific sanitization.
func sanitizeOutput(output string, opts ...sanitizeOption) (string, error) {
	// Apply options to configuration.
	config := &sanitizeConfig{}
	for _, opt := range opts {
		opt(config)
	}
	// 1. Get the repository root.
	repoRoot, err := findGitRepoRoot(startingDir)
	if err != nil {
		return "", err
	}

	if repoRoot == "" {
		return "", errors.New("failed to determine repository root")
	}

	// 2. Pre-process: Join word-wrapped paths that were broken by terminal width wrapping.
	// Glamour/terminal rendering may wrap long paths at arbitrary positions, breaking paths like:
	//   "/Users/erik/conductor/atmos/.conductor/da-\nnang/tests/..." (broken mid-word)
	// This regex finds the repo root path broken across lines and rejoins it.
	// We need to handle this BEFORE normalizing because the repo root regex won't match split paths.
	normalizedRepoRoot := collapseExtraSlashes(filepath.ToSlash(filepath.Clean(repoRoot)))

	// Build a pattern that matches the repo root potentially split by newlines anywhere.
	// For path "/a/b/c", create pattern that allows optional \n between characters.
	// We need to handle escape sequences from QuoteMeta carefully - don't insert [\n]?
	// between \ and the character it escapes (like \. for literal dot).
	var wrappedRootPattern strings.Builder
	runes := []rune(normalizedRepoRoot)
	for i, r := range runes {
		if i > 0 {
			// Insert optional newline between characters (not at start).
			wrappedRootPattern.WriteString("[\n]?")
		}
		// Escape the rune for regex.
		escaped := regexp.QuoteMeta(string(r))
		wrappedRootPattern.WriteString(escaped)
	}
	wrappedRootRegex, err := regexp.Compile(wrappedRootPattern.String())
	if err == nil {
		// Replace any wrapped occurrences with the normalized (unwrapped) version.
		output = wrappedRootRegex.ReplaceAllString(output, normalizedRepoRoot)
	}

	// 3. Normalize the repository root:
	//    - Clean the path (which may not collapse all extra slashes after the drive letter, etc.)
	//    - Convert to forward slashes,
	//    - And explicitly collapse extra slashes.
	// Also normalize the output to use forward slashes.
	// Note: filepath.ToSlash() on Windows converts path separators; on Unix it does nothing.
	// We also need to handle Windows-style paths that may appear in test output even on Unix (for testing).
	// Replace backslashes with forward slashes, EXCEPT those that are escape sequences (\n, \t, \r, etc.).
	// Since actual CLI output has escape sequences already processed (they appear as actual newlines/tabs),
	// we can safely replace backslashes that are followed by path-like characters.
	//
	// First, protect JSON unicode escapes like \u003e from being corrupted by filepath.ToSlash
	// and the path normalization regex below. On Windows, filepath.ToSlash converts ALL backslashes
	// to forward slashes, which would turn \u003e into /u003e.
	jsonUnicodeEscape := regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)
	const unicodePlaceholder = "\x00UNICODE_ESCAPE_"
	protectedOutput := jsonUnicodeEscape.ReplaceAllString(output, unicodePlaceholder+"$1")
	normalizedOutput := filepath.ToSlash(protectedOutput)
	// Replace backslashes that look like path separators (followed by alphanumeric, ., -, _, *, etc.).
	normalizedOutput = regexp.MustCompile(`\\([a-zA-Z0-9._*\-/])`).ReplaceAllString(normalizedOutput, "/$1")
	// Restore protected unicode escapes.
	normalizedOutput = regexp.MustCompile(regexp.QuoteMeta(unicodePlaceholder)+`([0-9a-fA-F]{4})`).ReplaceAllString(normalizedOutput, `\u$1`)

	// 3. Build a regex that matches the repository root even if extra slashes appear.
	//    First, escape any regex metacharacters in the normalized repository root.
	quoted := regexp.QuoteMeta(normalizedRepoRoot)
	// Replace each literal "/" with the regex token "/+" so that e.g. "a/b/c" becomes "a/+b/+c".
	patternBody := strings.ReplaceAll(quoted, "/", "/+")
	// Allow for extra trailing slashes.
	// Use case-insensitive matching to handle Windows drive letters (D: vs d:) and path differences.
	pattern := "(?i)" + patternBody + "/*"
	repoRootRegex, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}

	// 4. Replace any occurrence of the repository root (with extra slashes) with a fixed placeholder.
	//    The placeholder will end with exactly one slash.
	placeholder := "/absolute/path/to/repo/"
	replaced := repoRootRegex.ReplaceAllString(normalizedOutput, placeholder)

	// 5. Now collapse extra slashes in the remainder of file paths that start with the placeholder.
	//    We use a regex to find segments that start with the placeholder followed by some path characters.
	//    (We assume that file paths appear in quotes or other delimited contexts, and that URLs won't match.)
	fixRegex := regexp.MustCompile(`(/absolute/path/to/repo)([^",]+)`)
	result := fixRegex.ReplaceAllStringFunc(replaced, func(match string) string {
		// The regex has two groups: group 1 is the placeholder, group 2 is the remainder.
		groups := fixRegex.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		// Collapse extra slashes in the remainder.
		fixedRemainder := collapseExtraSlashes(groups[2])
		return groups[1] + fixedRemainder
	})

	// 5b. Join hint paths that may be split across lines due to terminal width wrapping.
	// This ensures consistent snapshots across platforms with different terminal widths.
	// Example:
	//   Input:  "💡 Path points to the stacks configuration directory, not a component:\n/absolute/path/to/repo/..."
	//   Output: "💡 Path points to the stacks configuration directory, not a component: /absolute/path/to/repo/..."
	hintPathRegex := regexp.MustCompile(`(💡[^:]+:)\s*\n+(/absolute/path/to/repo[^\n]*)`)
	result = hintPathRegex.ReplaceAllString(result, "$1 $2")

	// Also handle "Stacks directory:" and "Workflows directory:" patterns.
	// Example:
	//   Input:  "Stacks directory:\n/absolute/path/to/repo/..."
	//   Output: "Stacks directory: /absolute/path/to/repo/..."
	dirPathRegex := regexp.MustCompile(`((?:Stacks|Workflows) directory:)\s*\n+(/absolute/path/to/repo[^\n]*)`)
	result = dirPathRegex.ReplaceAllString(result, "$1 $2")

	// 6. Handle URLs in the output to ensure they are normalized.
	//    Use a regex to find URLs and collapse extra slashes while preserving the protocol.
	urlRegex := regexp.MustCompile(`(https?:/+[^\s]+)`)
	result = urlRegex.ReplaceAllStringFunc(result, collapseExtraSlashes)

	// 6b. Redact volatile request IDs to avoid snapshot flakiness.
	requestIDRegex1 := regexp.MustCompile(`(?i)\bRequestI[Dd]\s*:\s*[A-Za-z0-9-]+`)
	requestIDRegex2 := regexp.MustCompile(`(?i)\bX-Amzn-RequestId\s*:\s*[A-Za-z0-9-]+`)
	result = requestIDRegex1.ReplaceAllString(result, "RequestID: <REDACTED>")
	result = requestIDRegex2.ReplaceAllString(result, "RequestID: <REDACTED>")

	// 7. Remove the random number added to file name like `atmos-import-454656846`
	filePathRegex := regexp.MustCompile(`file_path=[^ ]+/atmos-import-\d+/atmos-import-\d+\.yaml`)
	result = filePathRegex.ReplaceAllString(result, "file_path=/atmos-import/atmos-import.yaml")

	// 8. Mask PostHog tokens to prevent real tokens from appearing in snapshots.
	// Match any token starting with phc_ followed by alphanumeric characters and underscores.
	posthogTokenRegex := regexp.MustCompile(`phc_[a-zA-Z0-9_]+`)
	result = posthogTokenRegex.ReplaceAllString(result, "phc_TEST_TOKEN_PLACEHOLDER")

	// 9. Normalize expiration timestamps to avoid snapshot mismatches.
	// Replace the relative duration part (e.g., "(59m 59s)", "expired") with a deterministic placeholder.
	// This preserves the actual timestamp while normalizing the time-sensitive duration.
	// Use "1h 0m" format which matches the actual formatDuration output for hour-based durations.
	expiresRegex := regexp.MustCompile(`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\s+[A-Z]{3,4})\s+\([^)]+\)`)
	result = expiresRegex.ReplaceAllString(result, "$1 (1h 0m)")

	// 10. Normalize debug log timestamps (Go time.Time string format).
	// These appear in debug logs like: expiration="2025-10-26 23:04:36.236866 -0500 CDT m=+3600.098519710"
	// Replace with a constant timestamp to avoid snapshot mismatches.
	debugTimestampRegex := regexp.MustCompile(`expiration="[^"]+\s+[+-]\d{4}\s+[A-Z]{3,4}\s+m=[+-][\d.]+`)
	result = debugTimestampRegex.ReplaceAllString(result, `expiration="2025-01-01 12:00:00.000000 +0000 UTC m=+3600.000000000`)

	// 11. Normalize external absolute paths to avoid environment-specific paths in snapshots.
	// Replace common absolute path prefixes with generic placeholders.
	// This handles paths outside the repo (e.g., /Users/username/other-projects/).
	// Match Unix-style absolute paths (/Users/, /home/, /opt/, etc.) and Windows paths (C:\Users\, etc.).
	externalPathRegex := regexp.MustCompile(`(/Users/[^/]+/[^/]+/[^/]+/[^/\s":]+|/home/[^/]+/[^/]+/[^/]+/[^/\s":]+|C:\\Users\\[^\\]+\\[^\\]+\\[^\\]+\\[^\\\s":]+)`)
	result = externalPathRegex.ReplaceAllString(result, "/absolute/path/to/external")

	// 12. Normalize "Last Updated" timestamps in auth whoami output.
	// These appear as "Last Updated  2025-10-28 13:10:27 CDT" in table output.
	// Replace with a fixed timestamp to avoid snapshot mismatches.
	lastUpdatedRegex := regexp.MustCompile(`Last Updated\s+\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\s+[A-Z]{3,4}`)
	result = lastUpdatedRegex.ReplaceAllString(result, "Last Updated  2025-01-01 12:00:00 UTC")

	// 13. Normalize credential expiration durations in auth list output.
	// These appear as "● mock-identity (mock) [DEFAULT] 650202h14m" in tree output.
	// The duration changes every minute, so normalize to "1h 0m" like other duration normalizations.
	// Matches patterns like "650202h14m", "650194h", "1h30m", "45m", etc. at the end of identity lines.
	expirationDurationRegex := regexp.MustCompile(`(\(mock\)(?:\s+\[DEFAULT\])?)\s+\d+h(?:\d+m)?\b`)
	result = expirationDurationRegex.ReplaceAllString(result, "$1 1h 0m")

	// 14. Normalize credential_store values in error messages.
	// The keyring backend varies by platform: "system-keyring" (Mac/Windows) vs "noop" (Linux CI).
	// Replace with a stable placeholder to avoid platform-specific snapshot differences.
	credentialStoreRegex := regexp.MustCompile(`credential_store=(system-keyring|noop|file)`)
	result = credentialStoreRegex.ReplaceAllString(result, "credential_store=keyring-placeholder")

	// 15. Apply custom replacements if provided.
	// These are test-specific patterns that don't need to be part of the global sanitization.
	// IMPORTANT: This must run LAST so it can override any built-in sanitization results.
	for pattern, replacement := range config.customReplacements {
		customRegex, err := regexp.Compile(pattern)
		if err != nil {
			return "", fmt.Errorf("failed to compile custom replacement pattern %q: %w", pattern, err)
		}
		result = customRegex.ReplaceAllString(result, replacement)
	}

	// 15a. Normalize temporary directory paths from TEST_GIT_ROOT and other test fixtures.
	// These appear in trace logs as path=/var/folders/.../mock-git-root or path=/absolute/path/to/repo/mock-git-root.
	// Replace with a stable placeholder since these are test-specific paths.
	// Matches both raw paths and already-sanitized repo paths.
	tempGitRootRegex := regexp.MustCompile(`path=(/var/folders/[^\s]+/mock-git-root|/tmp/[^\s]+/mock-git-root|/Users/[^\s]+/mock-git-root|/home/[^\s]+/mock-git-root|[A-Z]:/[^\s]+/mock-git-root|/absolute/path/to/repo/mock-git-root|/absolute/path/to/external/mock-git-root)`)
	result = tempGitRootRegex.ReplaceAllString(result, "path=/mock-git-root")

	// 15b. Normalize temp home directory paths in trace logs (e.g., path=/var/folders/.../T/TestCLI.../.atmos).
	// These are used for home directory mocking in tests.
	// Matches both raw paths and already-sanitized repo paths.
	tempHomeDirRegex := regexp.MustCompile(`path=(/var/folders/[^\s]+/\.atmos|/tmp/[^\s]+/\.atmos|/Users/[^\s]+/\.atmos|/home/[^\s]+/\.atmos|[A-Z]:/[^\s]+/\.atmos|/absolute/path/to/repo/[^\s]+/\.atmos)`)
	result = tempHomeDirRegex.ReplaceAllString(result, "path=/mock-home/.atmos")

	// 15c. Normalize external absolute paths (additional pattern with forward slashes for Windows).
	// Note: Windows paths use forward slashes here because filepath.ToSlash normalizes them earlier.
	// The pattern matches the entire path including subdirectories by not excluding slashes in the final segment.
	externalPathRegex2 := regexp.MustCompile(`(/Users/[^/]+/[^/]+/[^/]+/[^\s":]+|/home/[^/]+/[^/]+/[^/]+/[^\s":]+|C:/Users/[^/]+/[^/]+/[^/]+/[^\s":]+)`)
	result = externalPathRegex2.ReplaceAllString(result, "/absolute/path/to/external")

	// 16. Normalize provisioned_by_user values in component output.
	// This field shows the current username, which varies by environment (erik, runner, etc.).
	// Replace with a generic placeholder.
	provisionedByUserRegex := regexp.MustCompile(`provisioned_by_user: [^\s]+`)
	result = provisionedByUserRegex.ReplaceAllString(result, "provisioned_by_user: user")

	// 17. Join hint messages where the sanitized path ended up on the next line.
	// This must run AFTER path sanitization because it matches the sanitized path pattern.
	// E.g., "💡 Stacks directory not found:\n/absolute/path" vs "💡 Stacks directory not found: /absolute/path"
	// Also handles plain labels like "Stacks directory:\n/path"
	hintPathRegex2 := regexp.MustCompile(`(?m)(💡[^\n]{0,200}?:|^[A-Z][^\n]{0,200}?directory:)\s*\n(/absolute/path/to/repo[^\s\n]*)`)
	result = hintPathRegex2.ReplaceAllString(result, "$1 $2")

	return result, nil
}

// sanitizeTestName converts t.Name() into a valid filename.
func sanitizeTestName(name string) string {
	// Replace slashes with underscores
	name = strings.ReplaceAll(name, "/", "_")

	// Remove or replace other problematic characters
	invalidChars := regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`) // Matches invalid filename characters
	name = invalidChars.ReplaceAllString(name, "_")

	// Trim trailing periods and spaces (Windows-specific issue)
	name = strings.TrimRight(name, " .")

	return name
}

// Drop any lines matched by the ignore patterns so they do not affect the comparison.
// stripTrailingWhitespace removes trailing whitespace from each line.
func stripTrailingWhitespace(input string) string {
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func applyIgnorePatterns(input string, patterns []string) string {
	lines := strings.Split(input, "\n") // Split input into lines
	var filteredLines []string          // Store lines that don't match the patterns

	for _, line := range lines {
		shouldIgnore := false
		for _, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			if re.MatchString(line) { // Check if the line matches the pattern
				shouldIgnore = true
				break // No need to check further patterns for this line
			}
		}
		if !shouldIgnore {
			filteredLines = append(filteredLines, line) // Add non-matching lines
		}
	}

	return strings.Join(filteredLines, "\n") // Join the filtered lines back into a string
}

// simulateTtyCommand executes a command in a pseudo-terminal (PTY) environment.
//
// IMPORTANT: PTY behavior merges stderr and stdout into a single stream!
// This is not a bug - it's how terminals work. A terminal display shows all output
// in one place; there's no separate "stderr screen" and "stdout screen".
//
// This means:
// - All output (stdout + stderr) will be captured together.
// - The returned string contains both streams merged.
// - This matches real terminal behavior where users see everything in one stream.
//
// For tests that need separate stderr/stdout streams, use non-TTY execution instead.
func simulateTtyCommand(t *testing.T, cmd *exec.Cmd, input string) (string, error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to start TTY: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	// t.Logf("PTY Fd: %d, IsTerminal: %v", ptmx.Fd(), term.IsTerminal(int(ptmx.Fd())))

	if input != "" {
		go func() {
			_, _ = ptmx.Write([]byte(input))
			_ = ptmx.Close() // Ensure we close the input after writing
		}()
	}

	var buffer bytes.Buffer
	done := make(chan error, 1)
	go func() {
		// Use ReadFrom which properly handles EOF.
		_, err := buffer.ReadFrom(ptmx)
		done <- ptyError(err) // Wrap the error handling
	}()

	err = cmd.Wait()
	if err != nil {
		logger.Info("Command execution error", "err", err)
	}

	if readErr := <-done; readErr != nil {
		return "", fmt.Errorf("failed to read PTY output: %v", readErr)
	}

	output := buffer.String()
	// t.Logf("Captured Output:\n%s", output)

	return output, nil
}

// Linux kernel return EIO when attempting to read from a master pseudo
// terminal which no longer has an open slave. So ignore error here.
// See https://github.com/creack/pty/issues/21
// See https://github.com/owenthereal/upterm/pull/11
func ptyError(err error) error {
	if pathErr, ok := err.(*os.PathError); !ok || pathErr.Err != syscall.EIO {
		return err
	}
	return nil
}
