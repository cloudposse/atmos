// Package awssso implements the AWS SSO migration steps.
package awssso

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/migrate"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ConfigureProvider adds or updates the SSO auth provider in atmos.yaml.
type ConfigureProvider struct {
	migCtx *migrate.MigrationContext
	fs     migrate.FileSystem
}

// NewConfigureProvider creates a new configure provider step.
func NewConfigureProvider(migCtx *migrate.MigrationContext, fs migrate.FileSystem) *ConfigureProvider {
	return &ConfigureProvider{migCtx: migCtx, fs: fs}
}

// Name returns the step identifier.
func (s *ConfigureProvider) Name() string { return "configure-provider" }

// Description returns a human-readable description of the step.
func (s *ConfigureProvider) Description() string { return "Configure SSO provider in atmos.yaml" }

// Detect checks whether the SSO provider is already configured.
func (s *ConfigureProvider) Detect(ctx context.Context) (migrate.StepStatus, error) {
	defer perf.Track(nil, "awssso.ConfigureProvider.Detect")()

	if s.migCtx.ExistingAuth == nil {
		return migrate.StepNeeded, nil
	}

	// Check if any provider is already configured with SSO kind.
	for _, provider := range s.migCtx.ExistingAuth.Providers {
		if provider.Kind == "aws/iam-identity-center" {
			return migrate.StepComplete, nil
		}
	}

	return migrate.StepNeeded, nil
}

// Plan returns the list of changes this step would make.
func (s *ConfigureProvider) Plan(ctx context.Context) ([]migrate.Change, error) {
	defer perf.Track(nil, "awssso.ConfigureProvider.Plan")()

	yamlBlock, err := s.generateAuthBlock()
	if err != nil {
		return nil, err
	}

	return []migrate.Change{
		{
			FilePath:    s.migCtx.AtmosConfigPath,
			Description: "Add SSO auth provider configuration",
			Detail:      yamlBlock,
		},
	}, nil
}

// Apply executes the migration step by appending the auth block to atmos.yaml.
func (s *ConfigureProvider) Apply(ctx context.Context) error {
	defer perf.Track(nil, "awssso.ConfigureProvider.Apply")()

	yamlBlock, err := s.generateAuthBlock()
	if err != nil {
		return err
	}

	// Read existing atmos.yaml.
	existing, err := s.fs.ReadFile(s.migCtx.AtmosConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read atmos.yaml: %w", err)
	}

	// Append the auth block.
	content := append(existing, []byte("\n"+yamlBlock)...)

	return s.fs.WriteFile(s.migCtx.AtmosConfigPath, content, 0o644)
}

// generateAuthBlock renders the auth YAML block from the SSOConfig.
func (s *ConfigureProvider) generateAuthBlock() (string, error) {
	if s.migCtx.SSOConfig == nil {
		return "", fmt.Errorf("%w: SSO configuration required", errUtils.ErrSSOConfigNotFound)
	}

	const authTemplate = `auth:
  providers:
    {{ .ProviderName }}:
      kind: aws/iam-identity-center
      region: {{ .Region }}
      start_url: {{ .StartURL }}
      auto_provision_identities: true
      session:
        duration: 12h
      console:
        session_duration: 12h
`
	tmpl, err := template.New("auth").Parse(authTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, s.migCtx.SSOConfig); err != nil {
		return "", err
	}

	return buf.String(), nil
}
