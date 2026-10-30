package publicaccess

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestVisibility(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       bool
	}{
		{"public", `{"visibility":"public"}`, 200, false},
		{"private", `{"visibility":"private"}`, 200, true},
		{"missing", `{}`, 404, true},
		{"malformed", `{`, 200, true},
		{"oversized", strings.Repeat(" ", maxManifest+1), 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewMockHTTPClient(gomock.NewController(t))
			client.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, "/orgs/example/packages/container/atmos%2Fplugins%2Fhello", r.URL.EscapedPath())
				require.Equal(t, "Bearer metadata-only", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			err := New(WithHTTPClient(client), WithGitHubToken("metadata-only")).RequirePublic(t.Context(), "oci://ghcr.io/example/atmos/plugins/hello")
			if tc.want {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
	client := NewMockHTTPClient(gomock.NewController(t))
	client.EXPECT().Do(gomock.Any()).Return(nil, context.Canceled)
	require.ErrorIs(t, New(WithHTTPClient(client)).RequirePublic(t.Context(), "ghcr.io/example/plugin"), context.Canceled)
	for _, source := range []string{"invalid reference", "localhost:5000/example/plugin", "ghcr.io/plugin"} {
		require.Error(t, New().RequirePublic(t.Context(), source))
	}
}

func TestVerifyCompleteAnonymousGraph(t *testing.T) {
	backend := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			require.Empty(t, r.Header.Get("Authorization"))
			requested = append(requested, r.URL.Path)
		}
		backend.ServeHTTP(w, r)
	}))
	defer server.Close()
	ref, err := name.NewTag(server.Listener.Addr().String() + "/example/plugin:1.0.0")
	require.NoError(t, err)
	first, err := mutate.AppendLayers(empty.Image, static.NewLayer([]byte("linux"), types.OCILayer))
	require.NoError(t, err)
	second, err := mutate.AppendLayers(empty.Image, static.NewLayer([]byte("windows"), types.OCILayer))
	require.NoError(t, err)
	index := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: first}, mutate.IndexAddendum{Add: second})
	require.NoError(t, remote.WriteIndex(ref, index))
	hash, err := index.Digest()
	require.NoError(t, err)
	requested = nil
	client := New(WithGitHubToken("must-not-reach-registry"), WithTransport(http.DefaultTransport))
	actual, err := client.Verify(t.Context(), ref.Context().Name(), "1.0.0", hash.String())
	require.NoError(t, err)
	require.Equal(t, hash.String(), actual)
	require.GreaterOrEqual(t, len(requested), 7, "both platform images and all their config/layer blobs must be read")
	_, err = client.Verify(t.Context(), ref.Context().Name(), hash.String(), "")
	require.NoError(t, err)
	_, err = client.Verify(t.Context(), ref.Context().Name(), "1.0.0", "sha256:"+strings.Repeat("f", 64))
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	_, err = client.Verify(t.Context(), ref.Context().Name(), "missing", "")
	require.Error(t, err)
	for _, tc := range [][3]string{{"invalid repository", "tag", ""}, {ref.Context().Name(), "", ""}, {ref.Context().Name(), "invalid tag", ""}, {ref.Context().Name(), "1.0.0", "bad"}} {
		_, err = client.Verify(t.Context(), tc[0], tc[1], tc[2])
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = client.Verify(ctx, ref.Context().Name(), "1.0.0", "")
	require.Error(t, err)
}

func TestGraphRejectsUntrustedContent(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		size       int64
		blob       bool
		expected   string
	}{
		{"json", "{", -1, false, ""},
		{"empty", "{}", -1, false, ""},
		{"bad child", `{"manifests":[{"digest":"sha256:a","size":1}]}`, -1, false, ""},
		{"negative child", `{"manifests":[{"digest":"sha256:` + strings.Repeat("a", 64) + `","size":-1}]}`, -1, false, ""},
		{"wrong size", "ok", 3, true, ""},
		{"wrong hash", "ok", -1, true, "sha256:" + strings.Repeat("a", 64)},
		{"oversized", "", maxBlob + 1, true, ""},
		{"invalid size", "", -2, true, ""},
		{"mixed", `{"manifests":[{}],"config":{}}`, -1, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := NewMockHTTPClient(gomock.NewController(t))
			if tc.size <= maxBlob && tc.size >= -1 {
				mock.EXPECT().Do(gomock.Any()).Return(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil)
			}
			repo, err := name.NewRepository("ghcr.io/example/plugin")
			require.NoError(t, err)
			g := graph{ctx: t.Context(), client: mock, repository: repo, seen: map[string]int64{}}
			_, err = g.pull("1.0.0", tc.blob, tc.expected, tc.size)
			require.Error(t, err)
		})
	}
	mock := NewMockHTTPClient(gomock.NewController(t))
	mock.EXPECT().Do(gomock.Any()).Return(nil, context.Canceled)
	repo, err := name.NewRepository("ghcr.io/example/plugin")
	require.NoError(t, err)
	g := graph{ctx: t.Context(), client: mock, repository: repo, seen: map[string]int64{}}
	_, err = g.pull("tag", false, "", -1)
	require.ErrorIs(t, err, context.Canceled)
	g.seen["blobs/duplicate"] = 2
	_, err = g.pull("duplicate", true, "", 3)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity, "duplicate digest references must agree on size")
	for i := range maxObjects {
		g.seen[string(rune(i))] = 0
	}
	_, err = g.pull("another", false, "", -1)
	require.ErrorIs(t, err, errUtils.ErrPluginPayloadTooLarge)
	_, err = readBounded(strings.NewReader("large"), 1)
	require.ErrorIs(t, err, errUtils.ErrPluginPayloadTooLarge)
}
