package ci

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.PlanfilesConfig{Default: "", Priority: nil, Stores: nil}

// TestCreatePlanfileStore_ConfiguredSelection verifies that native CI reads from
// the configured destination even when GitHub Actions auto-detection is active.
func TestCreatePlanfileStore_ConfiguredSelection(t *testing.T) {
	for _, tc := range []struct {
		name         string
		defaultStore string
		priority     []string
		wantStore    string
		wantError    error
	}{
		{name: "priority overrides environment", priority: []string{"primary"}, wantStore: "primary"},
		{name: "priority preserves order", priority: []string{"secondary", "primary"}, wantStore: "secondary"},
		{name: "unconfigured priority entry is skipped", priority: []string{"missing", "primary"}, wantStore: "primary"},
		{name: "unavailable remote stores are skipped", priority: []string{"github", "s3", "primary"}, wantStore: "primary"},
		{name: "default overrides priority", defaultStore: "secondary", priority: []string{"primary"}, wantStore: "secondary"},
		{name: "missing default fails", defaultStore: "missing", priority: []string{"primary"}, wantError: errUtils.ErrArtifactStoreNotFound},
		{name: "exhausted priority fails", priority: []string{"missing"}, wantError: errUtils.ErrArtifactStoreNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_ACTIONS", "true")
			t.Setenv("ATMOS_PLANFILE_BUCKET", "")
			for _, name := range []string{"ATMOS_CI_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_ROLE_ARN", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI"} {
				t.Setenv(name, "")
			}
			t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
			t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "config"))
			t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))
			config := &schema.AtmosConfiguration{}
			config.Components.Terraform.Planfiles = schema.PlanfilesConfig{
				Default: tc.defaultStore, Priority: tc.priority,
				Stores: seededPlanfileStores(t),
			}
			config.Components.Terraform.Planfiles.Stores["github"] = schema.PlanfileStoreSpec{Type: "github/artifacts"}
			config.Components.Terraform.Planfiles.Stores["s3"] = schema.PlanfileStoreSpec{Type: "aws/s3", Options: map[string]any{"bucket": "unused", "region": "us-east-1"}}

			selected, err := createPlanfileStore(ExecuteOptions{AtmosConfig: config})
			if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, selected)
			for _, name := range []string{"primary", "secondary"} {
				exists, err := selected.Exists(context.Background(), name)
				require.NoError(t, err)
				assert.Equal(t, name == tc.wantStore, exists, "artifact from %s", name)
			}
		})
	}
}

func seededPlanfileStores(t *testing.T) map[string]schema.PlanfileStoreSpec {
	t.Helper()
	stores := make(map[string]schema.PlanfileStoreSpec)
	for _, name := range []string{"primary", "secondary"} {
		spec := schema.PlanfileStoreSpec{
			Type: "local/dir", Options: map[string]any{"path": filepath.Join(t.TempDir(), name)},
		}
		backend, err := artifact.NewStore(artifact.StoreOptions{Type: spec.Type, Options: spec.Options})
		require.NoError(t, err)
		require.NoError(t, backend.Upload(context.Background(), name, []artifact.FileEntry{
			{Name: "plan.tfplan", Data: strings.NewReader(name), Size: int64(len(name))},
		}, nil))
		stores[name] = spec
	}
	return stores
}

func TestPlanfileCredentialAvailability(t *testing.T) {
	t.Run("GitHub token", func(t *testing.T) {
		t.Setenv("ATMOS_CI_GITHUB_TOKEN", "fixture-token")
		checker := planfileEnvironmentChecker{options: artifact.StoreOptions{Type: "github/artifacts"}}
		assert.True(t, checker.IsAvailable(t.Context()))
	})
	t.Run("AWS default credential chain", func(t *testing.T) {
		t.Setenv("AWS_ACCESS_KEY_ID", "fixture-access-key")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "fixture-secret-key")
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_DEFAULT_PROFILE", "")
		t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "config"))
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))
		checker := planfileEnvironmentChecker{options: artifact.StoreOptions{Type: "aws/s3"}}
		assert.True(t, checker.IsAvailable(t.Context()))
	})
	t.Run("Atmos identity does not require ambient AWS credentials", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "not-an-ambient-profile")
		checker := planfileEnvironmentChecker{options: artifact.StoreOptions{Type: "aws/s3", Identity: "deployment"}}
		assert.True(t, checker.IsAvailable(t.Context()))
	})
}
