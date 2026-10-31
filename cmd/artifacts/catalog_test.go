package artifacts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/catalog"
	"github.com/cloudposse/atmos/pkg/schema"
)

const inventoryYAML = `apiVersion: atmos/v1
kind: AtmosArtifactInventory
metadata:
  name: releases
spec:
  refs:
    - components/terraform/vpc:1.2.0
    - tools/helper:0.1.0
`

// catalogHarness runs the catalog commands against a mock maintainer.
func catalogHarness(t *testing.T) (*harness, *catalog.MockMaintainer) {
	t.Helper()
	h, _ := newHarness(t)
	for _, name := range []string{envInput, envExpectedDigest} {
		t.Setenv(name, "")
	}
	maintainer := catalog.NewMockMaintainer(gomock.NewController(t))
	previous := newCatalogs
	newCatalogs = func(*schema.AtmosConfiguration) (catalog.Maintainer, error) { return maintainer, nil }
	t.Cleanup(func() { newCatalogs = previous })
	return h, maintainer
}

func writeInventory(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inventory.yaml")
	require.NoError(t, os.WriteFile(path, []byte(inventoryYAML), 0o600))
	return path
}

func sampleResult() *catalog.Result {
	return &catalog.Result{
		Repository: "dir", Catalog: "oci://ghcr.io/acme/atmos/catalog:stable", Digest: "sha256:" + string(manifestDigest)[7:],
		PreviousDigest: string(recordDigest), Revision: 3, Listed: 4, Added: []string{"a:1"}, Updated: []string{"b:2"}, Removed: []string{"c:3"}, LockScope: artifact.LockScopeHost,
	}
}

func TestCatalogBuildResolvesTheInventoryAndWritesTheSnapshot(t *testing.T) {
	h, maintainer := catalogHarness(t)
	output := filepath.Join(t.TempDir(), "catalog.json")
	file := catalog.New("catalog")
	file.Metadata.Revision = 2
	file.Spec.Entries = []catalog.Entry{{
		Name: "components/terraform/vpc", Kind: "files", Source: "oci://ghcr.io/acme/atmos/components/terraform/vpc",
		Versions: []catalog.Version{{Version: "1.2.0", ResolvedDigest: string(manifestDigest), RecordDigest: string(recordDigest), ContentDigest: string(contentDigest)}},
	}}
	maintainer.EXPECT().Build(gomock.Any(), &catalog.BuildRequest{
		Repository: "dir", Name: "custom",
		Refs: []artifact.Ref{{Name: "components/terraform/vpc", Version: "1.2.0"}, {Name: "tools/helper", Version: "0.1.0"}},
	}).Return(&catalog.BuildResult{File: file, BaseDigest: string(recordDigest)}, nil)

	got := h.run("catalog", "build", "--repository=dir", "--input="+writeInventory(t), "--output="+output, "--name=custom")
	require.NoError(t, got.err)
	assert.Contains(t, got.stdout, "output:")
	assert.Contains(t, got.stdout, string(recordDigest))
	assert.Regexp(t, `revision:\s+2`, got.stdout)
	written, err := catalog.ReadFile(output)
	require.NoError(t, err)
	assert.Equal(t, file.Spec, written.Spec)
}

func TestCatalogBuildRequiresRepositoryInputAndOutput(t *testing.T) {
	h, _ := catalogHarness(t)
	inventory := writeInventory(t)
	output := filepath.Join(t.TempDir(), "catalog.json")
	for name, args := range map[string][]string{
		"repository": {"catalog", "build", "--input=" + inventory, "--output=" + output},
		"input":      {"catalog", "build", "--repository=dir", "--output=" + output},
		"output":     {"catalog", "build", "--repository=dir", "--input=" + inventory},
	} {
		t.Run(name, func(t *testing.T) {
			got := h.run(args...)
			require.ErrorIs(t, got.err, errUtils.ErrRequiredFlagNotProvided)
		})
	}
	t.Run("inventory is validated before the service runs", func(t *testing.T) {
		bad := filepath.Join(t.TempDir(), "bad.yaml")
		require.NoError(t, os.WriteFile(bad, []byte("kind: Nope"), 0o600))
		got := h.run("catalog", "build", "--repository=dir", "--input="+bad, "--output="+output)
		require.ErrorIs(t, got.err, errUtils.ErrArtifactCatalogInvalid)
		_, statErr := os.Stat(output)
		require.ErrorIs(t, statErr, os.ErrNotExist, "nothing is written for an invalid inventory")
	})
}

func TestCatalogPublishReadsTheSnapshotAndPassesTheExpectedDigest(t *testing.T) {
	h, maintainer := catalogHarness(t)
	file := catalog.New("catalog")
	path := filepath.Join(t.TempDir(), "catalog.yaml")
	require.NoError(t, catalog.WriteFile(path, file))
	maintainer.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, request *catalog.PublishRequest) (*catalog.Result, error) {
		assert.Equal(t, "dir", request.Repository)
		assert.Equal(t, "sha256:abc", request.ExpectedDigest)
		assert.Equal(t, "catalog", request.File.Metadata.Name)
		return sampleResult(), nil
	})
	got := h.run("catalog", "publish", path, "--repository=dir", "--expected-digest=sha256:abc")
	require.NoError(t, got.err)
	for _, want := range []string{`catalog:\s+oci://`, `revision:\s+3`, `listed:\s+4`, `added:\s+a:1`, `updated:\s+b:2`, `removed:\s+c:3`, `lock_scope:\s+host`} {
		assert.Regexp(t, want, got.stdout)
	}
}

func TestCatalogPublishFailureKeepsTheExitCodeAndExplainsTheRemedy(t *testing.T) {
	h, maintainer := catalogHarness(t)
	path := filepath.Join(t.TempDir(), "catalog.json")
	require.NoError(t, catalog.WriteFile(path, catalog.New("catalog")))
	maintainer.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(nil, errors.Join(errUtils.ErrArtifactCatalogConflict)).Times(2)

	got := h.run("catalog", "publish", path, "--repository=dir")
	require.ErrorIs(t, got.err, errUtils.ErrArtifactCatalogConflict)
	assert.Empty(t, got.stdout, "text format prints nothing for a failure")

	got = h.run("catalog", "publish", path, "--repository=dir", "--format=json")
	require.ErrorIs(t, got.err, errUtils.ErrArtifactCatalogConflict)
	envelope := decodeObject(t, got.stdout)
	assert.Equal(t, "catalog-publish", envelope["action"])
	assert.Equal(t, "dir", envelope["repository"])
	assert.Contains(t, envelope["error"], "catalog changed")
}

func TestCatalogPublishRejectsAnInvalidSnapshotFile(t *testing.T) {
	h, _ := catalogHarness(t)
	got := h.run("catalog", "publish", filepath.Join(t.TempDir(), "missing.json"), "--repository=dir")
	require.ErrorIs(t, got.err, os.ErrNotExist)
	bad := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte("{}"), 0o600))
	got = h.run("catalog", "publish", bad, "--repository=dir")
	require.ErrorIs(t, got.err, errUtils.ErrArtifactCatalogInvalid)
	got = h.run("catalog", "publish", "--repository=dir")
	require.ErrorIs(t, got.err, errUtils.ErrInvalidPositionalArgs)
}

func TestCatalogAddListsEveryReference(t *testing.T) {
	h, maintainer := catalogHarness(t)
	maintainer.EXPECT().Add(gomock.Any(), &catalog.AddRequest{
		Repository: "dir", Refs: []artifact.Ref{{Name: "a", Version: "1"}, {Name: "b", Digest: manifestDigest}},
	}).Return(sampleResult(), nil)
	got := h.run("catalog", "add", "a:1", "b@"+string(manifestDigest), "--repository=dir", "--format=json")
	require.NoError(t, got.err)
	envelope := decodeObject(t, got.stdout)
	assert.Equal(t, "catalog-add", envelope["action"])
	assert.EqualValues(t, 3, envelope["revision"])
	assert.Equal(t, []any{"a:1"}, envelope["added"])

	require.ErrorIs(t, h.run("catalog", "add", "--repository=dir").err, errUtils.ErrInvalidPositionalArgs)
	require.ErrorIs(t, h.run("catalog", "add", "latest", "--repository=dir").err, errUtils.ErrArtifactRefInvalid, "there is no implicit latest")
}

func TestCatalogRemoveUnlistsOneVersion(t *testing.T) {
	h, maintainer := catalogHarness(t)
	maintainer.EXPECT().Remove(gomock.Any(), &catalog.RemoveRequest{Repository: "dir", Ref: artifact.Ref{Name: "a", Version: "1"}}).Return(sampleResult(), nil)
	require.NoError(t, h.run("catalog", "remove", "a:1", "--repository=dir").err)
	require.ErrorIs(t, h.run("catalog", "remove", "a", "--repository=dir").err, errUtils.ErrArtifactRefInvalid)
	require.ErrorIs(t, h.run("catalog", "remove", "--repository=dir").err, errUtils.ErrInvalidPositionalArgs)
}

func TestCatalogSyncMergesTheInventory(t *testing.T) {
	h, maintainer := catalogHarness(t)
	maintainer.EXPECT().Sync(gomock.Any(), &catalog.SyncRequest{
		Repository: "dir", Refs: []artifact.Ref{{Name: "components/terraform/vpc", Version: "1.2.0"}, {Name: "tools/helper", Version: "0.1.0"}},
	}).Return(sampleResult(), nil)
	got := h.run("catalog", "sync", "--repository=dir", "--input="+writeInventory(t), "--format=yaml")
	require.NoError(t, got.err)
	assert.Contains(t, got.stdout, "action: catalog-sync")

	require.ErrorIs(t, h.run("catalog", "sync", "--repository=dir").err, errUtils.ErrRequiredFlagNotProvided)
	require.ErrorIs(t, h.run("catalog", "sync", "extra", "--repository=dir", "--input=x").err, errUtils.ErrInvalidPositionalArgs)
}

func TestCatalogCommandsRejectUnsupportedFormatsAndMissingConfiguration(t *testing.T) {
	h, _ := catalogHarness(t)
	require.ErrorIs(t, h.run("catalog", "remove", "a:1", "--repository=dir", "--format=csv").err, errUtils.ErrInvalidFlag)

	previous := atmosConfigPtr
	atmosConfigPtr = nil
	t.Cleanup(func() { atmosConfigPtr = previous })
	require.ErrorIs(t, h.run("catalog", "remove", "a:1", "--repository=dir").err, errUtils.ErrArtifactConfigInvalid)
}

func TestCatalogFactoryFailureIsReported(t *testing.T) {
	h, _ := catalogHarness(t)
	failure := errors.New("cannot compose")
	newCatalogs = func(*schema.AtmosConfiguration) (catalog.Maintainer, error) { return nil, failure }
	require.ErrorIs(t, h.run("catalog", "remove", "a:1", "--repository=dir").err, failure)

	restore := UseCatalogFactory(func(*schema.AtmosConfiguration) (catalog.Maintainer, error) { return nil, failure })
	require.ErrorIs(t, h.run("catalog", "remove", "a:1", "--repository=dir").err, failure)
	restore()
}

func TestCatalogCommandsAreRegisteredWithDocumentationAndHints(t *testing.T) {
	command := NewCommand()
	var catalogCmd *cobra.Command
	for _, child := range command.Commands() {
		if child.Name() == "catalog" {
			catalogCmd = child
		}
	}
	require.NotNil(t, catalogCmd, "the catalog group is registered")
	var names []string
	for _, child := range catalogCmd.Commands() {
		names = append(names, child.Name())
		assert.NotEmpty(t, child.Example, "%s has usage examples", child.Name())
		assert.NotEmpty(t, child.Long, child.Name())
	}
	assert.ElementsMatch(t, []string{"build", "publish", "add", "remove", "sync"}, names)

	decorated := decorate(errUtils.ErrArtifactCatalogNotConfigured, nil)
	assert.Contains(t, errUtils.Format(decorated, errUtils.FormatterConfig{}), "catalog: oci://")
}
