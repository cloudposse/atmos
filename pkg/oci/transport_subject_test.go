package oci_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci"
)

// layoutRoots reads the digests listed in a layout's index.json.
func layoutRoots(t *testing.T, directory string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(directory, ocispec.ImageIndexFile))
	require.NoError(t, err)
	var index ocispec.Index
	require.NoError(t, json.Unmarshal(raw, &index))
	roots := make([]string, 0, len(index.Manifests))
	for _, manifest := range index.Manifests {
		roots = append(roots, manifest.Digest.String())
	}
	return roots
}

func TestLayoutPush_RecordsOnlyManifestsThatNameASubject(t *testing.T) {
	const fourMiB = 4 << 20
	subject := `{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:` + strings.Repeat("a", 64) + `","size":1}`
	padding := strings.Repeat("p", fourMiB+1024)
	tests := []struct {
		name       string
		manifest   string
		wantRecord bool
		wantErr    error
	}{
		{"attachment is recorded", `{"schemaVersion":2,"subject":` + subject + `}`, true, nil},
		{"subject after other fields is found", `{"schemaVersion":2,"layers":[{"a":[1,{"b":null}]}],"annotations":{"k":"v"},"subject":` + subject + `}`, true, nil},
		{"manifest without a subject is not recorded", `{"schemaVersion":2,"layers":[]}`, false, nil},
		{"null subject is not recorded", `{"schemaVersion":2,"subject":null}`, false, nil},
		{"large manifest without a subject is accepted", `{"schemaVersion":2,"annotations":{"padding":"` + padding + `"}}`, false, nil},
		{"large attachment is refused", `{"schemaVersion":2,"annotations":{"padding":"` + padding + `"},"subject":` + subject + `}`, false, errUtils.ErrOCIIntegrity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			directory := filepath.Join(t.TempDir(), "layout")
			layout, err := oci.OpenLayout(ctx, directory)
			require.NoError(t, err)
			descriptor := oci.Descriptor(ocispec.MediaTypeImageManifest, []byte(tt.manifest))

			err = layout.Push(ctx, descriptor, strings.NewReader(tt.manifest))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.ErrorIs(t, err, errUtils.ErrOCILayout)
				return
			}
			require.NoError(t, err)
			exists, err := layout.Exists(ctx, descriptor)
			require.NoError(t, err)
			assert.True(t, exists)
			if tt.wantRecord {
				assert.Equal(t, []string{descriptor.Digest.String()}, layoutRoots(t, directory))
			} else {
				assert.Empty(t, layoutRoots(t, directory))
			}

			// Pushing the same manifest again reports it exists and keeps the index free of duplicates.
			before := layoutRoots(t, directory)
			err = layout.Push(ctx, descriptor, strings.NewReader(tt.manifest))
			require.ErrorIs(t, err, errUtils.ErrOCILayout)
			assert.Equal(t, before, layoutRoots(t, directory))
		})
	}
}
