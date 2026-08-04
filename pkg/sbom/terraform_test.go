package sbom

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// --- appendTerraform -------------------------------------------------------

func TestAppendTerraformRequiresConfig(t *testing.T) {
	t.Parallel()
	err := appendTerraform(t.Context(), &Graph{}, nil)
	require.ErrorIs(t, err, errTerraformConfigurationRequired)
}

func TestAppendTerraformPropagatesProviderLockParseError(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	componentDir := filepath.Join(base, "components", "terraform", "broken")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, ".terraform.lock.hcl"), []byte("not { valid hcl"), 0o644))

	config := &schema.AtmosConfiguration{BasePath: base}
	config.Components.Terraform.BasePath = filepath.Join("components", "terraform")

	err := appendTerraform(t.Context(), &Graph{}, config)
	require.Error(t, err)
}

func TestAppendTerraformSkipsWhenBaseDirectoryMissing(t *testing.T) {
	t.Parallel()
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	config.Components.Terraform.BasePath = filepath.Join("components", "does-not-exist")

	graph := &Graph{}
	require.NoError(t, appendTerraform(t.Context(), graph, config))
	require.Empty(t, graph.Components)
	for _, coverage := range graph.Coverage {
		require.Contains(t, []string{"no Terraform lock files discovered", "no initialized Terraform configurations discovered", "no OCI artifacts found in the vendor lock"}, coverage.Detail)
	}
}

// TestOCIArtifactsDetailReflectsActualPresence proves the oci-artifacts coverage detail is honest
// about whether any OCI-sourced vendor artifact actually exists, rather than unconditionally
// claiming OCI artifacts were represented -- confirmed by manual QA to previously always report
// "OCI artifacts are represented by Atmos source receipts" even in a project with zero OCI
// artifacts of any kind.
func TestOCIArtifactsDetailReflectsActualPresence(t *testing.T) {
	t.Parallel()

	t.Run("no OCI artifacts in the graph", func(t *testing.T) {
		t.Parallel()
		graph := &Graph{Components: []Component{
			{ID: "vendor:a", Properties: map[string]string{"atmos:domain": "vendor", "atmos:kind": "remote"}},
		}}
		require.Equal(t, "no OCI artifacts found in the vendor lock", ociArtifactsDetail(graph))
	})

	t.Run("one OCI artifact in the graph", func(t *testing.T) {
		t.Parallel()
		graph := &Graph{Components: []Component{
			{ID: "vendor:a", Properties: map[string]string{"atmos:domain": "vendor", "atmos:kind": "remote"}},
			{ID: "vendor:b", Properties: map[string]string{"atmos:domain": "vendor", "atmos:kind": "oci"}},
		}}
		require.Equal(t, "1 OCI artifact(s) represented by Atmos source receipts", ociArtifactsDetail(graph))
	})

	t.Run("ignores non-vendor components with a coincidentally matching kind", func(t *testing.T) {
		t.Parallel()
		graph := &Graph{Components: []Component{
			{ID: "terraform-provider:x", Properties: map[string]string{"atmos:domain": "terraform-provider", "atmos:kind": "oci"}},
		}}
		require.Equal(t, "no OCI artifacts found in the vendor lock", ociArtifactsDetail(graph))
	})
}

func TestAppendTerraformMarksModulesIncompleteWhenLocalModuleUnresolvable(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	componentDir := filepath.Join(base, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, ".terraform.lock.hcl"), []byte(`provider "registry.terraform.io/hashicorp/aws" {
  version = "5.95.0"
  hashes = ["zh:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]
}`), 0o644))

	previous := runTerraformModules
	runTerraformModules = func(context.Context, string, string) ([]byte, error) {
		// "skip" and "" entries exercise the empty key/source guard; "missing"
		// points at a local module directory that was never materialized, so
		// TreeSHA256 fails and the module is reported as incomplete.
		return []byte(`{"format_version":"1.0","modules":[{"key":"","source":"./ignored"},{"key":"skip","source":""},{"key":"missing","source":"./modules/missing"}]}`), nil
	}
	t.Cleanup(func() { runTerraformModules = previous })

	config := &schema.AtmosConfiguration{BasePath: base}
	config.Components.Terraform.BasePath = filepath.Join("components", "terraform")

	graph := &Graph{}
	require.NoError(t, appendTerraform(t.Context(), graph, config))

	require.Contains(t, componentIDs(graph), "terraform-module:terraform:vpc:missing")
	for _, coverage := range graph.Coverage {
		if coverage.Adapter == "terraform-modules" {
			require.Equal(t, "incomplete", coverage.Status)
			require.Contains(t, coverage.Detail, "lack immutable resolution evidence")
		}
	}
	for _, component := range graph.Components {
		if component.ID == "terraform-module:terraform:vpc:missing" {
			require.Equal(t, "NOASSERTION", component.Version)
		}
	}
}

// --- linkConfigurationSource -------------------------------------------------

func TestLinkConfigurationSourceSkipsNonMatchingDomains(t *testing.T) {
	t.Parallel()
	graph := &Graph{Components: []Component{
		{ID: "vendor:vpc", Properties: map[string]string{"atmos:target": "components/terraform/vpc", "atmos:domain": "vendor"}},
		{ID: "toolchain:terraform:darwin_arm64", Properties: map[string]string{"atmos:target": "components/terraform/vpc", "atmos:domain": "toolchain"}},
		{ID: "vendor:other", Properties: map[string]string{"atmos:target": "components/terraform/other", "atmos:domain": "vendor"}},
	}}

	linkConfigurationSource(graph, "terraform:vpc", "components/terraform/vpc")

	require.Equal(t, []Relationship{{From: "terraform:vpc", To: "vendor:vpc", Type: "depends_on"}}, graph.Relationships)
}

// --- appendModulesForDirectory -----------------------------------------------

func TestAppendModulesForDirectoryHandlesRunTerraformModulesErrors(t *testing.T) {
	dir := t.TempDir()
	config := &schema.AtmosConfiguration{}
	tests := []struct {
		name       string
		run        func(context.Context, string, string) ([]byte, error)
		wantDetail string
	}{
		{
			name:       "terraform binary not found",
			run:        func(context.Context, string, string) ([]byte, error) { return nil, os.ErrNotExist },
			wantDetail: "Terraform >= 1.10 with modules -json is unavailable",
		},
		{
			name:       "generic execution error",
			run:        func(context.Context, string, string) ([]byte, error) { return nil, errors.New("exit status 1") },
			wantDetail: "modules -json unavailable for",
		},
		{
			name:       "invalid JSON output",
			run:        func(context.Context, string, string) ([]byte, error) { return []byte("not json"), nil },
			wantDetail: "invalid modules -json output for",
		},
		{
			name:       "missing format_version",
			run:        func(context.Context, string, string) ([]byte, error) { return []byte(`{"modules":[]}`), nil },
			wantDetail: "modules -json did not return format_version for",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := runTerraformModules
			runTerraformModules = tt.run
			t.Cleanup(func() { runTerraformModules = previous })

			complete, detail := appendModulesForDirectory(t.Context(), &Graph{}, config, "terraform:vpc", dir)
			require.False(t, complete)
			require.Contains(t, detail, tt.wantDetail)
		})
	}
}

// --- resolveModuleArtifact -----------------------------------------------------

func TestResolveModuleArtifactResolvesDigestReferenceWithoutNetwork(t *testing.T) {
	t.Parallel()
	config := &schema.AtmosConfiguration{}
	artifact, err := resolveModuleArtifact(context.Background(), config, t.TempDir(), "registry.example.com/org/module@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	require.Equal(t, "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", artifact.Identity)
}

// --- findProviderLocks ---------------------------------------------------------

func TestFindProviderLocksReturnsNilForMissingBase(t *testing.T) {
	t.Parallel()
	locks, err := findProviderLocks(filepath.Join(t.TempDir(), "does-not-exist"))
	require.NoError(t, err)
	require.Nil(t, locks)
}

// TestFindProviderLocksPropagatesPermissionErrors exercises the two
// non-"not exist" os.Stat/filepath.WalkDir error branches, which require an
// unreadable directory. Unix permission bits don't apply the same way on
// Windows, and a test running as root ignores them entirely, so this is
// skipped there rather than producing a flaky, environment-dependent result.
func TestFindProviderLocksPropagatesPermissionErrors(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}

	t.Run("base itself is unreadable", func(t *testing.T) {
		t.Parallel()
		parent := t.TempDir()
		restricted := filepath.Join(parent, "restricted")
		require.NoError(t, os.Mkdir(restricted, 0o000))
		t.Cleanup(func() { _ = os.Chmod(restricted, 0o755) })

		_, err := findProviderLocks(filepath.Join(restricted, "nested"))
		require.Error(t, err)
	})

	t.Run("subdirectory is unreadable during walk", func(t *testing.T) {
		t.Parallel()
		base := t.TempDir()
		blocked := filepath.Join(base, "blocked")
		require.NoError(t, os.Mkdir(blocked, 0o000))
		t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

		_, err := findProviderLocks(base)
		require.Error(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		config.Components.Terraform.BasePath = "."
		err = appendTerraform(t.Context(), &Graph{}, config)
		require.Error(t, err)
	})
}

// --- relativeID / coverageStatus ------------------------------------------------

func TestRelativeIDReturnsRootForSamePath(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	require.Equal(t, "root", relativeID(base, base))
	require.Equal(t, "vpc", relativeID(base, filepath.Join(base, "vpc")))
}

func TestCoverageStatus(t *testing.T) {
	t.Parallel()
	require.Equal(t, "complete", coverageStatus(true))
	require.Equal(t, "incomplete", coverageStatus(false))
}
