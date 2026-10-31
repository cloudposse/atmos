package schema

import (
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const publishStepYAML = `
name: publish-vpc
type: artifact
action: publish
source: ./components/terraform/vpc
repository: shared
artifact_name: components/terraform/vpc
artifact_kind: component/terraform
version: "1.2.0"
force: true
dry_run: true
readme: README.md
changelog: CHANGELOG.md
output: log
metadata:
  labels:
    team: platform
    env: dev
`

// publishStep is the step the YAML above describes.
func publishStep() WorkflowStep {
	return WorkflowStep{
		Name: "publish-vpc", Type: TaskTypeArtifact, Action: "publish", Source: "./components/terraform/vpc",
		Repository: "shared", ArtifactName: "components/terraform/vpc", ArtifactKind: "component/terraform", Version: "1.2.0",
		Force: true, ArtifactDryRun: true, Readme: "README.md", Changelog: "CHANGELOG.md", Output: "log",
		Metadata: &ArtifactStepMetadata{Labels: map[string]string{"team": "platform", "env": "dev"}},
	}
}

func TestWorkflowStep_UnmarshalYAML_ArtifactFields(t *testing.T) {
	var step WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte(publishStepYAML), &step))
	assert.Equal(t, publishStep(), step)
	assert.Equal(t, "publish-vpc", step.Name, "the step name keeps its runner meaning")
	assert.Equal(t, "components/terraform/vpc", step.ArtifactName, "the artifact name is a separate field")
	assert.Equal(t, "log", step.Output, "output keeps its display-mode meaning")
	assert.Nil(t, step.Artifacts, "the runner's expected-output declaration is untouched")
}

func TestWorkflowStep_UnmarshalYAML_ArtifactStepOtherActions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(*testing.T, WorkflowStep)
	}{
		{"pull", "{type: artifact, action: pull, ref: 'team/app:1', repository: dir, destination: ./out}", func(t *testing.T, s WorkflowStep) {
			assert.Equal(t, "team/app:1", s.Ref)
			assert.Equal(t, "./out", s.Destination)
		}},
		{"copy", "{type: artifact, action: copy, ref: 'team/app:1', from: dir, to: archive, force: true}", func(t *testing.T, s WorkflowStep) {
			assert.Equal(t, "dir", s.From)
			assert.Equal(t, "archive", s.To)
		}},
		{"show", "{type: artifact, action: show, document: changelog, ref: 'team/app:1', repository: dir}", func(t *testing.T, s WorkflowStep) {
			assert.Equal(t, "changelog", s.Document)
		}},
		{"search scalar query", "{type: artifact, action: search, query: vpc, refresh: true}", func(t *testing.T, s WorkflowStep) {
			assert.Equal(t, "vpc", s.SearchQuery)
			assert.Nil(t, s.Query, "the http query mapping stays empty")
			assert.True(t, s.Refresh)
		}},
		{"list string labels", "{type: artifact, action: list, labels: 'team=platform,env=dev'}", func(t *testing.T, s WorkflowStep) {
			assert.Equal(t, "team=platform,env=dev", s.Labels)
		}},
		{"list sequence labels", "{type: artifact, action: list, labels: [team=platform, env=dev]}", func(t *testing.T, s WorkflowStep) {
			assert.Equal(t, []any{"team=platform", "env=dev"}, s.Labels)
		}},
		{"list mapping labels", "{type: artifact, action: list, labels: {team: platform}}", func(t *testing.T, s WorkflowStep) {
			assert.Equal(t, map[string]any{"team": "platform"}, s.Labels)
		}},
		{"http mapping query still decodes", "{type: http, url: 'https://example.com', query: {page: '2'}}", func(t *testing.T, s WorkflowStep) {
			assert.Equal(t, map[string]string{"page": "2"}, s.Query)
			assert.Empty(t, s.SearchQuery)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var step WorkflowStep
			require.NoError(t, yaml.Unmarshal([]byte(tc.input), &step))
			tc.check(t, step)
		})
	}

	t.Run("a sequence query is rejected", func(t *testing.T) {
		var step WorkflowStep
		require.ErrorIs(t, yaml.Unmarshal([]byte("{type: artifact, query: [a, b]}"), &step), ErrWorkflowControlStepInvalid)
	})
}

func TestTask_ConversionsCarryEveryArtifactField(t *testing.T) {
	step := publishStep()
	step.Labels = "a=b"
	step.Ref, step.From, step.To, step.Document, step.SearchQuery, step.Refresh = "team/app:1", "dir", "archive", "readme", "vpc", true
	step.Destination = "./out"

	task := TaskFromWorkflowStep(&step)
	assert.Equal(t, step.Repository, task.Repository)
	assert.Equal(t, step.ArtifactName, task.ArtifactName)
	assert.Equal(t, step.ArtifactKind, task.ArtifactKind)
	assert.Equal(t, step.Metadata, task.Metadata)
	assert.Equal(t, step.SearchQuery, task.SearchQuery)

	assert.Equal(t, step, task.ToWorkflowStep(), "WorkflowStep -> Task -> WorkflowStep loses nothing")
	roundTrip := TaskFromWorkflowStep(&step)
	assert.Equal(t, task, roundTrip)
}

func TestTasks_DecodeCustomCommandArtifactSteps(t *testing.T) {
	type config struct {
		Steps Tasks `mapstructure:"steps"`
	}
	decode := func(t *testing.T, steps ...any) Tasks {
		t.Helper()
		var result config
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: &result, WeaklyTypedInput: true, DecodeHook: TasksDecodeHook()})
		require.NoError(t, err)
		require.NoError(t, decoder.Decode(map[string]any{"steps": steps}))
		return result.Steps
	}

	t.Run("publish with metadata labels", func(t *testing.T) {
		tasks := decode(t, map[string]any{
			"type": "artifact", "action": "publish", "source": "./dist", "repository": "shared", "artifact_name": "team/app",
			"artifact_kind": "files", "version": "1", "force": true, "dry_run": true, "readme": "R.md", "changelog": "C.md",
			"metadata": map[string]any{"labels": map[string]any{"team": "platform"}},
		})
		require.Len(t, tasks, 1)
		require.NoError(t, tasks[0].LoadError)
		step := tasks[0].ToWorkflowStep()
		assert.Equal(t, "./dist", step.Source)
		assert.Equal(t, "shared", step.Repository)
		assert.Equal(t, "team/app", step.ArtifactName)
		assert.True(t, step.ArtifactDryRun)
		assert.Equal(t, map[string]string{"team": "platform"}, step.Metadata.Labels)
	})
	t.Run("search text in a scalar query", func(t *testing.T) {
		tasks := decode(t, map[string]any{"type": "artifact", "action": "search", "query": "vpc", "labels": []any{"a=b"}})
		require.NoError(t, tasks[0].LoadError)
		assert.Equal(t, "vpc", tasks[0].SearchQuery)
		assert.Nil(t, tasks[0].Query)
		assert.Equal(t, []any{"a=b"}, tasks[0].Labels)
	})
	t.Run("http query mapping is not disturbed", func(t *testing.T) {
		tasks := decode(t, map[string]any{"type": "http", "url": "https://example.com", "query": map[string]any{"page": "2"}})
		require.NoError(t, tasks[0].LoadError)
		assert.Equal(t, map[string]string{"page": "2"}, tasks[0].Query)
		assert.Empty(t, tasks[0].SearchQuery)
	})
	t.Run("an unknown artifact key still fails the step, not the load", func(t *testing.T) {
		tasks := decode(t, map[string]any{"type": "artifact", "action": "pull", "repositry": "dir"})
		require.Error(t, tasks[0].LoadError)
	})
}

func TestNormalizeSearchQueryMapDoesNotMutateItsInput(t *testing.T) {
	input := map[string]any{"query": "vpc", "type": "artifact"}
	normalized := normalizeSearchQueryMap(input)
	assert.Equal(t, map[string]any{"search_query": "vpc", "type": "artifact"}, normalized)
	assert.Equal(t, map[string]any{"query": "vpc", "type": "artifact"}, input)

	mapping := map[string]any{"query": map[string]any{"a": "b"}}
	assert.Equal(t, mapping, normalizeSearchQueryMap(mapping))
	assert.Equal(t, map[string]any{"type": "x"}, normalizeSearchQueryMap(map[string]any{"type": "x"}))
	assert.Equal(t, map[string]any{"query": nil}, normalizeSearchQueryMap(map[string]any{"query": nil}))
}
