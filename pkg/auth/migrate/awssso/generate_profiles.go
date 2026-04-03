// Package awssso implements the AWS SSO migration steps.
package awssso

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"unicode"

	"github.com/cloudposse/atmos/pkg/auth/migrate"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// profileTemplate is the Go template for rendering profile YAML files.
const profileTemplate = `auth:
  providers:
    {{ .ProviderName }}:
      kind: aws/iam-identity-center
      region: {{ .Region }}
      start_url: {{ .StartURL }}
      session:
        duration: 12h

  identities:
{{- range .Identities }}
    {{ .IdentityKey }}:
      kind: aws/permission-set
      via:
        provider: {{ .ProviderName }}
      principal:
        name: {{ .PermissionSetName }}
        account:
          name: {{ .AccountName }}
{{- end }}
`

// profileIdentity holds the data for a single identity entry in the template.
type profileIdentity struct {
	IdentityKey       string
	AccountName       string
	PermissionSetName string
	ProviderName      string
}

// profileData holds all data needed to render a profile YAML file.
type profileData struct {
	ProviderName string
	Region       string
	StartURL     string
	Identities   []profileIdentity
}

// GenerateProfiles creates profile directories with atmos.yaml files from SSO group assignments.
type GenerateProfiles struct {
	migCtx *migrate.MigrationContext
	fs     migrate.FileSystem
}

// NewGenerateProfiles creates a new generate profiles step.
func NewGenerateProfiles(migCtx *migrate.MigrationContext, fs migrate.FileSystem) *GenerateProfiles {
	return &GenerateProfiles{migCtx: migCtx, fs: fs}
}

// Name returns the step identifier.
func (s *GenerateProfiles) Name() string { return "generate-profiles" }

// Description returns a human-readable description of the step.
func (s *GenerateProfiles) Description() string {
	return "Generate profile directories from SSO group assignments"
}

// Detect checks if profile directories already exist with auth configuration.
func (s *GenerateProfiles) Detect(ctx context.Context) (migrate.StepStatus, error) {
	defer perf.Track(nil, "awssso.GenerateProfiles.Detect")()

	// If there are no SSO group assignments, there's nothing to generate profiles from.
	if s.migCtx.SSOConfig == nil || len(s.migCtx.SSOConfig.AccountAssignments) == 0 {
		log.Debug("No SSO group assignments found — profile generation not applicable",
			"sso_config_nil", s.migCtx.SSOConfig == nil)
		return migrate.StepNotApplicable, nil
	}

	log.Debug("SSO group assignments found", "groups", len(s.migCtx.SSOConfig.AccountAssignments))

	// Check if profiles directory exists with subdirectories containing atmos.yaml files.
	pattern := filepath.Join(s.migCtx.ProfilesPath, "*", "atmos.yaml")
	matches, err := s.fs.Glob(pattern)
	if err != nil {
		log.Debug("Error globbing for profiles", "pattern", pattern, "error", err)
		return migrate.StepNeeded, nil
	}

	if len(matches) > 0 {
		log.Debug("Profiles already exist", "matches", matches)
		return migrate.StepComplete, nil
	}

	log.Debug("No profiles found — generation needed", "pattern", pattern)
	return migrate.StepNeeded, nil
}

// Plan returns the list of changes this step would make.
func (s *GenerateProfiles) Plan(ctx context.Context) ([]migrate.Change, error) {
	defer perf.Track(nil, "awssso.GenerateProfiles.Plan")()

	if s.migCtx.SSOConfig == nil || len(s.migCtx.SSOConfig.AccountAssignments) == 0 {
		return nil, nil
	}

	// Collect and sort group names for deterministic output.
	groups := make([]string, 0, len(s.migCtx.SSOConfig.AccountAssignments))
	for group := range s.migCtx.SSOConfig.AccountAssignments {
		groups = append(groups, group)
	}
	sort.Strings(groups)

	changes := make([]migrate.Change, 0, len(groups))
	for _, group := range groups {
		profileName := normalizeProfileName(group)
		profilePath := filepath.Join("profiles", profileName, "atmos.yaml")
		content, err := s.renderProfile(group)
		if err != nil {
			return nil, fmt.Errorf("failed to render profile for group %q: %w", group, err)
		}

		changes = append(changes, migrate.Change{
			FilePath:    profilePath,
			Description: fmt.Sprintf("Create profile %q (from group %q)", profileName, group),
			Detail:      content,
		})
	}

	return changes, nil
}

// Apply executes the migration step by writing profile YAML files.
func (s *GenerateProfiles) Apply(ctx context.Context) error {
	defer perf.Track(nil, "awssso.GenerateProfiles.Apply")()

	if s.migCtx.SSOConfig == nil || len(s.migCtx.SSOConfig.AccountAssignments) == 0 {
		ui.Warning("No SSO group assignments found — skipping profile generation.")
		return nil
	}

	// Collect and sort group names for deterministic output.
	groups := make([]string, 0, len(s.migCtx.SSOConfig.AccountAssignments))
	for group := range s.migCtx.SSOConfig.AccountAssignments {
		groups = append(groups, group)
	}
	sort.Strings(groups)

	for _, group := range groups {
		content, err := s.renderProfile(group)
		if err != nil {
			return fmt.Errorf("failed to render profile for group %q: %w", group, err)
		}

		profilePath := filepath.Join(s.migCtx.ProfilesPath, normalizeProfileName(group), "atmos.yaml")
		if err := s.fs.WriteFile(profilePath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("failed to write profile for group %q: %w", group, err)
		}
	}

	return nil
}

// renderProfile renders the profile YAML for a given group.
func (s *GenerateProfiles) renderProfile(group string) (string, error) {
	assignments := s.migCtx.SSOConfig.AccountAssignments[group]

	// Build identity list from all permission-set → account mappings.
	var identities []profileIdentity

	// Sort permission set names for deterministic output.
	permSets := make([]string, 0, len(assignments))
	for ps := range assignments {
		permSets = append(permSets, ps)
	}
	sort.Strings(permSets)

	for _, ps := range permSets {
		accounts := assignments[ps]
		// Sort account names for deterministic output.
		sortedAccounts := make([]string, len(accounts))
		copy(sortedAccounts, accounts)
		sort.Strings(sortedAccounts)

		for _, acct := range sortedAccounts {
			identities = append(identities, profileIdentity{
				IdentityKey:       acct + "/" + camelToSnake(ps),
				AccountName:       acct,
				PermissionSetName: ps,
				ProviderName:      s.migCtx.SSOConfig.ProviderName,
			})
		}
	}

	data := profileData{
		ProviderName: s.migCtx.SSOConfig.ProviderName,
		Region:       s.migCtx.SSOConfig.Region,
		StartURL:     s.migCtx.SSOConfig.StartURL,
		Identities:   identities,
	}

	tmpl, err := template.New("profile").Parse(profileTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// normalizeProfileName converts a group name like "PP_InfraManagers" to
// lowercase snake case with common prefixes removed: "infra_managers".
func normalizeProfileName(group string) string {
	// Strip known prefixes (case-insensitive).
	name := group
	for _, prefix := range []string{"PP_", "pp_"} {
		if strings.HasPrefix(name, prefix) {
			name = name[len(prefix):]
			break
		}
	}

	return camelToSnake(name)
}

// camelToSnake converts "InfraManagers" → "infra_managers",
// "DevOps" → "dev_ops", "DB_PLAT_PROD_RO" → "db_plat_prod_ro".
func camelToSnake(s string) string {
	var buf strings.Builder
	runes := []rune(s)

	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			if unicode.IsLower(prev) {
				buf.WriteByte('_')
			} else if i+1 < len(runes) && unicode.IsLower(runes[i+1]) && prev != '_' {
				buf.WriteByte('_')
			}
		}
		if r != '_' {
			buf.WriteRune(unicode.ToLower(r))
		} else {
			buf.WriteByte('_')
		}
	}

	return buf.String()
}
