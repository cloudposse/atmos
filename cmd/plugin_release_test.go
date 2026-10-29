package cmd

import (
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	plugincatalog "github.com/cloudposse/atmos/pkg/plugin/catalog"
	pluginmanifest "github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"

	containerregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginplatform "github.com/cloudposse/atmos/pkg/plugin/platform"
)

func TestPluginReleaseBuildsTestsAndPublishesSource(t *testing.T) {
	_ = NewTestKit(t)
	repository, err := filepath.Abs("..")
	require.NoError(t, err)
	config := pluginCoverageProject(t)
	directory := filepath.Join(config.BasePath, "plugin")
	_, err = invokePluginCoverage(t, config, "create", directory, "--defaults", "--sdk-path", filepath.Join(repository, "sdk"))
	require.NoError(t, err)
	server := httptest.NewServer(containerregistry.New(containerregistry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	destination := strings.TrimPrefix(server.URL, "http://") + "/plugins/lifecycle"
	progress := capturePluginUI(t)
	output, err := invokePluginCoverage(t, config, "release", directory, "--to", destination, "--format=json")
	require.NoError(t, err)
	var published map[string]any
	require.NoError(t, json.Unmarshal([]byte(output), &published), "stdout remains machine-readable despite compiler and conformance output")
	require.Contains(t, output, "sha256:")
	require.Contains(t, progress.String(), "current host")
	require.FileExists(t, filepath.Join(directory, pluginManifestExecutable(t, directory)))
	require.NoDirExists(t, filepath.Join(directory, ".atmos", "release"), "the CLI owns disposable staging")
	// Test failures must stop the lifecycle before any version is published.
	writePluginFixture(t, directory, wrongStdoutFixture)
	output, err = invokePluginCoverage(t, config, "release", directory, "--to", destination, "--format=json")
	require.ErrorIs(t, err, errUtils.ErrTestsFailed)
	require.Empty(t, output)
}

func TestPluginReleaseLocalPreflight(t *testing.T) {
	_ = NewTestKit(t)
	config := pluginCoverageProject(t)
	for _, args := range [][]string{
		{"--to=example.com/plugin:1.0.0"},
		{"--to=example.com/plugin", "--yes"},
		{"--to=example.com/plugin", "--format=unknown"},
	} {
		output, err := invokePluginCoverage(t, config, append([]string{"release", "absent"}, args...)...)
		require.Error(t, err)
		require.Empty(t, output)
	}
	_, err := invokePluginCoverage(t, nil, "release", "--to=example.com/plugin")
	require.Error(t, err)
	_, err = invokePluginCoverage(t, config, "release", filepath.Join(t.TempDir(), "absent"), "--to=example.com/plugin")
	require.Error(t, err)
	config, directory := pluginNativeProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", ".private.json"), []byte("{}"), 0o600))
	_, err = invokePluginCoverage(t, config, "release", directory, "--to=example.com/plugin")
	require.ErrorIs(t, err, errUtils.ErrPluginBundlePathForbidden)
}

func TestPluginPublishRequiresPublicVisibilityBeforeRegistryWrites(t *testing.T) {
	_ = NewTestKit(t)
	config := pluginCoverageProject(t)
	// A layout is not opened and the destination is never contacted when the
	// requested visibility preflight cannot establish public GHCR visibility.
	output, err := invokePluginCoverage(t, config, "publish", "missing-layout", "--to=registry.example.com/plugin", "--verify-public")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.Contains(t, err.Error(), "visibility preflight requires a GHCR")
	require.Empty(t, output)
}

func TestPluginTestBuildCoverageAndHostGuard(t *testing.T) {
	_ = NewTestKit(t)
	repository, err := filepath.Abs("..")
	require.NoError(t, err)
	config := pluginCoverageProject(t)
	directory := filepath.Join(config.BasePath, "plugin")
	_, err = invokePluginCoverage(t, config, "create", directory, "--defaults", "--sdk-path", filepath.Join(repository, "sdk"))
	require.NoError(t, err)
	executable := filepath.Join(directory, pluginManifestExecutable(t, directory))
	output, err := invokePluginCoverage(t, config, "test", directory, "--build", "--platform=unsupported/amd64", "--coverprofile=coverage.out")
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedPlatform)
	require.Empty(t, output)
	require.NoFileExists(t, executable, "foreign platform rejected before invoking compiler or Go tests")
	diagnostics, err := os.CreateTemp(t.TempDir(), "diagnostics")
	require.NoError(t, err)
	previousStderr := os.Stderr
	os.Stderr = diagnostics
	t.Cleanup(func() { os.Stderr = previousStderr; _ = diagnostics.Close() })
	output, err = invokePluginCoverage(t, config, "test", directory, "--build", "--platform="+pluginplatform.Current(), "--coverprofile=coverage.out", "--format=json")
	require.NoError(t, err)
	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(output), &report), "compiler/unit coverage output must stay off stdout")
	require.Contains(t, report, "results")
	require.FileExists(t, executable)
	profile, err := os.ReadFile(filepath.Join(directory, "coverage.out"))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(profile), "mode: atomic\n"))
	_, err = diagnostics.Seek(0, 0)
	require.NoError(t, err)
	progress, err := io.ReadAll(diagnostics)
	require.NoError(t, err)
	require.Contains(t, string(progress), "total:")
	require.Contains(t, string(progress), "(statements)")
}

func TestPluginReleaseModeConflicts(t *testing.T) {
	_ = NewTestKit(t)
	config := pluginCoverageProject(t)
	for _, args := range [][]string{
		{"source", "--inventory=inventory.yaml"},
		{"--inventory=inventory.yaml", "--to=example.com/plugin"},
		{"--check", "--sign"},
		{"--check", "--verify-public"},
		{"--check", "--catalog=cloudposse"},
		{"--catalog=cloudposse"},
		{"--base=HEAD"},
		{"--head=HEAD~1"},
	} {
		output, err := invokePluginCoverage(t, config, append([]string{"release"}, args...)...)
		require.ErrorIs(t, err, errUtils.ErrPluginFlagConflict)
		require.Empty(t, output)
	}
	output, err := invokePluginCoverage(t, config, "release", "--inventory=missing.yaml", "--check")
	require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
	require.Empty(t, output)
}

func TestPluginReleaseCheckDoesNotContactDestination(t *testing.T) {
	_ = NewTestKit(t)
	repository, err := filepath.Abs("..")
	require.NoError(t, err)
	config := pluginCoverageProject(t)
	directory := filepath.Join(config.BasePath, "check-plugin")
	_, err = invokePluginCoverage(t, config, "create", directory, "--defaults", "--sdk-path", filepath.Join(repository, "sdk"))
	require.NoError(t, err)
	output, err := invokePluginCoverage(t, config, "release", directory, "--check")
	require.NoError(t, err)
	require.Empty(t, output)
	require.FileExists(t, filepath.Join(directory, pluginManifestExecutable(t, directory)))
}

func TestPluginInventoryReleaseUsesRecordedSourcesAndCurrentCatalog(t *testing.T) {
	_ = NewTestKit(t)
	sdk, err := filepath.Abs("../sdk")
	require.NoError(t, err)
	config := pluginCoverageProject(t)
	directory := t.TempDir()
	source := filepath.Join(directory, "packages", "example")
	_, err = invokePluginCoverage(t, config, "create", source, "--defaults", "--kind=team/example", "--sdk-path", sdk)
	require.NoError(t, err)
	server := httptest.NewServer(containerregistry.New(containerregistry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	destination := "oci://" + server.Listener.Addr().String() + "/team/example"
	require.NoError(t, os.WriteFile(filepath.Join(directory, "inventory.yaml"), []byte("packages:\n  example:\n    path: packages/example\n    repository: "+destination+"\n"), 0o600))
	repository, err := git.PlainInit(directory, false)
	require.NoError(t, err)
	worktree, err := repository.Worktree()
	require.NoError(t, err)
	commit := func(message string) string {
		require.NoError(t, worktree.AddWithOptions(&git.AddOptions{All: true}))
		head, commitErr := worktree.Commit(message, &git.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Unix(1, 0)}})
		require.NoError(t, commitErr)
		return head.String()
	}
	t.Setenv("GOWORK", "off")
	require.NoError(t, os.WriteFile(filepath.Join(source, "go.mod"), []byte("module example.com/atmos-oidc\n\ngo 1.25.0\n\nrequire github.com/cloudposse/atmos/sdk v0.0.0\n\nreplace github.com/cloudposse/atmos/sdk => "+filepath.ToSlash(sdk)+"\n"), 0o600))
	oldHead := commit("first release")
	original, err := pluginmanifest.Load(source)
	require.NoError(t, err)
	oldVersion := original.Metadata.Version
	original.Metadata.Version = "2.0.0"
	raw, err := yaml.Marshal(original)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "plugin.yaml"), raw, 0o600))
	commit("new accepted release")
	_, err = repository.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{directory}})
	require.NoError(t, err)
	// Publish the current accepted version before simulating a queued older job.
	_, err = invokePluginCoverage(t, config, "release", source, "--to", destination, "--format=json")
	require.NoError(t, err)
	dirty := []byte("invalid dirty working manifest: preserved\n")
	require.NoError(t, os.WriteFile(filepath.Join(source, "plugin.yaml"), dirty, 0o600))
	t.Chdir(directory)
	progress := capturePluginUI(t)
	output, err := invokePluginCoverage(t, config, "release", "--inventory=inventory.yaml", "--head", oldHead, "--check")
	require.NoError(t, err)
	require.Empty(t, output)
	require.Contains(t, progress.String(), "Checked 1 plugin releases")
	tag, err := name.NewTag(strings.TrimPrefix(destination, "oci://") + ":" + oldVersion)
	require.NoError(t, err)
	_, err = remote.Head(tag)
	require.Error(t, err, "check mode must not publish the older version")
	target := "oci://" + server.Listener.Addr().String() + "/team/catalog:stable"
	output, err = invokePluginCoverage(t, config, "release", "--inventory=inventory.yaml", "--head", oldHead, "--catalog", target, "--format=json")
	require.NoError(t, err)
	var published []pluginoutput.PublishedRelease
	require.NoError(t, json.Unmarshal([]byte(output), &published))
	require.Len(t, published, 1)
	require.Equal(t, oldVersion, published[0].Version)
	require.Equal(t, destination, published[0].Repository)
	require.NotEmpty(t, published[0].Files, "file inventory survives temporary layout cleanup")
	client := atmosoci.NewClient(config)
	file, _, err := plugincatalog.Read(t.Context(), client, target)
	require.NoError(t, err)
	require.Len(t, file.Spec.Entries, 1)
	require.Len(t, file.Spec.Entries[0].Versions, 1)
	require.Equal(t, "2.0.0", file.Spec.Entries[0].Versions[0].Version, "catalog reconciliation uses current origin inventory, not the old job's head")
	actual, err := os.ReadFile(filepath.Join(source, "plugin.yaml"))
	require.NoError(t, err)
	require.Equal(t, dirty, actual)
	// An invalid catalog makes the entire release fail without success outputs.
	config.CI.Enabled = true
	t.Cleanup(ci.SwapRegistryForTest())
	ci.Register(generic.NewProvider())
	t.Setenv("CI", "true")
	t.Setenv("ATMOS_CI_BASE_REF", "")
	outputs := filepath.Join(t.TempDir(), "ci-output")
	t.Setenv("ATMOS_CI_OUTPUT", outputs)
	output, err = invokePluginCoverage(t, config, "release", "--inventory=inventory.yaml", "--head", oldHead, "--catalog=unknown-alias", "--format=json")
	require.Error(t, err)
	require.Empty(t, output)
	require.NoFileExists(t, outputs)
}
