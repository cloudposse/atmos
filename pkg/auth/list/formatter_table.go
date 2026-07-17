package list

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// RenderTable renders providers and identities as formatted tables.
func RenderTable(
	authManager authTypes.AuthManager,
	providers map[string]schema.Provider,
	identities map[string]schema.Identity,
) (string, error) {
	defer perf.Track(nil, "list.RenderTable")()

	var output strings.Builder

	// Create section header style.
	sectionHeaderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorCyan)).
		Bold(true).
		Underline(true)

	// Render providers table if we have providers.
	if len(providers) > 0 {
		providerTable, err := createProvidersTable(providers)
		if err != nil {
			return "", err
		}
		output.WriteString(sectionHeaderStyle.Render("PROVIDERS"))
		output.WriteString(newline)
		output.WriteString(providerTable.View())
		output.WriteString(newline)
	}

	// Render identities table if we have identities.
	if len(identities) > 0 {
		if len(providers) > 0 {
			output.WriteString(newline)
		}

		identityTable, err := createIdentitiesTable(authManager, identities)
		if err != nil {
			return "", err
		}
		output.WriteString(sectionHeaderStyle.Render("IDENTITIES"))
		output.WriteString(newline)
		output.WriteString(identityTable.View())
		output.WriteString(newline)
	}

	// Handle empty result.
	if len(providers) == 0 && len(identities) == 0 {
		styles := theme.GetCurrentStyles()
		output.WriteString(styles.Notice.Render("No providers or identities configured."))
		output.WriteString(newline)
	}

	return output.String(), nil
}

// createProvidersTable creates a table for providers.
func createProvidersTable(providers map[string]schema.Provider) (table.Model, error) {
	defer perf.Track(nil, "list.createProvidersTable")()

	// Without a TTY, keep the legacy fixed-size URL truncation so piped output stays stable.
	urlLimit := 0
	if !isTTYForTable() {
		urlLimit = maxURLDisplay
	}

	rows := buildProviderRows(providers, urlLimit)

	specs := []columnSizingSpec{
		{title: "NAME", legacy: providerNameWidth, floor: minProviderNameWidth},
		{title: "KIND", legacy: providerKindWidth, floor: minProviderKindWidth},
		{title: "REGION", legacy: providerRegionWidth, floor: minProviderRegionWidth},
		{title: "START URL / URL", legacy: providerURLWidth, floor: minProviderURLWidth},
		{title: "DEFAULT", legacy: providerDefaultWidth, floor: providerDefaultWidth},
	}
	// Under width pressure, shrink the most truncatable columns first: URL, then kind, then name, then region.
	widths := computeColumnWidths(specs, rows, []int{3, 1, 0, 2})
	columns := tableColumns(specs, widths)

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(len(rows)+1), // +1 for header row.
	)

	applyTableStyles(&t)

	return t, nil
}

// buildProviderRows builds table rows from providers.
// A positive urlLimit pre-truncates URLs to that many characters (the legacy
// non-TTY behavior); zero or negative keeps URLs at their natural length so
// the column can size to content.
func buildProviderRows(providers map[string]schema.Provider, urlLimit int) []table.Row {
	rows := make([]table.Row, 0, len(providers))

	// Sort provider names for consistent output.
	names := getSortedProviderNames(providers)

	for _, name := range names {
		provider := providers[name]

		// Determine URL to display.
		url := emptyMarker
		if provider.StartURL != "" {
			url = provider.StartURL
		} else if provider.URL != "" {
			url = provider.URL
		}
		if urlLimit > 0 {
			url = truncateString(url, urlLimit)
		}

		// Determine region.
		region := emptyMarker
		if provider.Region != "" {
			region = provider.Region
		}

		// Default marker.
		defaultStr := ""
		if provider.Default {
			defaultStr = defaultMarker
		}

		rows = append(rows, table.Row{
			name,
			provider.Kind,
			region,
			url,
			defaultStr,
		})
	}

	return rows
}

// applyTableStyles applies consistent theme styles to a table.
func applyTableStyles(t *table.Model) {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Background(lipgloss.Color("")).
		Bold(false)

	t.SetStyles(s)
}

// createIdentitiesTable creates a table for identities.
func createIdentitiesTable(authManager authTypes.AuthManager, identities map[string]schema.Identity) (table.Model, error) {
	defer perf.Track(nil, "list.createIdentitiesTable")()

	rows := make([]table.Row, 0, len(identities))

	// Sort identity names for consistent output.
	names := make([]string, 0, len(identities))
	for name := range identities {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		identity := identities[name]
		row := buildIdentityTableRow(authManager, &identity, name)
		rows = append(rows, row)
	}

	specs := []columnSizingSpec{
		{title: "", legacy: 1, floor: 1}, // Status indicator column.
		{title: "NAME", legacy: identityNameWidth, floor: minIdentityNameWidth},
		{title: "KIND", legacy: identityKindWidth, floor: minIdentityKindWidth},
		{title: "VIA PROVIDER", legacy: identityViaProviderWidth, floor: minIdentityViaProviderWidth},
		{title: "VIA IDENTITY", legacy: identityViaIdentityWidth, floor: minIdentityViaIdentityWidth},
		{title: "DEFAULT", legacy: identityDefaultWidth, floor: identityDefaultWidth},
		{title: "ALIAS", legacy: identityAliasWidth, floor: minIdentityAliasWidth},
		{title: "EXPIRES", legacy: identityExpiresWidth, floor: identityExpiresWidth},
	}
	// Under width pressure, shrink the most truncatable columns first: kind,
	// then alias, then via provider/identity, then name.
	widths := computeColumnWidths(specs, rows, []int{2, 6, 3, 4, 1})
	columns := tableColumns(specs, widths)

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(len(rows)+1), // +1 for header row.
	)

	applyTableStyles(&t)

	return t, nil
}

// buildIdentityTableRow builds a table row for a single identity.
func buildIdentityTableRow(authManager authTypes.AuthManager, identity *schema.Identity, name string) table.Row {
	// Get authentication status indicator.
	status := getIdentityAuthStatus(authManager, name)
	statusIndicator := getStatusIndicator(status)

	// Determine via provider.
	viaProvider := emptyMarker
	if identity.Via != nil && identity.Via.Provider != "" {
		viaProvider = identity.Via.Provider
	}

	// Determine via identity.
	viaIdentity := emptyMarker
	if identity.Via != nil && identity.Via.Identity != "" {
		viaIdentity = identity.Via.Identity
	}

	// For aws/user, show aws-user as provider.
	if identity.Kind == "aws/user" && viaProvider == emptyMarker {
		viaProvider = "aws-user"
	}

	// Default marker.
	defaultStr := ""
	if identity.Default {
		defaultStr = defaultMarker
	}

	// Alias.
	alias := emptyMarker
	if identity.Alias != "" {
		alias = identity.Alias
	}

	// Expiration.
	expiresStr := emptyMarker
	if authManager != nil {
		expirationDuration, expirationStatus := getExpirationInfo(authManager, name)
		if expirationDuration != "" {
			expiresStr = formatExpirationWithColor(expirationDuration, expirationStatus)
		}
	}

	return table.Row{
		statusIndicator, // Status dot as first column.
		name,
		identity.Kind,
		viaProvider,
		viaIdentity,
		defaultStr,
		alias,
		expiresStr,
	}
}
