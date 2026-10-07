package step

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPublishStepSchemaAndDryRun(t *testing.T) {
	t.Parallel()
	var task schema.Task
	require.NoError(t, yaml.Unmarshal([]byte(`name: upload
type: publish
source: artifact.zip
target:
  kind: aws/s3
  bucket: bucket
  region: us-west-2
  content_type: application/zip
  cache_control: no-cache
 `), &task))
	step := task.ToWorkflowStep()
	roundTrip := schema.TaskFromWorkflowStep(&step)
	assert.Equal(t, task.Target, roundTrip.Target)
	handler, ok := Get("publish")
	require.True(t, ok)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "artifact.zip"), []byte("artifact"), 0o600))
	step.WorkingDirectory = dir
	step.DryRun = true
	result, err := handler.Execute(t.Context(), &step, NewVariables())
	require.NoError(t, err)
	assert.True(t, result.Skipped)
	for _, bad := range []schema.WorkflowStep{{Source: "file", Destination: "s3://b/k", Action: "delete"}, {Source: map[string]any{"uri": "x"}, Destination: "s3://b/k"}, {Source: "file"}} {
		assert.Error(t, handler.Validate(&bad))
	}
}

func TestPublishStepSDKAndCredentials(t *testing.T) {
	// Exercise the real SDK against HTTP only; no AWS account is contacted.
	t.Setenv("AWS_ACCESS_KEY_ID", "ambient-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	var puts, heads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("Authorization"), "Credential=step-key/")
		if r.Method == http.MethodHead {
			heads++
			w.WriteHeader(http.StatusNotFound)
			return
		}
		puts++
		assert.Equal(t, "/bucket/release #1.zip", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("X-Amz-Meta-Atmos-Sha256"))
		assert.NotEmpty(t, r.Header.Get("X-Amz-Checksum-Sha256"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	vars := NewVariables()
	vars.Env = map[string]string{"AWS_ACCESS_KEY_ID": "step-key", "AWS_SECRET_ACCESS_KEY": "step-secret", "AWS_REGION": "us-east-1", "AWS_ENDPOINT_URL_S3": server.URL}
	source := filepath.Join(t.TempDir(), "artifact.zip")
	require.NoError(t, os.WriteFile(source, []byte("artifact"), 0o600))
	vars.Set("package", NewStepResult(source))
	handler, ok := Get("publish")
	require.True(t, ok)
	result, err := handler.Execute(t.Context(), &schema.WorkflowStep{Source: "{{ .steps.package.value }}", Destination: "release #1.zip", Target: map[string]any{"kind": "aws/s3", "bucket": "bucket", "region": "us-east-1"}}, vars)
	require.NoError(t, err)
	assert.Equal(t, "s3://bucket/release%20%231.zip", result.Value)
	assert.Equal(t, "release #1.zip", result.Metadata["key"])
	assert.Equal(t, 1, result.Metadata["uploaded"])
	assert.Equal(t, 1, heads)
	assert.Equal(t, 1, puts)
	assert.Equal(t, "ambient-key", os.Getenv("AWS_ACCESS_KEY_ID"))
}

func TestPublishDryRunValidation(t *testing.T) {
	t.Parallel()
	handler, ok := Get("publish")
	require.True(t, ok)
	source := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(source, []byte("data"), 0o600))
	for _, tc := range []struct {
		name   string
		target any
		source string
	}{
		{"named target without scope", "missing", source},
		{"missing source", map[string]any{"kind": "aws/s3", "bucket": "b", "region": "us-east-1"}, source + "missing"},
		{"missing region", map[string]any{"kind": "aws/s3", "bucket": "b"}, source},
		{"unsupported kind", map[string]any{"kind": "aws/cloudformation"}, source},
		{"invalid auth", map[string]any{"kind": "aws/s3", "bucket": "b", "region": "us-east-1", "auth": "wrong"}, source},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := handler.Execute(t.Context(), &schema.WorkflowStep{Source: tc.source, Target: tc.target, DryRun: true}, NewVariables())
			require.Error(t, err)
		})
	}
	vars := NewVariables()
	workdir := filepath.Join(t.TempDir(), "must-not-be-created")
	vars.AtmosConfig = &schema.AtmosConfiguration{Git: schema.GitConfig{Repositories: map[string]schema.GitRepository{
		"repo": {URI: "https://invalid.example/repo.git", Workdir: workdir},
	}}}
	result, err := handler.Execute(t.Context(), &schema.WorkflowStep{Source: source, Target: map[string]any{"kind": "git", "repository": "repo", "path": "output"}, DryRun: true}, vars)
	require.NoError(t, err)
	assert.True(t, result.Skipped)
	assert.NoDirExists(t, workdir)
}

func TestPublishTargetRoundTrips(t *testing.T) {
	t.Parallel()
	for _, target := range []any{"assets", map[string]any{"kind": "aws/s3", "bucket": "bucket", "region": "us-east-1"}} {
		task := schema.Task{Type: "publish", Source: "file", Target: target}
		encoded, err := yaml.Marshal(task)
		require.NoError(t, err)
		var decoded schema.Task
		require.NoError(t, yaml.Unmarshal(encoded, &decoded))
		assert.Equal(t, target, decoded.Target)
		workflow := decoded.ToWorkflowStep()
		encodedJSON, err := json.Marshal(workflow)
		require.NoError(t, err)
		var restored schema.WorkflowStep
		require.NoError(t, json.Unmarshal(encodedJSON, &restored))
		assert.Equal(t, target, restored.Target)
		assert.Equal(t, target, schema.TaskFromWorkflowStep(&restored).Target)
	}
}
