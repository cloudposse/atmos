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

	// Avoid unused-parameter compile error; pass config to perf if available.
	_ = authManager

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

		identityTable, err := createIdentitiesTable(identities)
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
		warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorOrange))
		output.WriteString(warningStyle.Render("No providers or identities configured."))
		output.WriteString(newline)
	}

	return output.String(), nil
}

// createProvidersTable creates a table for providers.
func createProvidersTable(providers map[string]schema.Provider) (table.Model, error) {
	defer perf.Track(nil, "list.createProvidersTable")()

	columns := []table.Column{
		{Title: "NAME", Width: providerNameWidth},
		{Title: "KIND", Width: providerKindWidth},
		{Title: "REGION", Width: providerRegionWidth},
		{Title: "START URL / URL", Width: providerURLWidth},
		{Title: "DEFAULT", Width: providerDefaultWidth},
	}

	rows := buildProviderRows(providers)

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(len(rows)),
	)

	applyTableStyles(&t)

	return t, nil
}

// buildProviderRows builds table rows from providers.
func buildProviderRows(providers map[string]schema.Provider) []table.Row {
	rows := make([]table.Row, 0, len(providers))

	// Sort provider names for consistent output.
	names := getSortedProviderNames(providers)

	for _, name := range names {
		provider := providers[name]

		// Determine URL to display.
		url := emptyMarker
		if provider.StartURL != "" {
			url = truncateString(provider.StartURL, maxURLDisplay)
		} else if provider.URL != "" {
			url = truncateString(provider.URL, maxURLDisplay)
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
func createIdentitiesTable(identities map[string]schema.Identity) (table.Model, error) {
	defer perf.Track(nil, "list.createIdentitiesTable")()

	columns := []table.Column{
		{Title: "NAME", Width: identityNameWidth},
		{Title: "KIND", Width: identityKindWidth},
		{Title: "VIA PROVIDER", Width: identityViaProviderWidth},
		{Title: "VIA IDENTITY", Width: identityViaIdentityWidth},
		{Title: "DEFAULT", Width: identityDefaultWidth},
		{Title: "ALIAS", Width: identityAliasWidth},
	}

	rows := make([]table.Row, 0, len(identities))

	// Sort identity names for consistent output.
	names := make([]string, 0, len(identities))
	for name := range identities {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		identity := identities[name]
		row := buildIdentityTableRow(&identity, name)
		rows = append(rows, row)
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(len(rows)),
	)

	applyTableStyles(&t)

	return t, nil
}

// buildIdentityTableRow builds a table row for a single identity.
func buildIdentityTableRow(identity *schema.Identity, name string) table.Row {
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

	return table.Row{
		name,
		identity.Kind,
		viaProvider,
		viaIdentity,
		defaultStr,
		alias,
	}
}
