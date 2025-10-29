package list

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/lipgloss"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Table dimensions.
	providerNameWidth    = 15
	providerKindWidth    = 30
	providerRegionWidth  = 12
	providerURLWidth     = 35
	providerDefaultWidth = 7

	identityNameWidth        = 18
	identityKindWidth        = 22
	identityViaProviderWidth = 18
	identityViaIdentityWidth = 18
	identityDefaultWidth     = 7
	identityAliasWidth       = 15
	identityExpiresWidth     = 10

	// Formatting.
	defaultMarker = "✓"
	emptyMarker   = "-"
	maxURLDisplay = 32
	newline       = "\n"

	// Tree colors.
	treeBranchColor = "#555555" // Dark grey for tree branches.
	treeKeyColor    = "#888888" // Medium grey for keys.
	treeValueColor  = "#FFFFFF" // White for values.

	// Status indicator - expiration thresholds.
	expiringThreshold = 15 * time.Minute // Show yellow dot when credentials expire within 15 minutes.

	// Time constants for duration formatting.
	secondsPerMinute = 60
)

// Identity authentication status.
type authStatus int

const (
	authStatusUnknown  authStatus = iota // No credentials found or unable to check.
	authStatusExpired                    // Credentials exist but are expired.
	authStatusExpiring                   // Credentials valid but expiring soon.
	authStatusValid                      // Credentials valid with sufficient time remaining.
)

// getSortedProviderNames returns sorted provider names.
func getSortedProviderNames(providers map[string]schema.Provider) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// getSortedIdentityNames returns sorted identity names.
func getSortedIdentityNames(identities map[string]schema.Identity) []string {
	names := make([]string, 0, len(identities))
	for name := range identities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// truncateString truncates a string to the specified length with ellipsis.
func truncateString(s string, maxLen int) string {
	defer perf.Track(nil, "list.truncateString")()

	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// getIdentityAuthStatus checks if an identity is authenticated and returns its status.
func getIdentityAuthStatus(authManager authTypes.AuthManager, identityName string) authStatus {
	defer perf.Track(nil, "list.getIdentityAuthStatus")()

	// If authManager is nil (e.g., in tests), return unknown.
	if authManager == nil {
		return authStatusUnknown
	}

	// Try to get cached credentials (passive check without triggering auth).
	ctx := context.Background()
	whoami, err := authManager.GetCachedCredentials(ctx, identityName)
	if err != nil || whoami == nil {
		return authStatusUnknown
	}

	// Check if credentials have expiration.
	if whoami.Expiration == nil {
		// No expiration means credentials don't expire (e.g., long-term keys).
		return authStatusValid
	}

	// Check if expired or expiring soon.
	now := time.Now()
	timeUntilExpiry := whoami.Expiration.Sub(now)

	if timeUntilExpiry <= 0 {
		return authStatusExpired
	}

	if timeUntilExpiry <= expiringThreshold {
		return authStatusExpiring
	}

	return authStatusValid
}

// getStatusIndicator returns a colored dot indicator based on authentication status.
// Uses the same ANSI colored dots pattern as the version list command.
func getStatusIndicator(status authStatus) string {
	defer perf.Track(nil, "list.getStatusIndicator")()

	// Use lipgloss colors matching the version list command.
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))  // Green.
	yellowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Yellow.
	redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))     // Red.

	switch status {
	case authStatusValid:
		return greenStyle.Render("●") // Green dot - authenticated and valid.
	case authStatusExpiring:
		return yellowStyle.Render("●") // Yellow dot - authenticated but expiring soon.
	case authStatusExpired:
		return redStyle.Render("●") // Red dot - credentials expired.
	case authStatusUnknown:
		return " " // No indicator - not authenticated or unknown.
	default:
		return " "
	}
}

// getExpirationInfo returns formatted expiration time remaining and its status.
// Returns empty string if no expiration information available.
func getExpirationInfo(authManager authTypes.AuthManager, identityName string) (string, authStatus) {
	defer perf.Track(nil, "list.getExpirationInfo")()

	// If authManager is nil (e.g., in tests), return unknown.
	if authManager == nil {
		return "", authStatusUnknown
	}

	// Try to get cached credentials (passive check without triggering auth).
	ctx := context.Background()
	whoami, err := authManager.GetCachedCredentials(ctx, identityName)
	if err != nil || whoami == nil {
		return "", authStatusUnknown
	}

	// Check if credentials have expiration.
	if whoami.Expiration == nil {
		// No expiration means credentials don't expire (e.g., long-term keys).
		return "", authStatusValid
	}

	// Calculate time remaining.
	now := time.Now()
	remaining := whoami.Expiration.Sub(now)

	// Determine status.
	var status authStatus
	if remaining <= 0 {
		status = authStatusExpired
	} else if remaining <= expiringThreshold {
		status = authStatusExpiring
	} else {
		status = authStatusValid
	}

	// Format duration.
	formatted := formatDuration(remaining)
	return formatted, status
}

// formatDuration formats a duration into a human-readable string.
// Examples: "2h", "45m", "30s", "expired".
func formatDuration(d time.Duration) string {
	defer perf.Track(nil, "list.formatDuration")()

	if d <= 0 {
		return "expired"
	}

	// Round to nearest second.
	d = d.Round(time.Second)

	// Format based on magnitude.
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % secondsPerMinute
	seconds := int(d.Seconds()) % secondsPerMinute

	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}

	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}

	return fmt.Sprintf("%ds", seconds)
}

// formatExpirationWithColor formats expiration time with color coding.
func formatExpirationWithColor(duration string, status authStatus) string {
	defer perf.Track(nil, "list.formatExpirationWithColor")()

	if duration == "" {
		return ""
	}

	// Use same colors as status indicator.
	var style lipgloss.Style
	switch status {
	case authStatusValid:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green.
	case authStatusExpiring:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Yellow.
	case authStatusExpired:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // Red.
	default:
		return duration // No coloring for unknown status.
	}

	return style.Render(duration)
}
