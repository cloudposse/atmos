package updater

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestValidateSelectors(t *testing.T) {
	tests := []struct {
		name       string
		invocation Invocation
		config     ValidationConfig
		wantError  string
	}{
		{name: "conflicting all selector", invocation: Invocation{All: true, Group: "platform"}, config: ValidationConfig{Format: "table"}, wantError: "--all cannot"},
		{name: "conflicting group and component selectors", invocation: Invocation{Group: "platform", Components: []string{"vpc"}}, config: ValidationConfig{Format: "table"}, wantError: "--group and --component"},
		{name: "missing group", invocation: Invocation{Group: "platform"}, config: ValidationConfig{Format: "table"}, wantError: "is not configured"},
		{name: "invalid format", invocation: Invocation{}, config: ValidationConfig{Format: "yaml"}, wantError: "--format"},
		{name: "configured group passes", invocation: Invocation{Group: "platform"}, config: ValidationConfig{Format: "table", GroupConfigured: true}},
		{name: "no selectors passes", invocation: Invocation{}, config: ValidationConfig{Format: "json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSelectors(tt.invocation, &tt.config)
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestValidateConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		config    ValidationConfig
		wantError string
	}{
		{name: "invalid execution mode", config: ValidationConfig{ExecutionMode: "invalid"}, wantError: "execution.mode"},
		{name: "invalid batching mode", config: ValidationConfig{BatchingMode: "invalid"}, wantError: "batching.mode"},
		// "component" is a deferred future value, not currently accepted at all -- it must be
		// rejected the same way any other unsupported string is, not given special-cased handling.
		{name: "component batching mode is rejected like any other unsupported value", config: ValidationConfig{BatchingMode: "component"}, wantError: "batching.mode"},
		{name: "empty configuration passes", config: ValidationConfig{}},
		{name: "current execution and scope batching passes", config: ValidationConfig{ExecutionMode: "current", BatchingMode: "scope"}},
		{name: "worktree execution and scope batching passes", config: ValidationConfig{ExecutionMode: "worktree", BatchingMode: "scope"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfiguration(&tt.config)
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestValidatePullRequestTemplates(t *testing.T) {
	tests := []struct {
		name      string
		templates PRTemplates
		wantError string
	}{
		{name: "invalid title", templates: PRTemplates{Title: "{{"}, wantError: "invalid pull request template"},
		{name: "invalid body", templates: PRTemplates{Body: "{{ .broken"}, wantError: "invalid pull request template"},
		{name: "empty templates pass", templates: PRTemplates{}},
		{name: "valid templates pass", templates: PRTemplates{Title: "{{ .scope.name }}", Body: "{{ .updates | markdownTable }}"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePullRequestTemplates(tt.templates)
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestValidateInvocation(t *testing.T) {
	tests := []struct {
		name                     string
		invocation               Invocation
		config                   ValidationConfig
		checkExplicitlyRequested bool
		wantError                string
	}{
		{name: "conflicting all selector", invocation: Invocation{All: true, Group: "platform"}, config: ValidationConfig{Format: "table"}, wantError: "--all cannot"},
		{name: "conflicting group and component selectors", invocation: Invocation{Group: "platform", Components: []string{"vpc"}}, config: ValidationConfig{Format: "table"}, wantError: "--group and --component"},
		{name: "missing group", invocation: Invocation{Group: "platform"}, config: ValidationConfig{Format: "table"}, wantError: "is not configured"},
		{name: "invalid format", invocation: Invocation{}, config: ValidationConfig{Format: "yaml"}, wantError: "--format"},
		{name: "invalid execution mode", invocation: Invocation{}, config: ValidationConfig{Format: "table", ExecutionMode: "invalid"}, wantError: "execution.mode"},
		{name: "invalid batching mode", invocation: Invocation{}, config: ValidationConfig{Format: "table", BatchingMode: "invalid"}, wantError: "batching.mode"},
		{name: "component batching mode is rejected like any other unsupported value", invocation: Invocation{}, config: ValidationConfig{Format: "table", ExecutionMode: "worktree", BatchingMode: "component"}, wantError: "batching.mode"},
		{
			name:       "invalid template",
			invocation: Invocation{PullRequest: true},
			config:     ValidationConfig{Format: "table", Templates: PRTemplates{Title: "{{"}},
			wantError:  "invalid pull request template",
		},
		{
			name:                     "pull request check is a dry run and skips template validation",
			invocation:               Invocation{PullRequest: true},
			config:                   ValidationConfig{Format: "table", Templates: PRTemplates{Title: "{{"}},
			checkExplicitlyRequested: true,
		},
		{
			name:       "pull request with valid templates passes",
			invocation: Invocation{PullRequest: true},
			config:     ValidationConfig{Format: "table", Templates: PRTemplates{Title: "{{ .scope.name }}"}},
		},
		{name: "no selectors and no pull request passes", invocation: Invocation{}, config: ValidationConfig{Format: "json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInvocation(tt.invocation, &tt.config, tt.checkExplicitlyRequested)
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrComponentUpdaterConfig))
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}
