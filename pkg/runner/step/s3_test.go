package step

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/aws/s3upload"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestS3StepSchemaAndDryRun(t *testing.T) {
	t.Parallel()
	var task schema.Task
	require.NoError(t, yaml.Unmarshal([]byte(`name: upload
type: aws/s3
source: artifact.zip
destination: s3://bucket/artifact.zip
region: us-west-2
content_type: application/zip
cache_control: no-cache
 `), &task))
	step := task.ToWorkflowStep()
	roundTrip := schema.TaskFromWorkflowStep(&step)
	assert.Equal(t, task.Region, roundTrip.Region)
	assert.Equal(t, task.ContentType, roundTrip.ContentType)
	assert.Equal(t, task.CacheControl, roundTrip.CacheControl)
	handler := &S3Handler{BaseHandler: NewBaseHandler("aws/s3", CategoryCommand, false), newClient: func(context.Context, string, *Variables) (s3upload.Client, error) {
		t.Fatal("dry-run must not initialize AWS")
		return nil, nil
	}}
	step.DryRun = true
	result, err := handler.Execute(t.Context(), &step, NewVariables())
	require.NoError(t, err)
	assert.True(t, result.Skipped)
	for _, bad := range []schema.WorkflowStep{{Source: "file", Destination: "s3://b/k", Action: "delete"}, {Source: map[string]any{"uri": "x"}, Destination: "s3://b/k"}, {Source: "file"}} {
		assert.Error(t, handler.Validate(&bad))
	}
}

func TestS3StepSDKAndCredentials(t *testing.T) {
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
	handler, ok := Get("aws/s3")
	require.True(t, ok)
	result, err := handler.Execute(t.Context(), &schema.WorkflowStep{Source: "{{ .steps.package.value }}", Destination: "s3://bucket/release%20%231.zip"}, vars)
	require.NoError(t, err)
	assert.Equal(t, "s3://bucket/release%20%231.zip", result.Value)
	assert.Equal(t, "release #1.zip", result.Metadata["key"])
	assert.Equal(t, 1, result.Metadata["uploaded"])
	assert.Equal(t, 1, heads)
	assert.Equal(t, 1, puts)
	assert.Equal(t, "ambient-key", os.Getenv("AWS_ACCESS_KEY_ID"))
}
