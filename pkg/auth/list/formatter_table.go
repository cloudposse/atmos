package list

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// RenderTable renders providers and identities as formatted tables, going
// through the same pkg/list/renderer pipeline every other `atmos list *`
// table uses (column widths, TTY-vs-piped fallback, styling all live there
// instead of a separate implementation in this package).
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

	// Render providers table if we have providers. createProvidersTable's
	// returned string already carries its own leading/trailing blank-line
	// padding (from format.CreateStyledTableWithOptions on the TTY path), so
	// this doesn't add a redundant separator newline after it.
	if len(providers) > 0 {
		providerTable, err := createProvidersTable(providers)
		if err != nil {
			return "", err
		}
		output.WriteString(sectionHeaderStyle.Render("PROVIDERS"))
		output.WriteString(newline)
		output.WriteString(providerTable)
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
		output.WriteString(identityTable)
	}

	// Handle empty result.
	if len(providers) == 0 && len(identities) == 0 {
		styles := theme.GetCurrentStyles()
		output.WriteString(styles.Notice.Render("No providers or identities configured."))
		output.WriteString(newline)
	}

	return output.String(), nil
}

// createProvidersTable renders the providers section as a table.
func createProvidersTable(providers map[string]schema.Provider) (string, error) {
	defer perf.Track(nil, "list.createProvidersTable")()

	selector, err := column.NewSelector(providerColumns(), column.BuildColumnFuncMap())
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCreateColumnSelector, err)
	}

	r := renderer.New(nil, selector, nil, format.FormatTable, "",
		renderer.WithTableOptions(format.TableOptions{SemanticCellStyling: false}))

	return r.RenderToString(buildProviderRows(providers))
}

func providerColumns() []column.Config {
	return []column.Config{
		{Name: "NAME", Value: "{{ .name }}"},
		{Name: "KIND", Value: "{{ .kind }}"},
		{Name: "REGION", Value: "{{ .region }}"},
		{Name: "START URL / URL", Value: "{{ .url }}"},
		{Name: "DEFAULT", Value: "{{ .default }}"},
	}
}

// buildProviderRows builds renderer rows from providers, sorted by name.
func buildProviderRows(providers map[string]schema.Provider) []map[string]any {
	names := getSortedProviderNames(providers)
	rows := make([]map[string]any, 0, len(names))

	for _, name := range names {
		provider := providers[name]

		// Determine URL to display.
		url := emptyMarker
		if provider.StartURL != "" {
			url = provider.StartURL
		} else if provider.URL != "" {
			url = provider.URL
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

		rows = append(rows, map[string]any{
			"name":    name,
			"kind":    provider.Kind,
			"region":  region,
			"url":     url,
			"default": defaultStr,
		})
	}

	return rows
}

// createIdentitiesTable renders the identities section as a table.
func createIdentitiesTable(authManager authTypes.AuthManager, identities map[string]schema.Identity) (string, error) {
	defer perf.Track(nil, "list.createIdentitiesTable")()

	selector, err := column.NewSelector(identityColumns(), column.BuildColumnFuncMap())
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCreateColumnSelector, err)
	}

	r := renderer.New(nil, selector, nil, format.FormatTable, "",
		renderer.WithTableOptions(format.TableOptions{SemanticCellStyling: false}))

	return r.RenderToString(buildIdentityRows(authManager, identities))
}

func identityColumns() []column.Config {
	return []column.Config{
		// Status indicator: a single space (rather than "") since column.Config
		// requires a non-empty Name, but the header itself should read as blank.
		{Name: " ", Value: "{{ .status }}"},
		{Name: "NAME", Value: "{{ .name }}"},
		{Name: "KIND", Value: "{{ .kind }}"},
		{Name: "VIA PROVIDER", Value: "{{ .via_provider }}"},
		{Name: "VIA IDENTITY", Value: "{{ .via_identity }}"},
		{Name: "DEFAULT", Value: "{{ .default }}"},
		{Name: "ALIAS", Value: "{{ .alias }}"},
		{Name: "EXPIRES", Value: "{{ .expires }}"},
	}
}

// buildIdentityRows builds renderer rows from identities, sorted by name.
func buildIdentityRows(authManager authTypes.AuthManager, identities map[string]schema.Identity) []map[string]any {
	names := getSortedIdentityNames(identities)
	rows := make([]map[string]any, 0, len(names))

	for _, name := range names {
		identity := identities[name]
		rows = append(rows, buildIdentityRow(authManager, &identity, name))
	}

	return rows
}

// buildIdentityRow builds one renderer row for a single identity.
func buildIdentityRow(authManager authTypes.AuthManager, identity *schema.Identity, name string) map[string]any {
	statusIndicator := getStatusIndicator(getIdentityAuthStatus(authManager, name))
	viaProvider, viaIdentity := resolveIdentityVia(identity)

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

	return map[string]any{
		"status":       statusIndicator,
		"name":         name,
		"kind":         identity.Kind,
		"via_provider": viaProvider,
		"via_identity": viaIdentity,
		"default":      defaultStr,
		"alias":        alias,
		"expires":      resolveIdentityExpiration(authManager, name),
	}
}

// resolveIdentityVia determines the via-provider and via-identity display
// values, special-casing aws/user identities (which have no `via:` config)
// to show "aws-user" as their provider.
func resolveIdentityVia(identity *schema.Identity) (viaProvider, viaIdentity string) {
	viaProvider, viaIdentity = emptyMarker, emptyMarker
	if identity.Via != nil {
		if identity.Via.Provider != "" {
			viaProvider = identity.Via.Provider
		}
		if identity.Via.Identity != "" {
			viaIdentity = identity.Via.Identity
		}
	}
	if identity.Kind == "aws/user" && viaProvider == emptyMarker {
		viaProvider = "aws-user"
	}
	return viaProvider, viaIdentity
}

// resolveIdentityExpiration returns the identity's colored, formatted
// expiration string, or emptyMarker when no expiration info is available.
func resolveIdentityExpiration(authManager authTypes.AuthManager, name string) string {
	if authManager == nil {
		return emptyMarker
	}
	duration, status := getExpirationInfo(authManager, name)
	if duration == "" {
		return emptyMarker
	}
	return formatExpirationWithColor(duration, status)
}
