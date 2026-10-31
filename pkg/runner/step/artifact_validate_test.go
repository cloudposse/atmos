package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards for the schema fields these tests set.
var _ = schema.WorkflowStep{Repository: "", ArtifactName: "", Version: "", ArtifactKind: "", Force: false, ArtifactDryRun: false, Readme: "", Changelog: "", Ref: "", From: "", To: "", Document: "", SearchQuery: "", Refresh: false}

// artifactFieldSetters sets one operation input on a step, keyed by the contract field name.
var artifactFieldSetters = map[invoke.Field]func(*schema.WorkflowStep){
	invoke.FieldSource:       func(s *schema.WorkflowStep) { s.Source = "./dist" },
	invoke.FieldRepository:   func(s *schema.WorkflowStep) { s.Repository = "dir" },
	invoke.FieldArtifactName: func(s *schema.WorkflowStep) { s.ArtifactName = "team/app" },
	invoke.FieldVersion:      func(s *schema.WorkflowStep) { s.Version = "1.0.0" },
	invoke.FieldArtifactKind: func(s *schema.WorkflowStep) { s.ArtifactKind = "files" },
	invoke.FieldForce:        func(s *schema.WorkflowStep) { s.Force = true },
	invoke.FieldDryRun:       func(s *schema.WorkflowStep) { s.ArtifactDryRun = true },
	invoke.FieldMetadata: func(s *schema.WorkflowStep) {
		s.Metadata = &schema.ArtifactStepMetadata{Labels: map[string]string{"team": "platform"}}
	},
	invoke.FieldLabels:      func(s *schema.WorkflowStep) { s.Labels = "team=platform" },
	invoke.FieldReadme:      func(s *schema.WorkflowStep) { s.Readme = "README.md" },
	invoke.FieldChangelog:   func(s *schema.WorkflowStep) { s.Changelog = "CHANGELOG.md" },
	invoke.FieldRef:         func(s *schema.WorkflowStep) { s.Ref = "team/app:1.0.0" },
	invoke.FieldDestination: func(s *schema.WorkflowStep) { s.Destination = "./out" },
	invoke.FieldFrom:        func(s *schema.WorkflowStep) { s.From = "dir" },
	invoke.FieldTo:          func(s *schema.WorkflowStep) { s.To = "archive" },
	invoke.FieldDocument:    func(s *schema.WorkflowStep) { s.Document = "readme" },
	invoke.FieldQuery:       func(s *schema.WorkflowStep) { s.SearchQuery = "app" },
	invoke.FieldRefresh:     func(s *schema.WorkflowStep) { s.Refresh = true },
}

// artifactStepWith builds a step for the action with exactly the given inputs set.
func artifactStepWith(action artifact.Action, fields ...invoke.Field) *schema.WorkflowStep {
	step := &schema.WorkflowStep{Name: "artifact-step", Type: schema.TaskTypeArtifact, Action: string(action)}
	for _, field := range fields {
		artifactFieldSetters[field](step)
	}
	return step
}

func TestArtifactHandler_Identity(t *testing.T) {
	handler, ok := Get(schema.TaskTypeArtifact)
	require.True(t, ok, "the artifact step type must be registered")
	assert.Equal(t, "artifact", handler.GetName())
	assert.Equal(t, CategoryCommand, handler.GetCategory())
	assert.False(t, handler.RequiresTTY())

	known, ok := handler.(knownFieldsHandler)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{
		"action", "source", "repository", "artifact_name", "version", "artifact_kind", "force", "dry_run", "metadata",
		"labels", "readme", "changelog", "ref", "destination", "from", "to", "document", "refresh", "query",
	}, known.KnownFields())
}

// TestArtifactHandler_ValidateMatrix proves, for every action, that the required inputs are
// enforced and that every input outside the contract is rejected by name.
func TestArtifactHandler_ValidateMatrix(t *testing.T) {
	handler := &ArtifactHandler{}
	require.NotEmpty(t, invoke.Actions())
	require.NotEmpty(t, invoke.AllFields())

	for _, action := range invoke.Actions() {
		spec, ok := invoke.SpecFor(action)
		require.True(t, ok)

		t.Run(string(action)+"/valid with required fields only", func(t *testing.T) {
			require.NoError(t, handler.Validate(artifactStepWith(action, spec.Required...)))
		})
		t.Run(string(action)+"/valid with every accepted field", func(t *testing.T) {
			require.NoError(t, handler.Validate(artifactStepWith(action, spec.Fields()...)))
		})
		for _, missing := range spec.Required {
			t.Run(string(action)+"/missing "+string(missing), func(t *testing.T) {
				var present []invoke.Field
				for _, field := range spec.Required {
					if field != missing {
						present = append(present, field)
					}
				}
				err := handler.Validate(artifactStepWith(action, present...))
				require.ErrorIs(t, err, errUtils.ErrArtifactStepFieldRequired)
				assert.Contains(t, err.Error(), string(missing))
			})
		}
		for _, field := range invoke.AllFields() {
			if spec.Accepts(field) {
				continue
			}
			t.Run(string(action)+"/forbids "+string(field), func(t *testing.T) {
				err := handler.Validate(artifactStepWith(action, append(spec.Required, field)...))
				require.ErrorIs(t, err, errUtils.ErrArtifactStepFieldForbidden)
				assert.Contains(t, err.Error(), string(field))
			})
		}
	}
}

func TestArtifactHandler_ValidateRejectsMalformedSteps(t *testing.T) {
	handler := &ArtifactHandler{}
	tests := []struct {
		name   string
		mutate func(*schema.WorkflowStep)
		want   error
	}{
		{"missing action", func(s *schema.WorkflowStep) { s.Action = "" }, errUtils.ErrArtifactStepInvalidAction},
		{"unknown action", func(s *schema.WorkflowStep) { s.Action = "build" }, errUtils.ErrArtifactStepInvalidAction},
		{"pin is not a step action", func(s *schema.WorkflowStep) { s.Action = "pin" }, errUtils.ErrArtifactStepInvalidAction},
		{"source map", func(s *schema.WorkflowStep) { s.Source = map[string]any{"uri": "git::https://example.com/repo.git"} }, errUtils.ErrArtifactStepInvalidSource},
		{"source number", func(s *schema.WorkflowStep) { s.Source = 3 }, errUtils.ErrArtifactStepInvalidSource},
		{"http query mapping", func(s *schema.WorkflowStep) { s.Query = map[string]string{"a": "b"} }, errUtils.ErrArtifactStepInvalidField},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			step := artifactStepWith(artifact.ActionPublish, invoke.FieldSource, invoke.FieldRepository)
			tc.mutate(step)
			require.ErrorIs(t, handler.Validate(step), tc.want)
		})
	}

	t.Run("label shapes", func(t *testing.T) {
		for _, valid := range []any{"team=platform", []any{"a=b", "c=d"}, []string{"a=b"}, map[string]any{"a": "b"}, map[string]string{"a": "b"}} {
			step := artifactStepWith(artifact.ActionList)
			step.Labels = valid
			require.NoError(t, handler.Validate(step), "%T", valid)
		}
		for _, invalid := range []any{3, true, []any{1, 2}} {
			step := artifactStepWith(artifact.ActionList)
			step.Labels = invalid
			require.ErrorIs(t, handler.Validate(step), errUtils.ErrArtifactStepInvalidField, "%T", invalid)
		}
	})
}

func TestArtifactHandler_ValidateThroughTheRegistry(t *testing.T) {
	step := artifactStepWith(artifact.ActionPublish, invoke.FieldSource, invoke.FieldRepository)
	require.NoError(t, ValidateStep(step))

	step.Action = "nope"
	require.ErrorIs(t, ValidateStep(step), errUtils.ErrArtifactStepInvalidAction)
}
