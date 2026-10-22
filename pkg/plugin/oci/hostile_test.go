package oci

import (
	"io"
	"log"
	"maps"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/plugin/oci/ocitest"
)

// nativeFixture is a clean plugin whose executable is a real linux/amd64 header.
func nativeFixture(t *testing.T) (string, *extension.Registry) {
	t.Helper()
	directory, descriptors := validationFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), ocitest.Binary("linux", "amd64"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", "provider.json"), []byte(`{"type":"object"}`), 0o644))
	return directory, descriptors
}

func setExecutable(t *testing.T, directory, path string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(directory, "plugin.yaml"))
	require.NoError(t, err)
	changed := strings.ReplaceAll(string(raw), "bin/plugin", path)
	require.NotEqual(t, string(raw), changed)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(changed), 0o644))
}

func requireNothingWritten(t *testing.T, output string) {
	t.Helper()
	_, err := os.Lstat(output)
	require.ErrorIs(t, err, os.ErrNotExist, "a rejected pack must not create the layout")
	_, err = os.Lstat(filepath.Dir(output))
	require.ErrorIs(t, err, os.ErrNotExist, "a rejected pack must not create or stage anything")
}

func TestPackRejectsHostilePackagesWithoutWritingAnything(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, directory string)
		want    error
	}{
		{"executable points into .git", func(t *testing.T, directory string) {
			require.NoError(t, os.MkdirAll(filepath.Join(directory, ".git"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(directory, ".git", "config"), []byte("[core]\n"), 0o644))
			setExecutable(t, directory, ".git/config")
		}, errUtils.ErrPluginBundlePathForbidden},
		{"executable points into .atmos", func(t *testing.T, directory string) {
			require.NoError(t, os.MkdirAll(filepath.Join(directory, ".atmos"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(directory, ".atmos", "plugin"), ocitest.Binary("linux", "amd64"), 0o755))
			setExecutable(t, directory, ".atmos/plugin")
		}, errUtils.ErrPluginBundlePathForbidden},
		{"hidden credentials schema", func(t *testing.T, directory string) {
			require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", ".credentials.json"), []byte(`{}`), 0o644))
		}, errUtils.ErrPluginBundlePathForbidden},
		{"credential-named schema", func(t *testing.T, directory string) {
			require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", "aws-credentials.json"), []byte(`{}`), 0o644))
		}, errUtils.ErrPluginBundlePathForbidden},
		{"symlinked schema", func(t *testing.T, directory string) {
			skipWithoutSymlinks(t, filepath.Join(directory, "plugin.yaml"), filepath.Join(directory, "schemas", "link.json"))
		}, errUtils.ErrPluginBundlePathForbidden},
		{"symlinked executable", func(t *testing.T, directory string) {
			require.NoError(t, os.Rename(filepath.Join(directory, "bin", "plugin"), filepath.Join(directory, "bin", "real")))
			skipWithoutSymlinks(t, "real", filepath.Join(directory, "bin", "plugin"))
		}, errUtils.ErrPluginBundlePathForbidden},
		{"symlinked executable directory", func(t *testing.T, directory string) {
			require.NoError(t, os.MkdirAll(filepath.Join(directory, "dist"), 0o755))
			require.NoError(t, os.Rename(filepath.Join(directory, "bin", "plugin"), filepath.Join(directory, "dist", "plugin")))
			require.NoError(t, os.Remove(filepath.Join(directory, "bin")))
			skipWithoutSymlinks(t, "dist", filepath.Join(directory, "bin"))
		}, errUtils.ErrPluginBundlePathForbidden},
		{"executable is text", func(t *testing.T, directory string) {
			require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("#!/bin/sh\necho hi\n"), 0o755))
		}, errUtils.ErrPluginBundleExecutableInvalid},
		{"executable is for another OS", func(t *testing.T, directory string) {
			require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), ocitest.Binary("darwin", "amd64"), 0o755))
		}, errUtils.ErrPluginBundleExecutableInvalid},
		{"executable is for another architecture", func(t *testing.T, directory string) {
			require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), ocitest.Binary("linux", "arm64"), 0o755))
		}, errUtils.ErrPluginBundleExecutableInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directory, descriptors := nativeFixture(t)
			tt.prepare(t, directory)
			output := filepath.Join(t.TempDir(), "out", "layout")
			err := Pack(directory, output, descriptors, WithExecutableFormatCheck())
			require.ErrorIs(t, err, tt.want)
			requireNothingWritten(t, output)
		})
	}
}

func skipWithoutSymlinks(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable on %s: %v", runtime.GOOS, err)
	}
}

func TestFormatCheckIsOptInForPackButPathGuardAlwaysRuns(t *testing.T) {
	directory, descriptors := nativeFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("fixture, never executed"), 0o755))
	require.NoError(t, Pack(directory, filepath.Join(t.TempDir(), "layout"), descriptors), "library callers with placeholder binaries keep working")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", ".hidden.json"), []byte(`{}`), 0o644))
	require.ErrorIs(t, Pack(directory, filepath.Join(t.TempDir(), "layout"), descriptors), errUtils.ErrPluginBundlePathForbidden)
}

func TestCleanBundleContainsExactlyTheAllowList(t *testing.T) {
	directory, descriptors := nativeFixture(t)
	// Junk beside the package must never be picked up.
	for _, junk := range []string{".env", ".gitignore", "README.md", "main.go", "terraform.tfstate"} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, junk), []byte("junk"), 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(directory, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, ".git", "config"), []byte("[core]\n"), 0o644))
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, Pack(directory, layout, descriptors, WithExecutableFormatCheck()))
	index, err := atmosoci.LoadIndex(layout, "")
	require.NoError(t, err)
	release, err := Inspect(index, "")
	require.NoError(t, err)
	files, err := release.Files()
	require.NoError(t, err)
	require.Equal(t, []BundleFile{
		{Platform: "linux/amd64", Path: "bin/plugin", Size: int64(len(ocitest.Binary("linux", "amd64"))), Mode: "0755"},
		{Platform: "linux/amd64", Path: "plugin.yaml", Size: int64(len(validationManifest)), Mode: "0644"},
		{Platform: "linux/amd64", Path: "schemas/provider.json", Size: int64(len(`{"type":"object"}`)), Mode: "0644"},
	}, files)
	fromLayout, err := LayoutFiles(layout)
	require.NoError(t, err)
	require.Equal(t, files, fromLayout, "offline inventory must verify and report the same payload as the release")
}

// layoutWithArchive builds an OCI layout like Pack does, but around caller-chosen
// payload files, imitating a layout produced by another tool or an older version.
func layoutWithArchive(t *testing.T, directory string, extra map[string][]byte, executable []byte) string {
	t.Helper()
	definition, err := manifest.Load(directory)
	require.NoError(t, err)
	files, err := packageFiles(directory)
	require.NoError(t, err)
	maps.Copy(files, extra)
	archive, err := bundle(files, definition.Spec.Runtime.Executable, executable)
	require.NoError(t, err)
	annotations := releaseAnnotations(definition, files["plugin.yaml"])
	platform := &v1.Platform{OS: "linux", Architecture: "amd64"}
	img, err := mutate.AppendLayers(empty.Image, static.NewLayer(archive, BundleMediaType))
	require.NoError(t, err)
	config, err := img.ConfigFile()
	require.NoError(t, err)
	config.OS, config.Architecture = platform.OS, platform.Architecture
	config.Config.Labels = map[string]string{manifestLabel: string(files["plugin.yaml"])}
	img, err = mutate.ConfigFile(img, config)
	require.NoError(t, err)
	img = mutate.ConfigMediaType(mutate.MediaType(img, types.OCIManifestSchema1), ConfigMediaType)
	img = mutate.Annotations(img, annotations).(v1.Image)
	index := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: img, Descriptor: v1.Descriptor{Platform: platform, Annotations: annotations}})
	index = mutate.Annotations(index, annotations).(v1.ImageIndex)
	layout := t.TempDir()
	require.NoError(t, atmosoci.SaveIndex(layout, index))
	return layout
}

func TestPublishReinspectsLayoutsBuiltByOtherTools(t *testing.T) {
	binary := ocitest.Binary("linux", "amd64")
	tests := []struct {
		name  string
		extra map[string][]byte
		want  error
	}{
		{"git config", map[string][]byte{".git/config": []byte("[core]\n")}, errUtils.ErrPluginBundlePathForbidden},
		{"env file", map[string][]byte{".env": []byte("TOKEN=1\n")}, errUtils.ErrPluginBundlePathForbidden},
		{"terraform state", map[string][]byte{"terraform.tfstate": []byte("{}")}, errUtils.ErrPluginBundlePathForbidden},
		{"hidden schema", map[string][]byte{"schemas/.credentials.json": []byte("{}")}, errUtils.ErrPluginBundlePathForbidden},
		{"unlisted file", map[string][]byte{"notes.txt": []byte("hello")}, errUtils.ErrPluginBundlePathForbidden},
		{"second binary", map[string][]byte{"bin/other": binary}, errUtils.ErrPluginBundlePathForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directory, descriptors := nativeFixture(t)
			layout := layoutWithArchive(t, directory, tt.extra, binary)
			index, err := atmosoci.LoadIndex(layout, "")
			require.NoError(t, err)
			release, err := Inspect(index, "")
			require.NoError(t, err)
			destination := filepath.Join(t.TempDir(), "extracted")
			require.ErrorIs(t, release.Extract("linux/amd64", destination), tt.want)
			_, err = os.Stat(destination)
			require.ErrorIs(t, err, os.ErrNotExist, "reject unlisted archive files before writing any payload")

			// The mock has no expectations: any registry call fails the test.
			client := NewMockRegistry(gomock.NewController(t))
			_, err = Publish(t.Context(), client, layout, "registry.example.com/acme/plugin", descriptors, WithExecutableFormatCheck())
			require.ErrorIs(t, err, tt.want)
			for key := range tt.extra {
				require.Contains(t, err.Error(), key)
			}
		})
	}
	t.Run("text executable", func(t *testing.T) {
		directory, descriptors := nativeFixture(t)
		layout := layoutWithArchive(t, directory, nil, []byte("#!/bin/sh\n"))
		client := NewMockRegistry(gomock.NewController(t))
		_, err := Publish(t.Context(), client, layout, "registry.example.com/acme/plugin", descriptors, WithExecutableFormatCheck())
		require.ErrorIs(t, err, errUtils.ErrPluginBundleExecutableInvalid)
	})
	t.Run("wrong platform executable", func(t *testing.T) {
		directory, descriptors := nativeFixture(t)
		layout := layoutWithArchive(t, directory, nil, ocitest.Binary("darwin", "arm64"))
		client := NewMockRegistry(gomock.NewController(t))
		_, err := Publish(t.Context(), client, layout, "registry.example.com/acme/plugin", descriptors, WithExecutableFormatCheck())
		require.ErrorIs(t, err, errUtils.ErrPluginBundleExecutableInvalid)
	})
}

func TestPublishAcceptsCleanLayoutAndReportsInventory(t *testing.T) {
	directory, descriptors := nativeFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, Pack(directory, layout, descriptors, WithExecutableFormatCheck()))
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	destination := strings.TrimPrefix(server.URL, "http://") + "/acme/plugin"
	release, err := Publish(t.Context(), atmosoci.NewClient(nil), layout, destination, descriptors, WithExecutableFormatCheck())
	require.NoError(t, err)
	files, err := release.Files()
	require.NoError(t, err)
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	require.Equal(t, []string{"bin/plugin", "plugin.yaml", "schemas/provider.json"}, paths)
}

func TestExtractRejectsHostileReleaseBeforeWritingAnything(t *testing.T) {
	directory, _ := nativeFixture(t)
	layout := layoutWithArchive(t, directory, map[string][]byte{".git/config": []byte("[core]\n")}, ocitest.Binary("linux", "amd64"))
	index, err := atmosoci.LoadIndex(layout, "")
	require.NoError(t, err)
	release, err := Inspect(index, "")
	require.NoError(t, err)
	destination := t.TempDir()
	require.ErrorIs(t, release.Extract("linux/amd64", destination), errUtils.ErrPluginBundlePathForbidden)
	entries, err := os.ReadDir(destination)
	require.NoError(t, err)
	require.Empty(t, entries, "a hostile release must not materialize any file")
}

func TestValidatePackageAppliesGuardsWithoutABuiltBinary(t *testing.T) {
	load := func(t *testing.T, directory string) *manifest.Config {
		t.Helper()
		definition, err := manifest.Load(directory)
		require.NoError(t, err)
		return definition
	}
	t.Run("clean package with a native binary", func(t *testing.T) {
		directory, _ := nativeFixture(t)
		require.NoError(t, ValidatePackage(directory, load(t, directory)))
	})
	t.Run("missing binary is skipped", func(t *testing.T) {
		directory, _ := nativeFixture(t)
		require.NoError(t, os.Remove(filepath.Join(directory, "bin", "plugin")))
		require.NoError(t, ValidatePackage(directory, load(t, directory)))
	})
	t.Run("hostile executable path is rejected even when absent", func(t *testing.T) {
		directory, _ := nativeFixture(t)
		setExecutable(t, directory, ".git/config")
		require.ErrorIs(t, ValidatePackage(directory, load(t, directory)), errUtils.ErrPluginBundlePathForbidden)
	})
	t.Run("hidden schema", func(t *testing.T) {
		directory, _ := nativeFixture(t)
		require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", ".credentials.json"), []byte(`{}`), 0o644))
		require.ErrorIs(t, ValidatePackage(directory, load(t, directory)), errUtils.ErrPluginBundlePathForbidden)
	})
	t.Run("existing binary of the wrong format", func(t *testing.T) {
		directory, _ := nativeFixture(t)
		require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("text"), 0o755))
		require.ErrorIs(t, ValidatePackage(directory, load(t, directory)), errUtils.ErrPluginBundleExecutableInvalid)
	})
}

func TestLayoutInventoryRejectsMissingAndInvalidIndexes(t *testing.T) {
	_, err := LayoutFiles(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	layout := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(layout, "blobs", "sha256"), 0o755))
	invalid := mutate.Annotations(empty.Index, map[string]string{"fixture": "missing-platforms"}).(v1.ImageIndex)
	require.NoError(t, atmosoci.SaveIndex(layout, invalid))
	files, err := LayoutFiles(layout)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.Nil(t, files)
}
