package oci_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/errdef"

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

// pushSubjectManifest pushes an image manifest that optionally names a subject and returns its descriptor.
func pushSubjectManifest(t *testing.T, layout *oci.Layout, subject *ocispec.Descriptor, note string) ocispec.Descriptor {
	t.Helper()
	ctx := t.Context()
	config := []byte("{}")
	configDesc := oci.Descriptor("application/vnd.oci.empty.v1+json", config)
	if exists, err := layout.Exists(ctx, configDesc); err == nil && !exists {
		require.NoError(t, layout.Push(ctx, configDesc, strings.NewReader(string(config))))
	}
	manifest := ocispec.Manifest{Config: configDesc, Layers: []ocispec.Descriptor{}, Subject: subject, Annotations: map[string]string{"note": note}}
	manifest.SchemaVersion = 2
	manifest.MediaType = ocispec.MediaTypeImageManifest
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	desc := oci.Descriptor(ocispec.MediaTypeImageManifest, raw)
	require.NoError(t, layout.Push(ctx, desc, strings.NewReader(string(raw))))
	return desc
}

// Moving a tag off an attachment manifest must not drop the attachment from the layout: it stays an
// unnamed root, so reopening the layout still finds it as a referrer of its subject.
func TestTagMovePreservesTheDisplacedAttachmentRoot(t *testing.T) {
	ctx := t.Context()
	directory := filepath.Join(t.TempDir(), "layout")
	layout, err := oci.OpenLayout(ctx, directory)
	require.NoError(t, err)
	subject := pushSubjectManifest(t, layout, nil, "subject")
	attachment := pushSubjectManifest(t, layout, &subject, "attachment")
	replacement := pushSubjectManifest(t, layout, nil, "replacement")
	require.NoError(t, layout.Tag(ctx, attachment, "moving"))

	require.NoError(t, layout.Tag(ctx, replacement, "moving"))

	reopened, err := oci.OpenLayout(ctx, directory)
	require.NoError(t, err)
	resolved, err := reopened.Resolve(ctx, "moving")
	require.NoError(t, err)
	assert.Equal(t, replacement.Digest, resolved.Digest, "the tag moved to the new manifest")
	referrers, err := reopened.Predecessors(ctx, subject)
	require.NoError(t, err)
	digests := make([]string, 0, len(referrers))
	for _, referrer := range referrers {
		digests = append(digests, referrer.Digest.String())
	}
	assert.Contains(t, digests, attachment.Digest.String(), "the displaced attachment is still discoverable after reopening")
	assert.Contains(t, layoutRoots(t, directory), attachment.Digest.String())
}

// A tag that could not be written to index.json would vanish when the layout is reopened, so it must not
// resolve: otherwise a recovery check would report success for a tag that was never persisted.
func TestResolveDoesNotTrustATagThatWasNotPersisted(t *testing.T) {
	ctx := t.Context()
	directory := filepath.Join(t.TempDir(), "layout")
	layout, err := oci.OpenLayout(ctx, directory)
	require.NoError(t, err)
	root := pushSubjectManifest(t, layout, nil, "root")

	// Replace index.json with a non-empty directory so the atomic write cannot land on any platform.
	indexPath := filepath.Join(directory, "index.json")
	require.NoError(t, os.Remove(indexPath))
	require.NoError(t, os.MkdirAll(filepath.Join(indexPath, "blocker"), 0o755))

	require.Error(t, layout.Tag(ctx, root, "1.0.0"), "the index write fails")
	_, err = layout.Resolve(ctx, "1.0.0")
	require.Error(t, err, "an unpersisted tag does not resolve")
	require.ErrorIs(t, err, errdef.ErrNotFound)

	resolved, err := layout.Resolve(ctx, root.Digest.String())
	require.NoError(t, err, "a digest reference still resolves")
	assert.Equal(t, root.Digest, resolved.Digest)
}

// A relative layout root is anchored when the layout is opened: changing the working directory afterwards
// must not send index commits to a different directory than the one holding the blobs.
func TestOpenLayoutAnchorsARelativeRoot(t *testing.T) {
	ctx := t.Context()
	base := t.TempDir()
	other := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(other, "layout"), 0o755))
	t.Chdir(base)
	layout, err := oci.OpenLayout(ctx, "layout")
	require.NoError(t, err)
	root := pushSubjectManifest(t, layout, nil, "root")

	t.Chdir(other)
	require.NoError(t, layout.Tag(ctx, root, "1.0.0"))

	assert.Contains(t, layoutRoots(t, filepath.Join(base, "layout")), root.Digest.String(), "the tag landed in the layout that was opened")
	_, err = os.Stat(filepath.Join(other, "layout", "index.json"))
	assert.ErrorIs(t, err, os.ErrNotExist, "an unrelated layout in the new working directory is untouched")
}

// A reference that exists throughout a concurrent Tag keeps resolving: the resolver and the committed index
// are read under one lock, so the tag moving between two digests never looks unpersisted.
func TestResolveIsAtomicWithConcurrentTags(t *testing.T) {
	ctx := t.Context()
	layout, err := oci.OpenLayout(ctx, filepath.Join(t.TempDir(), "layout"))
	require.NoError(t, err)
	first := pushSubjectManifest(t, layout, nil, "first")
	second := pushSubjectManifest(t, layout, nil, "second")
	require.NoError(t, layout.Tag(ctx, first, "moving"))

	const rounds = 300
	var wg sync.WaitGroup
	failures := make(chan error, rounds)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range rounds {
			target := first
			if i%2 == 0 {
				target = second
			}
			if err := layout.Tag(ctx, target, "moving"); err != nil {
				failures <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range rounds {
			if _, err := layout.Resolve(ctx, "moving"); err != nil {
				failures <- err
				return
			}
		}
	}()
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
}

func TestExtendedCopyWithoutSubjectTraversalKeepsTheSelectedRootAndItsReferrers(t *testing.T) {
	ctx := t.Context()
	source, err := oci.OpenLayout(ctx, filepath.Join(t.TempDir(), "source"))
	require.NoError(t, err)
	grandSubject := pushSubjectManifest(t, source, nil, "grand-subject")
	root := pushSubjectManifest(t, source, &grandSubject, "root")
	signature := pushSubjectManifest(t, source, &root, "signature")
	require.NoError(t, source.Tag(ctx, root, "1.0.0"))

	dest, err := oci.OpenLayout(ctx, filepath.Join(t.TempDir(), "dest"))
	require.NoError(t, err)
	got, err := oci.ExtendedCopy(ctx, oci.GraphSource{Store: source, Reference: "1.0.0"}, oci.Destination{Store: dest, Reference: "1.0.0"}, oci.WithoutSubjectTraversal())
	require.NoError(t, err)
	assert.Equal(t, root.Digest, got.Digest)

	for name, want := range map[string]struct {
		desc   ocispec.Descriptor
		wanted bool
	}{
		"the selected root":        {root, true},
		"the referrer of the root": {signature, true},
		"the subject of the root":  {grandSubject, false},
	} {
		exists, err := dest.Exists(ctx, want.desc)
		require.NoError(t, err, name)
		assert.Equal(t, want.wanted, exists, name)
	}
	resolved, err := dest.Resolve(ctx, "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, root.Digest, resolved.Digest)
}
