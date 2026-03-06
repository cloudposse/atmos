package terraform_backend_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tb "github.com/cloudposse/atmos/internal/terraform_backend"
)

// newFakeGCSState creates a fake GCS server pre-loaded with a terraform state file and returns the server.
func newFakeGCSState(t *testing.T, bucket, prefix, workspace string, stateOutputs map[string]any) *fakestorage.Server {
	t.Helper()

	// Build a terraform state JSON.
	outputs := make(map[string]any)
	for k, v := range stateOutputs {
		outputs[k] = map[string]any{
			"value": v,
			"type":  "string",
		}
	}
	state := map[string]any{
		"version":           4,
		"terraform_version": "1.4.0",
		"serial":            1,
		"lineage":           "test-lineage",
		"outputs":           outputs,
		"resources":         []any{},
		"check_results":     nil,
	}
	stateJSON, err := json.Marshal(state)
	require.NoError(t, err)

	// Build the object key matching Terraform's convention: <prefix>/<workspace>.tfstate.
	var objectKey string
	if prefix == "" {
		objectKey = workspace + ".tfstate"
	} else {
		objectKey = fmt.Sprintf("%s/%s.tfstate", prefix, workspace)
	}

	server := fakestorage.NewServer([]fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: bucket,
				Name:       objectKey,
			},
			Content: stateJSON,
		},
	})
	t.Cleanup(server.Stop)

	return server
}

// TestReadTerraformBackendGCSInternal_FakeServer verifies reading state with prefix from a fake GCS server.
func TestReadTerraformBackendGCSInternal_FakeServer(t *testing.T) {
	server := newFakeGCSState(t, "my-tf-state", "env/dev", "default", map[string]any{
		"vpc_id":    "vpc-abc123",
		"subnet_id": "subnet-def456",
	})

	// Wrap the fake server's client in the GCSClient interface.
	client := tb.NewGCSClientFromStorageClient(server.Client())

	componentSections := map[string]any{
		"workspace": "default",
	}
	backend := map[string]any{
		"bucket": "my-tf-state",
		"prefix": "env/dev",
	}

	content, err := tb.ReadTerraformBackendGCSInternal(client, &componentSections, &backend)
	require.NoError(t, err)
	require.NotNil(t, content)
	assert.Contains(t, string(content), "vpc-abc123")
	assert.Contains(t, string(content), "subnet-def456")
}

// TestReadTerraformBackendGCSInternal_FakeServer_NoPrefix verifies reading state without a prefix.
func TestReadTerraformBackendGCSInternal_FakeServer_NoPrefix(t *testing.T) {
	server := newFakeGCSState(t, "my-tf-state", "", "production", map[string]any{
		"cluster_name": "prod-cluster",
	})

	client := tb.NewGCSClientFromStorageClient(server.Client())

	componentSections := map[string]any{
		"workspace": "production",
	}
	backend := map[string]any{
		"bucket": "my-tf-state",
	}

	content, err := tb.ReadTerraformBackendGCSInternal(client, &componentSections, &backend)
	require.NoError(t, err)
	require.NotNil(t, content)
	assert.Contains(t, string(content), "prod-cluster")
}

// TestReadTerraformBackendGCSInternal_FakeServer_DefaultWorkspace verifies fallback to the default workspace.
func TestReadTerraformBackendGCSInternal_FakeServer_DefaultWorkspace(t *testing.T) {
	// When no workspace is set, Terraform defaults to "default".
	server := newFakeGCSState(t, "state-bucket", "terraform/state", "default", map[string]any{
		"output_key": "output_value",
	})

	client := tb.NewGCSClientFromStorageClient(server.Client())

	componentSections := map[string]any{
		// No workspace set - should default to "default".
	}
	backend := map[string]any{
		"bucket": "state-bucket",
		"prefix": "terraform/state",
	}

	content, err := tb.ReadTerraformBackendGCSInternal(client, &componentSections, &backend)
	require.NoError(t, err)
	require.NotNil(t, content)
	assert.Contains(t, string(content), "output_value")
}

// TestReadTerraformBackendGCSInternal_FakeServer_NotFound verifies that a missing state file returns nil without error.
func TestReadTerraformBackendGCSInternal_FakeServer_NotFound(t *testing.T) {
	// Create a server with no objects — state file doesn't exist.
	server := fakestorage.NewServer(nil)
	server.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: "empty-bucket"})
	t.Cleanup(server.Stop)

	client := tb.NewGCSClientFromStorageClient(server.Client())

	componentSections := map[string]any{
		"workspace": "default",
	}
	backend := map[string]any{
		"bucket": "empty-bucket",
		"prefix": "terraform/state",
	}

	// When the state file doesn't exist, it should return nil, nil (not an error).
	content, err := tb.ReadTerraformBackendGCSInternal(client, &componentSections, &backend)
	assert.NoError(t, err)
	assert.Nil(t, content)
}

// TestReadTerraformBackendGCSInternal_FakeServer_ValidJSON verifies the returned content is valid Terraform state JSON.
func TestReadTerraformBackendGCSInternal_FakeServer_ValidJSON(t *testing.T) {
	// Verify the returned content is valid JSON matching Terraform state format.
	server := newFakeGCSState(t, "json-bucket", "state", "default", map[string]any{
		"db_host": "db.example.com",
	})

	client := tb.NewGCSClientFromStorageClient(server.Client())

	componentSections := map[string]any{
		"workspace": "default",
	}
	backend := map[string]any{
		"bucket": "json-bucket",
		"prefix": "state",
	}

	content, err := tb.ReadTerraformBackendGCSInternal(client, &componentSections, &backend)
	require.NoError(t, err)

	// Parse as JSON to verify valid terraform state.
	var state map[string]any
	err = json.Unmarshal(content, &state)
	require.NoError(t, err)
	assert.Equal(t, float64(4), state["version"])
	assert.Equal(t, "1.4.0", state["terraform_version"])

	outputs, ok := state["outputs"].(map[string]any)
	require.True(t, ok)
	dbHost, ok := outputs["db_host"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "db.example.com", dbHost["value"])
}
