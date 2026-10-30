package exec

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
)

func TestVendorLocalFileURL(t *testing.T) {
	root := t.TempDir()
	for _, singleFile := range []bool{false, true} {
		path := filepath.Join(root, "source with spaces")
		if singleFile {
			path = filepath.Join(path, "main.tf")
			require.NoError(t, os.WriteFile(path, []byte("terraform {}"), 0o600))
		} else {
			require.NoError(t, os.Mkdir(path, 0o700))
		}
		uri := (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/")}).String()
		oci, local, file, err := determineSourceType(&uri, root)
		require.NoError(t, err)
		require.False(t, oci)
		require.True(t, local)
		require.Equal(t, singleFile, file)
		require.Equal(t, path, uri)
	}
}

func TestVendorAbsoluteTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "destination")
	for _, declared := range []string{target, "destination"} {
		packages, err := processTargets(&processTargetsParams{
			AtmosConfig:          &schema.AtmosConfiguration{},
			VendorConfigFilePath: root,
			URI:                  filepath.Join(root, "source"),
			PkgType:              install.PkgTypeLocal,
			Source:               &schema.AtmosVendorSource{Targets: schema.AtmosVendorTargets{{Path: declared}}},
		})
		require.NoError(t, err)
		require.Len(t, packages, 1)
		require.Equal(t, target, packages[0].Target())
	}
}

func TestVendorFixtureLocalSources(t *testing.T) {
	manifestDir, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "vendor"))
	require.NoError(t, err)
	manifest, err := mergeVendorConfigFiles([]string{filepath.Join(manifestDir, "vendor.yaml")})
	require.NoError(t, err)
	checked := 0
	for _, source := range manifest.Spec.Sources {
		if source.Component != "test-components" && source.Component != "myapp2" {
			continue
		}
		checked++
		t.Run(source.Component, func(t *testing.T) {
			uri := source.Source
			oci, local, file, err := determineSourceType(&uri, manifestDir)
			require.NoError(t, err)
			require.False(t, oci)
			require.True(t, local)
			require.False(t, file)
			base := t.TempDir()
			target := filepath.Join(base, "component")
			pkg := install.NewAtmosVendorPackage(&install.AtmosPackageParams{
				Name: source.Component, URI: uri, TargetPath: target,
				PkgType: install.PkgTypeLocal, Source: source,
			})
			result, err := install.Install(&schema.AtmosConfiguration{BasePath: base}, pkg, install.InstallOptions{})
			require.NoError(t, err)
			require.NoError(t, result.Err)
			for _, name := range []string{"main.tf", "README.md"} {
				want, err := os.ReadFile(filepath.Join(uri, name))
				require.NoError(t, err)
				got, err := os.ReadFile(filepath.Join(target, name))
				require.NoError(t, err)
				require.Equal(t, string(want), string(got))
			}
		})
	}
	require.Equal(t, 2, checked)
}

func TestVendorRelativeFileURL(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, parent := range []bool{false, true} {
		t.Run(fmt.Sprintf("parent=%t", parent), func(t *testing.T) {
			root := t.TempDir()
			manifestDir := filepath.Join(root, "manifest")
			require.NoError(t, os.Mkdir(manifestDir, 0o700))
			sourceRoot, host := manifestDir, "."
			if parent {
				sourceRoot, host = root, ".."
			}
			sourceDir := filepath.Join(sourceRoot, "source with spaces")
			require.NoError(t, os.Mkdir(sourceDir, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("terraform {}"), 0o600))
			for _, singleFile := range []bool{false, true} {
				path := "/source with spaces"
				expected := sourceDir
				if singleFile {
					path += "/main.tf"
					expected = filepath.Join(sourceDir, "main.tf")
				}
				uri := (&url.URL{Scheme: "file", Host: host, Path: path}).String()
				oci, local, file, err := determineSourceType(&uri, manifestDir)
				require.NoError(t, err)
				require.False(t, oci)
				require.True(t, local)
				require.Equal(t, expected, uri)
				require.Equal(t, singleFile, file)
				if singleFile {
					continue
				}
				target := filepath.Join(manifestDir, "installed")
				pkg := install.NewAtmosVendorPackage(&install.AtmosPackageParams{
					Name: "relative-source", URI: uri, TargetPath: target, PkgType: install.PkgTypeLocal,
					Source: schema.AtmosVendorSource{},
				})
				result, err := install.Install(&schema.AtmosConfiguration{BasePath: manifestDir}, pkg, install.InstallOptions{})
				require.NoError(t, err)
				require.NoError(t, result.Err)
				got, err := os.ReadFile(filepath.Join(target, "main.tf"))
				require.NoError(t, err)
				require.Equal(t, "terraform {}", string(got))
			}
		})
	}
}
