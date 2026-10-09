package source

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestVendorSource_SingleFilePreservesComponentDirectory(t *testing.T) {
	const template = "Resources: {}\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, template) }))
	defer server.Close()
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "replace"}[existing], func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "component")
			if existing {
				require.NoError(t, os.MkdirAll(target, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(target, "old.yaml"), []byte("old"), 0o644))
			}
			require.NoError(t, VendorSource(context.Background(), nil, &schema.VendorComponentSource{Uri: server.URL + "/template.yaml"}, target))
			require.DirExists(t, target)
			got, err := os.ReadFile(filepath.Join(target, "template.yaml"))
			require.NoError(t, err)
			assert.Equal(t, template, string(got))
			assert.NoFileExists(t, filepath.Join(target, "old.yaml"))
		})
	}
}

func TestVendorSource_SingleFileFiltersAndPermissions(t *testing.T) {
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "template.yaml")
	require.NoError(t, os.WriteFile(sourcePath, []byte("Resources: {}\n"), 0o755))
	for _, tc := range []struct {
		name             string
		include, exclude []string
		wantFile         bool
	}{
		{name: "included", include: []string{"*.yaml"}, wantFile: true},
		{name: "excluded", exclude: []string{"*.yaml"}},
		{name: "not included", include: []string{"*.json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "component")
			spec := &schema.VendorComponentSource{Uri: sourcePath, IncludedPaths: tc.include, ExcludedPaths: tc.exclude}
			require.NoError(t, VendorSource(context.Background(), nil, spec, target))
			require.DirExists(t, target)
			path := filepath.Join(target, "template.yaml")
			if !tc.wantFile {
				assert.NoFileExists(t, path)
				return
			}
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, "Resources: {}\n", string(got))
			srcInfo, err := os.Stat(sourcePath)
			require.NoError(t, err)
			dstInfo, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, srcInfo.Mode().Perm(), dstInfo.Mode().Perm())
		})
	}
}

func TestVendorToTarget_SingleFilePreservesDirectory(t *testing.T) {
	const template = "Resources: {}\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, template) }))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "component")
	require.NoError(t, vendorToTarget(context.Background(), &schema.AtmosConfiguration{}, &schema.VendorComponentSource{Uri: server.URL + "/template.yaml"}, vendorTarget{component: "example", path: target}))
	require.DirExists(t, target)
	got, err := os.ReadFile(filepath.Join(target, "template.yaml"))
	require.NoError(t, err)
	assert.Equal(t, template, string(got))
}

func TestCopySingleFileToDirectory_Errors(t *testing.T) {
	source := filepath.Join(t.TempDir(), "template.yaml")
	require.NoError(t, os.WriteFile(source, []byte("Resources: {}"), 0o644))
	t.Run("replacement disabled", func(t *testing.T) {
		target := t.TempDir()
		err := copySingleFileToDirectory(source, target, &schema.VendorComponentSource{}, vendorSourceOptions{replaceTarget: false})
		require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
	})
	t.Run("missing source", func(t *testing.T) {
		err := copySingleFileToDirectory(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "target"), &schema.VendorComponentSource{}, vendorSourceOptions{replaceTarget: true})
		require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
	})
	t.Run("invalid filter", func(t *testing.T) {
		err := copySingleFileToDirectory(source, filepath.Join(t.TempDir(), "target"), &schema.VendorComponentSource{IncludedPaths: []string{"["}}, vendorSourceOptions{replaceTarget: true})
		require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
	})
}
