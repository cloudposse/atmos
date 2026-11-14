package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// unsafeKeys are cache keys that must never resolve to a path outside the cache root.
var unsafeKeys = []struct {
	name string
	key  string
	want error
}{
	{"empty", "", errUtils.ErrCacheKeyRequired},
	{"parent traversal", "../escape.txt", errUtils.ErrPathTraversal},
	{"nested traversal", "providers/../../escape.txt", errUtils.ErrPathTraversal},
	{"trailing traversal", "providers/..", errUtils.ErrPathTraversal},
	{"current dir segment", "providers/./x", errUtils.ErrPathTraversal},
	{"absolute", "/etc/passwd", errUtils.ErrPathTraversal},
	{"backslash separator", `providers\..\..\escape.txt`, errUtils.ErrPathTraversal},
	{"NUL byte", "providers/x\x00.zip", errUtils.ErrPathTraversal},
}

func TestValidateKey_RejectsUnsafeKeys(t *testing.T) {
	for _, tc := range unsafeKeys {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, ValidateKey(tc.key), tc.want)
		})
	}
}

func TestValidateKey_AcceptsNormalKeys(t *testing.T) {
	for _, key := range []string{
		"providers/registry.terraform.io/hashicorp/aws/index.json",
		"providers/registry.terraform.io/hashicorp/aws/terraform-provider-aws_5.95.0_linux_amd64.zip",
		"modules/registry.terraform.io/terraform-aws-modules/vpc/aws/5.0.0.tar.gz",
		"obj/with..dots/in..name.zip",
		"metadata/produced.json",
	} {
		t.Run(key, func(t *testing.T) {
			require.NoError(t, ValidateKey(key))
		})
	}
}

func TestFileStore_RejectsTraversalOnEveryOperation(t *testing.T) {
	// Layout: <parent>/root is the cache root; <parent>/escape.txt is a sibling that a
	// traversing key would hit.
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	require.NoError(t, os.MkdirAll(root, 0o755))
	secret := filepath.Join(parent, "escape.txt")
	require.NoError(t, os.WriteFile(secret, []byte("secret"), 0o600))
	store := NewFileStore(root)

	for _, tc := range unsafeKeys {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := store.Stat(tc.key)
			require.ErrorIs(t, err, tc.want)

			rc, _, err := store.Open(tc.key)
			require.ErrorIs(t, err, tc.want)
			assert.Nil(t, rc)

			_, err = store.Commit(t.Context(), CommitRequest{
				Key:  tc.key,
				Data: bytes.NewReader([]byte("overwrite")),
				Kind: KindArtifact,
			})
			require.ErrorIs(t, err, tc.want)

			ran := false
			err = store.Lock(tc.key).WithLockContext(t.Context(), func() error {
				ran = true
				return nil
			})
			require.ErrorIs(t, err, tc.want)
			assert.False(t, ran, "the guarded function must not run for a rejected key")
			require.ErrorIs(t, store.Lock(tc.key).WithLock(func() error { return nil }), tc.want)
			require.ErrorIs(t, store.Lock(tc.key).WithRLock(func() error { return nil }), tc.want)
			acquired, err := store.Lock(tc.key).TryWithRLock(func() error { return nil })
			require.ErrorIs(t, err, tc.want)
			assert.False(t, acquired)
		})
	}

	// Nothing outside the root was touched, and nothing was written inside it.
	got, err := os.ReadFile(secret)
	require.NoError(t, err)
	assert.Equal(t, "secret", string(got), "a sibling file outside the cache root must be untouched")
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "a rejected key must not create anything under the cache root")
	entries, err = os.ReadDir(parent)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "a rejected key must not create anything beside the cache root")
}

func TestFileStore_NormalKeysStillWork(t *testing.T) {
	store := NewFileStore(t.TempDir())
	key := "providers/registry.terraform.io/hashicorp/aws/index.json"
	payload := []byte(`{"versions":{}}`)

	require.NoError(t, store.Lock(key).WithLock(func() error {
		_, err := store.Commit(t.Context(), CommitRequest{Key: key, Data: bytes.NewReader(payload), Kind: KindMetadata})
		return err
	}))

	meta, ok, err := store.Stat(key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, int64(len(payload)), meta.Size)
	assert.Equal(t, sha256Hex(payload), meta.SHA256)
}

func TestFileStore_RelativeRootStillWorks(t *testing.T) {
	t.Chdir(t.TempDir())
	store := NewFileStore("cache")
	key := "obj/a.zip"
	_, err := store.Commit(t.Context(), CommitRequest{Key: key, Data: bytes.NewReader([]byte("x")), Kind: KindArtifact})
	require.NoError(t, err)
	_, ok, err := store.Stat(key)
	require.NoError(t, err)
	assert.True(t, ok)
	require.ErrorIs(t, ValidateKey("../x"), errUtils.ErrPathTraversal)
	_, _, err = store.Stat("../x")
	require.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

func TestProxy_RejectsTraversalCacheKeyWithBadRequest(t *testing.T) {
	// A mirror that derives its key from the request path (as the real mirrors do) must
	// not be able to write outside the cache root.
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	require.NoError(t, os.MkdirAll(root, 0o755))

	var upstreamHits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamHits++
		_, _ = w.Write([]byte("payload"))
	}))
	defer upstream.Close()

	mirror := mirrorFunc{
		handles: func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/leak/") },
		route: func(r *http.Request) (Route, error) {
			return Route{
				Key:      "obj/" + strings.TrimPrefix(r.URL.Path, "/leak/"),
				Kind:     KindArtifact,
				Upstream: UpstreamRequest{URL: upstream.URL},
			}, nil
		},
	}
	srv := startProxy(t, []Mirror{mirror}, Options{Store: NewFileStore(root)})

	// The URL-encoded traversal survives net/http path cleaning because it arrives in
	// r.URL.Path decoded only after routing; request it raw.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.BaseURL()+"leak/%2e%2e/%2e%2e/escape.zip", http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Zero(t, upstreamHits, "a rejected key must not trigger an upstream fetch")
	_, statErr := os.Stat(filepath.Join(parent, "escape.zip"))
	assert.True(t, os.IsNotExist(statErr), "nothing may be written outside the cache root")

	// A normal key through the same mirror still works.
	body := httpGet(t, srv.BaseURL()+"leak/aws.zip")
	assert.Equal(t, "payload", string(body))
}

func TestFileStore_ObjectPathContainmentAcrossRoots(t *testing.T) {
	sep := string(filepath.Separator)
	// The filesystem (or drive) root of the temp dir: "/" on Unix, `C:\` on Windows.
	fsRoot := filepath.VolumeName(t.TempDir()) + sep
	nested := filepath.Join(t.TempDir(), "cache")

	tests := []struct {
		name    string
		root    string
		key     string
		want    string
		wantErr error
	}{
		{name: "filesystem root accepts a normal key", root: fsRoot, key: "obj/a.zip", want: filepath.Join(fsRoot, "obj", "a.zip")},
		{name: "filesystem root accepts a single-segment key", root: fsRoot, key: "a.zip", want: filepath.Join(fsRoot, "a.zip")},
		{name: "filesystem root with trailing separator is the same root", root: fsRoot + sep, key: "obj/a.zip", want: filepath.Join(fsRoot, "obj", "a.zip")},
		{name: "filesystem root rejects parent traversal", root: fsRoot, key: "../a.zip", wantErr: errUtils.ErrPathTraversal},
		{name: "filesystem root rejects nested traversal", root: fsRoot, key: "obj/../../a.zip", wantErr: errUtils.ErrPathTraversal},
		{name: "filesystem root rejects absolute key", root: fsRoot, key: "/etc/passwd", wantErr: errUtils.ErrPathTraversal},
		{name: "nested root accepts a normal key", root: nested, key: "obj/a.zip", want: filepath.Join(nested, "obj", "a.zip")},
		{name: "nested root with trailing separator accepts a normal key", root: nested + sep, key: "obj/a.zip", want: filepath.Join(nested, "obj", "a.zip")},
		{name: "nested root rejects traversal", root: nested, key: "../cache2/a.zip", wantErr: errUtils.ErrPathTraversal},
		{name: "empty key is rejected", root: nested, key: "", wantErr: errUtils.ErrCacheKeyRequired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewFileStore(tc.root).objectPath(tc.key)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			// A containment prefix check must keep the result strictly inside the root.
			rel, relErr := filepath.Rel(filepath.Clean(tc.root), got)
			require.NoError(t, relErr)
			assert.True(t, filepath.IsLocal(rel), "%q must be inside %q", got, tc.root)
		})
	}
}

func TestFileStore_FilesystemRootSidecarPath(t *testing.T) {
	fsRoot := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	store := NewFileStore(fsRoot)

	got, err := store.sidecarPath("obj/a.zip")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(fsRoot, "obj", "a.zip")+metadataSuffix, got)

	_, err = store.sidecarPath("../a.zip")
	require.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

func TestFileStore_SidecarHelpersRejectUnsafeKeys(t *testing.T) {
	store := NewFileStore(t.TempDir())

	for _, tc := range unsafeKeys {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, store.writeSidecar(tc.key, Meta{Kind: KindArtifact}), tc.want)

			sc, ok := store.readSidecar(tc.key)
			assert.False(t, ok, "an unsafe key never yields a sidecar")
			assert.Equal(t, sidecar{}, sc)
		})
	}
}

func TestFileStore_OpenMissingKeyIsNotFoundNotTraversal(t *testing.T) {
	store := NewFileStore(t.TempDir())

	rc, _, err := store.Open("obj/missing.zip")
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	require.NotErrorIs(t, err, errUtils.ErrPathTraversal)
	assert.Nil(t, rc)
}
