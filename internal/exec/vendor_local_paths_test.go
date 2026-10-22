package exec

import (
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
