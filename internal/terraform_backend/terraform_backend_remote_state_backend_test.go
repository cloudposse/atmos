package terraform_backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels: fail the build immediately if these section-name constants are
// renamed, since the remote-state backend resolution depends on their exact values.
var (
	_ = cfg.RemoteStateBackendSectionName
	_ = cfg.RemoteStateBackendTypeSectionName
	_ = cfg.BackendSectionName
	_ = cfg.BackendTypeSectionName
)

func TestGetComponentRemoteStateBackend(t *testing.T) {
	remoteBackend := map[string]any{"bucket": "ro-bucket"}
	sections := map[string]any{cfg.RemoteStateBackendSectionName: remoteBackend}
	assert.Equal(t, remoteBackend, GetComponentRemoteStateBackend(&sections))

	// Missing section returns nil.
	empty := map[string]any{}
	assert.Nil(t, GetComponentRemoteStateBackend(&empty))

	// Wrong type returns nil.
	wrongType := map[string]any{cfg.RemoteStateBackendSectionName: "not-a-map"}
	assert.Nil(t, GetComponentRemoteStateBackend(&wrongType))
}

func TestGetComponentRemoteStateBackendType(t *testing.T) {
	sections := map[string]any{cfg.RemoteStateBackendTypeSectionName: "s3"}
	assert.Equal(t, "s3", GetComponentRemoteStateBackendType(&sections))

	empty := map[string]any{}
	assert.Equal(t, "", GetComponentRemoteStateBackendType(&empty))

	wrongType := map[string]any{cfg.RemoteStateBackendTypeSectionName: 123}
	assert.Equal(t, "", GetComponentRemoteStateBackendType(&wrongType))
}

func TestResolveRemoteStateBackendSections(t *testing.T) {
	t.Run("nil sections returns nil", func(t *testing.T) {
		assert.Nil(t, resolveRemoteStateBackendSections(nil))
	})

	t.Run("no remote state backend returns the same sections unchanged", func(t *testing.T) {
		sections := map[string]any{
			cfg.BackendTypeSectionName: "s3",
			cfg.BackendSectionName:     map[string]any{"bucket": "primary"},
		}
		got := resolveRemoteStateBackendSections(&sections)
		// Identical pointer: no copy made when there is nothing to override.
		assert.Same(t, &sections, got)
	})

	t.Run("remote state backend overrides backend and backend_type", func(t *testing.T) {
		primaryBackend := map[string]any{
			"bucket":      "state-bucket",
			"key":         "terraform.tfstate",
			"region":      "us-west-2",
			"assume_role": map[string]any{"role_arn": "arn:aws:iam::123456789012:role/example-tfstate-write"},
		}
		remoteBackend := map[string]any{
			"bucket":      "state-bucket",
			"key":         "terraform.tfstate",
			"region":      "us-west-2",
			"assume_role": map[string]any{"role_arn": "arn:aws:iam::123456789012:role/example-tfstate-ro"},
		}
		sections := map[string]any{
			cfg.BackendTypeSectionName:            "s3",
			cfg.BackendSectionName:                primaryBackend,
			cfg.RemoteStateBackendTypeSectionName: "s3",
			cfg.RemoteStateBackendSectionName:     remoteBackend,
			cfg.WorkspaceSectionName:              "default",
			cfg.EnvSectionName:                    map[string]any{"AWS_REGION": "us-west-2"},
		}

		got := resolveRemoteStateBackendSections(&sections)
		require.NotNil(t, got)

		// A fresh map is returned; the original is never mutated.
		assert.NotSame(t, &sections, got)
		assert.Equal(t, primaryBackend, sections[cfg.BackendSectionName], "original backend section must be untouched")

		// The effective backend used for reads is the remote-state (read-only) backend.
		assert.Equal(t, remoteBackend, GetComponentBackend(got))
		assert.Equal(t, "s3", GetComponentBackendType(got))

		// The assume-role ARN resolved from the effective backend is the read-only role.
		assert.Equal(t,
			"arn:aws:iam::123456789012:role/example-tfstate-ro",
			GetS3BackendAssumeRoleArn(&remoteBackend),
		)

		// Unrelated sections (workspace, env) are carried through unchanged.
		assert.Equal(t, "default", GetTerraformWorkspace(got))
		assert.Equal(t, map[string]any{"AWS_REGION": "us-west-2"}, (*got)[cfg.EnvSectionName])
	})

	t.Run("remote_state_backend_type can differ from backend_type", func(t *testing.T) {
		sections := map[string]any{
			cfg.BackendTypeSectionName:            "s3",
			cfg.BackendSectionName:                map[string]any{"bucket": "primary"},
			cfg.RemoteStateBackendTypeSectionName: "local",
			cfg.RemoteStateBackendSectionName:     map[string]any{"path": "/tmp/remote.tfstate"},
		}
		got := resolveRemoteStateBackendSections(&sections)
		require.NotNil(t, got)
		assert.Equal(t, "local", GetComponentBackendType(got))
		assert.Equal(t, map[string]any{"path": "/tmp/remote.tfstate"}, GetComponentBackend(got))
		// Original backend_type is preserved on the source map.
		assert.Equal(t, "s3", sections[cfg.BackendTypeSectionName])
	})

	t.Run("only remote_state_backend_type set still copies and overrides the type", func(t *testing.T) {
		sections := map[string]any{
			cfg.BackendTypeSectionName:            "s3",
			cfg.BackendSectionName:                map[string]any{"bucket": "primary"},
			cfg.RemoteStateBackendTypeSectionName: "local",
		}
		got := resolveRemoteStateBackendSections(&sections)
		require.NotNil(t, got)
		assert.NotSame(t, &sections, got)
		assert.Equal(t, "local", GetComponentBackendType(got))
		// With no remote_state_backend section, the primary backend section is reused.
		assert.Equal(t, map[string]any{"bucket": "primary"}, GetComponentBackend(got))
	})

	t.Run("empty remote_state_backend map with no type is a no-op", func(t *testing.T) {
		sections := map[string]any{
			cfg.BackendTypeSectionName:        "s3",
			cfg.BackendSectionName:            map[string]any{"bucket": "primary"},
			cfg.RemoteStateBackendSectionName: map[string]any{},
		}
		got := resolveRemoteStateBackendSections(&sections)
		assert.Same(t, &sections, got)
	})
}

// TestGetTerraformBackend_PrefersRemoteStateBackend verifies end-to-end (through the public
// GetTerraformBackend entry point and the real local backend reader) that state is read from
// the component's remote_state_backend, not its primary backend. See issue #3446.
func TestGetTerraformBackend_PrefersRemoteStateBackend(t *testing.T) {
	tmpDir := t.TempDir()

	writePath := filepath.Join(tmpDir, "write.tfstate")
	readPath := filepath.Join(tmpDir, "read.tfstate")

	writeState := `{
		"version": 4,
		"terraform_version": "1.3.0",
		"outputs": {"value": {"value": "from-write-backend", "type": "string"}}
	}`
	readState := `{
		"version": 4,
		"terraform_version": "1.3.0",
		"outputs": {"value": {"value": "from-remote-state-backend", "type": "string"}}
	}`
	require.NoError(t, os.WriteFile(writePath, []byte(writeState), 0o644))
	require.NoError(t, os.WriteFile(readPath, []byte(readState), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		TerraformDirAbsolutePath: filepath.Join(tmpDir, "terraform"),
	}

	t.Run("reads from remote_state_backend when configured", func(t *testing.T) {
		sections := map[string]any{
			cfg.ComponentSectionName:              "vpc",
			cfg.WorkspaceSectionName:              "default",
			cfg.BackendTypeSectionName:            "local",
			cfg.BackendSectionName:                map[string]any{"path": writePath},
			cfg.RemoteStateBackendTypeSectionName: "local",
			cfg.RemoteStateBackendSectionName:     map[string]any{"path": readPath},
		}
		outputs, err := GetTerraformBackend(atmosConfig, &sections, nil)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"value": "from-remote-state-backend"}, outputs)
	})

	t.Run("falls back to backend when no remote_state_backend is configured", func(t *testing.T) {
		sections := map[string]any{
			cfg.ComponentSectionName:   "vpc",
			cfg.WorkspaceSectionName:   "default",
			cfg.BackendTypeSectionName: "local",
			cfg.BackendSectionName:     map[string]any{"path": writePath},
		}
		outputs, err := GetTerraformBackend(atmosConfig, &sections, nil)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"value": "from-write-backend"}, outputs)
	})
}
