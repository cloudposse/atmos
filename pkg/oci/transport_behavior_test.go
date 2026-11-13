//nolint:gocritic // Descriptor-by-value parameters mirror the ORAS content contracts.
package oci_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/oci"
)

func memoryClient(t *testing.T, host, username, password string) *oci.Client {
	t.Helper()
	store, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	chain := oci.CredentialKeychain{Store: store}
	require.NoError(t, chain.Login(host, &authn.AuthConfig{Username: username, Password: password}))
	return oci.NewClient(nil, oci.WithKeychain(chain))
}

// tokenRegistry fronts a registry with a bearer-token service. Only the identity "alice" with the
// password "alice-secret" receives a token; the token is valid for everybody who presents it, which
// is exactly what makes a shared token cache dangerous.
type tokenRegistry struct {
	*httptest.Server
	host          string
	tokenRequests map[string]*atomic.Int32
}

func newTokenRegistry(t *testing.T) *tokenRegistry {
	t.Helper()
	handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	result := &tokenRegistry{tokenRequests: map[string]*atomic.Int32{"alice": {}, "mallory": {}}}
	result.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			user, password, _ := r.BasicAuth()
			if counter := result.tokenRequests[user]; counter != nil {
				counter.Add(1)
			}
			if user != "alice" || password != "alice-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"token": "alice-token"}))
			return
		}
		if r.Header.Get("Authorization") != "Bearer alice-token" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+result.URL+`/token",service="fixture"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(result.Close)
	result.host = strings.TrimPrefix(result.URL, "http://")
	return result
}

func TestClientsDoNotShareCredentialCachesAcrossIdentities(t *testing.T) {
	ctx := t.Context()
	server := newTokenRegistry(t)
	alice := memoryClient(t, server.host, "alice", "alice-secret")
	mallory := memoryClient(t, server.host, "mallory", "guess")
	repository := oci.Repository{Registry: server.host, Path: "acme/plugin"}
	ref, err := name.NewTag(server.host + "/acme/plugin:1.0.0")
	require.NoError(t, err)
	require.NoError(t, alice.WriteIndex(ctx, ref, testIndex(t)))

	// Alice's token is valid and cached while Mallory acts; Mallory must still be challenged and denied.
	aliceStore, err := alice.Repository(ctx, repository)
	require.NoError(t, err)
	_, err = aliceStore.Resolve(ctx, "1.0.0")
	require.NoError(t, err)
	_, err = mallory.Head(ctx, ref)
	require.ErrorIs(t, err, errUtils.ErrPullImage)
	assert.Positive(t, server.tokenRequests["mallory"].Load(), "the second identity must authenticate on its own")

	// Reopening the same repository with Alice again must not pick up anything of Mallory's.
	_, err = alice.Head(ctx, ref)
	require.NoError(t, err)
}

// flakyRegistry wraps a registry and lets a test intercept requests.
type flakyRegistry struct {
	*httptest.Server
	host     string
	mu       sync.Mutex
	requests []string
}

func newFlakyRegistry(t *testing.T, intercept func(w http.ResponseWriter, r *http.Request, serve http.Handler) bool) *flakyRegistry {
	t.Helper()
	handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	result := &flakyRegistry{}
	result.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result.mu.Lock()
		result.requests = append(result.requests, r.Method+" "+r.URL.Path)
		result.mu.Unlock()
		if intercept(w, r, handler) {
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(result.Close)
	result.host = strings.TrimPrefix(result.URL, "http://")
	return result
}

// duplicates returns requests that were sent more than once, which would be a mutation retry.
func (f *flakyRegistry) duplicates(method string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := map[string]bool{}
	var repeated []string
	for _, request := range f.requests {
		if !strings.HasPrefix(request, method+" ") {
			continue
		}
		if seen[request] {
			repeated = append(repeated, request)
		}
		seen[request] = true
	}
	return repeated
}

func (f *flakyRegistry) count(method, fragment string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, request := range f.requests {
		if strings.HasPrefix(request, method+" ") && strings.Contains(request, fragment) {
			total++
		}
	}
	return total
}

func TestAmbiguousWriteIsReconciledWithoutRetryingTheMutation(t *testing.T) {
	// The registry commits the manifest and then fails the response: the client cannot know.
	server := newFlakyRegistry(t, func(w http.ResponseWriter, r *http.Request, serve http.Handler) bool {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/manifests/") {
			serve.ServeHTTP(httptest.NewRecorder(), r)
			http.Error(w, "gateway failure after commit", http.StatusBadGateway)
			return true
		}
		return false
	})
	client := memoryClient(t, server.host, "user", "password")
	ref, err := name.NewTag(server.host + "/acme/plugin:1.0.0")
	require.NoError(t, err)
	index := testIndex(t)
	want, err := index.Digest()
	require.NoError(t, err)

	require.NoError(t, client.WriteIndex(t.Context(), ref, index), "a write that landed must be reported as success")
	assert.Equal(t, 3, server.count(http.MethodPut, "/manifests/"), "two platform manifests and the index are each written exactly once")
	assert.Empty(t, server.duplicates(http.MethodPut))
	head, err := client.Head(t.Context(), ref)
	require.NoError(t, err)
	assert.Equal(t, want, head.Digest)
}

func TestAmbiguousWriteThatDidNotLandIsAnError(t *testing.T) {
	server := newFlakyRegistry(t, func(w http.ResponseWriter, r *http.Request, _ http.Handler) bool {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/manifests/") {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return true
		}
		return false
	})
	client := memoryClient(t, server.host, "user", "password")
	ref, err := name.NewTag(server.host + "/acme/plugin:1.0.0")
	require.NoError(t, err)

	err = client.WriteIndex(t.Context(), ref, testIndex(t))
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
	assert.Empty(t, server.duplicates(http.MethodPut), "a failed mutation is never blindly retried")
	assert.Positive(t, server.count(http.MethodPut, "/manifests/"))
	_, err = client.Head(t.Context(), ref)
	require.Error(t, err)
}

func TestRejectedWriteIsNotReconciledOrRetriedAnonymously(t *testing.T) {
	var anonymous, anonymousAfterRejection atomic.Int32
	var rejected atomic.Bool
	server := newFlakyRegistry(t, func(w http.ResponseWriter, r *http.Request, _ http.Handler) bool {
		if _, _, ok := r.BasicAuth(); !ok {
			anonymous.Add(1)
			if rejected.Load() {
				anonymousAfterRejection.Add(1)
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			w.WriteHeader(http.StatusUnauthorized)
			return true
		}
		if r.Method == http.MethodPut {
			rejected.Store(true)
			w.WriteHeader(http.StatusForbidden)
			return true
		}
		return false
	})
	client := memoryClient(t, server.host, "user", "password")
	ref, err := name.NewTag(server.host + "/acme/plugin:1.0.0")
	require.NoError(t, err)

	err = client.WriteIndex(t.Context(), ref, testIndex(t))
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
	assert.Equal(t, 0, server.count(http.MethodGet, "/manifests/1.0.0")+server.count(http.MethodHead, "/manifests/1.0.0"),
		"a definite rejection must not trigger destination reconciliation")
	assert.Empty(t, server.duplicates(http.MethodPut), "a rejected mutation is not retried")
	assert.Positive(t, anonymous.Load(), "the registry challenged the first request")
	assert.True(t, rejected.Load(), "the authenticated write was rejected")
	assert.Zero(t, anonymousAfterRejection.Load(), "no anonymous fallback after rejection")
}

func TestDeleteIsReconciledByResolvingTheDigest(t *testing.T) {
	server := newFlakyRegistry(t, func(w http.ResponseWriter, r *http.Request, serve http.Handler) bool {
		if r.Method == http.MethodDelete {
			serve.ServeHTTP(httptest.NewRecorder(), r)
			http.Error(w, "gateway failure after commit", http.StatusGatewayTimeout)
			return true
		}
		return false
	})
	client := memoryClient(t, server.host, "user", "password")
	ref, err := name.NewTag(server.host + "/acme/plugin:1.0.0")
	require.NoError(t, err)
	index := testIndex(t)
	require.NoError(t, client.WriteIndex(t.Context(), ref, index))
	digest, err := index.Digest()
	require.NoError(t, err)

	require.NoError(t, client.Delete(t.Context(), ref.Context().Digest(digest.String())))
	assert.Equal(t, 1, server.count(http.MethodDelete, "/manifests/"))
	_, err = client.Head(t.Context(), ref.Context().Digest(digest.String()))
	require.Error(t, err)
}

func TestDeleteDoesNotNeedReadAccessAndOnlyFallsBackOnDeniedReads(t *testing.T) {
	tests := []struct {
		name       string
		readStatus int
		wantDelete int
		wantErr    bool
	}{
		{"read forbidden still deletes", http.StatusForbidden, 1, false},
		{"read unauthorized still deletes", http.StatusUnauthorized, 1, false},
		{"missing digest sends no delete", http.StatusNotFound, 0, true},
		{"server failure sends no delete", http.StatusInternalServerError, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var deny atomic.Bool
			server := newFlakyRegistry(t, func(w http.ResponseWriter, r *http.Request, _ http.Handler) bool {
				if deny.Load() && (r.Method == http.MethodHead || r.Method == http.MethodGet) && strings.Contains(r.URL.Path, "/manifests/") {
					w.WriteHeader(tt.readStatus)
					return true
				}
				return false
			})
			client := memoryClient(t, server.host, "user", "password")
			ref, err := name.NewTag(server.host + "/acme/plugin:1.0.0")
			require.NoError(t, err)
			index := testIndex(t)
			require.NoError(t, client.WriteIndex(t.Context(), ref, index))
			digest, err := index.Digest()
			require.NoError(t, err)

			deny.Store(true)
			err = client.Delete(t.Context(), ref.Context().Digest(digest.String()))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err, "a delete-only credential must reach the DELETE request")
			}
			assert.Equal(t, tt.wantDelete, server.count(http.MethodDelete, "/manifests/"))
		})
	}
}

func TestReadsAreRetriedButCredentialsAreNotDowngraded(t *testing.T) {
	var failures atomic.Int32
	server := newFlakyRegistry(t, func(w http.ResponseWriter, r *http.Request, _ http.Handler) bool {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/tags/list") && failures.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return true
		}
		return false
	})
	client := memoryClient(t, server.host, "user", "password")
	ref, err := name.NewTag(server.host + "/acme/plugin:1.0.0")
	require.NoError(t, err)
	require.NoError(t, client.WriteIndex(t.Context(), ref, testIndex(t)))
	tags, err := client.Tags(t.Context(), ref.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"1.0.0"}, tags)
	assert.Equal(t, 2, server.count(http.MethodGet, "/tags/list"))
}

func TestFetchBytesIsBoundedAndDigestVerified(t *testing.T) {
	ctx := t.Context()
	directory := filepath.Join(t.TempDir(), "layout")
	layout, err := oci.OpenLayout(ctx, directory)
	require.NoError(t, err)
	content := []byte(`{"hello":"world"}`)
	descriptor := oci.Descriptor("application/json", content)
	require.NoError(t, layout.Push(ctx, descriptor, strings.NewReader(string(content))))

	got, err := oci.FetchBytes(ctx, layout, descriptor, int64(len(content)))
	require.NoError(t, err)
	assert.Equal(t, content, got)

	_, err = oci.FetchBytes(ctx, layout, descriptor, int64(len(content))-1)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity, "content larger than the limit is rejected before reading")
	_, err = oci.FetchBytes(ctx, layout, descriptor, 0)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)

	got, err = oci.FetchBytes(ctx, layout, descriptor, math.MaxInt64)
	require.NoError(t, err, "a MaxInt64 limit must not overflow into an empty read")
	assert.Equal(t, content, got)

	wrongSize := descriptor
	wrongSize.Size++
	_, err = oci.FetchBytes(ctx, fetchOverride(t, map[string][]byte{descriptor.Digest.String(): content}), wrongSize, 1<<10)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)

	tampered := fetchOverride(t, map[string][]byte{descriptor.Digest.String(): []byte(`{"hello":"WORLD"}`)})
	_, err = oci.FetchBytes(ctx, tampered, descriptor, 1<<10)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity, "same size, different bytes")

	_, err = oci.FetchBytes(ctx, layout, oci.Descriptor("application/json", []byte("absent")), 1<<10)
	require.True(t, oci.IsNotFound(err))
	require.ErrorIs(t, err, errdef.ErrNotFound)
}

// fetchOverride returns a generated source whose Fetch serves the supplied bytes by digest.
func fetchOverride(t *testing.T, contents map[string][]byte) *oci.MockReadOnlyStore {
	t.Helper()
	store := oci.NewMockReadOnlyStore(gomock.NewController(t))
	store.EXPECT().Fetch(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(contents[target.Digest.String()]))), nil
	}).AnyTimes()
	return store
}

// cancellingSource returns a generated source that delegates to base, cancels the copy after
// cancelAfter fetches, and counts how many nodes were read.
func cancellingSource(t *testing.T, base oci.ReadOnlyStore, cancelAfter int32, cancel context.CancelFunc) (*oci.MockReadOnlyStore, *atomic.Int32) {
	t.Helper()
	var fetches atomic.Int32
	store := oci.NewMockReadOnlyStore(gomock.NewController(t))
	store.EXPECT().Fetch(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
		if fetches.Add(1) == cancelAfter {
			cancel()
		}
		return base.Fetch(ctx, target)
	}).AnyTimes()
	store.EXPECT().Exists(gomock.Any(), gomock.Any()).DoAndReturn(base.Exists).AnyTimes()
	store.EXPECT().Resolve(gomock.Any(), gomock.Any()).DoAndReturn(base.Resolve).AnyTimes()
	return store, &fetches
}

func TestCancellationStopsGraphTraversalAndLeavesNoReference(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "source")
	index := testIndex(t)
	require.NoError(t, oci.SaveIndex(directory, index))
	digest, err := index.Digest()
	require.NoError(t, err)
	base, err := oci.OpenLayoutReadOnly(t.Context(), directory)
	require.NoError(t, err)

	// Establish how many nodes an uninterrupted copy fetches.
	complete, completeFetches := cancellingSource(t, base, -1, nil)
	full, err := oci.OpenLayout(t.Context(), filepath.Join(t.TempDir(), "full"))
	require.NoError(t, err)
	_, err = oci.Copy(t.Context(), oci.Source{Store: complete, Reference: digest.String()}, oci.Destination{Store: full, Reference: "1.0.0"}, oci.WithCopyConcurrency(1))
	require.NoError(t, err)
	total := completeFetches.Load()
	require.Greater(t, total, int32(3))

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	interrupted, interruptedFetches := cancellingSource(t, base, 2, cancel)
	target, err := oci.OpenLayout(t.Context(), filepath.Join(t.TempDir(), "target"))
	require.NoError(t, err)
	_, err = oci.Copy(ctx, oci.Source{Store: interrupted, Reference: digest.String()}, oci.Destination{Store: target, Reference: "1.0.0"}, oci.WithCopyConcurrency(1))
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, interruptedFetches.Load(), total, "traversal must stop once the context ends")
	assert.Empty(t, target.References(), "an interrupted copy must not publish the reference")
	_, err = target.Resolve(t.Context(), "1.0.0")
	require.Error(t, err)

	already, stop := context.WithCancel(t.Context())
	stop()
	_, err = oci.Copy(already, oci.Source{Store: base, Reference: digest.String()}, oci.Destination{Store: target, Reference: "1.0.0"})
	require.ErrorIs(t, err, context.Canceled)
}

// landingDestination returns a generated destination over layout whose reference push reports err
// after (optionally) completing it, and counts the push attempts.
func landingDestination(t *testing.T, layout *oci.Layout, land bool, pushErr error) (*oci.MockRepositoryStore, *atomic.Int32) {
	t.Helper()
	var pushes atomic.Int32
	store := oci.NewMockRepositoryStore(gomock.NewController(t))
	store.EXPECT().Fetch(gomock.Any(), gomock.Any()).DoAndReturn(layout.Fetch).AnyTimes()
	store.EXPECT().Exists(gomock.Any(), gomock.Any()).DoAndReturn(layout.Exists).AnyTimes()
	store.EXPECT().Push(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(layout.Push).AnyTimes()
	store.EXPECT().Resolve(gomock.Any(), gomock.Any()).DoAndReturn(layout.Resolve).AnyTimes()
	store.EXPECT().Tag(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(layout.Tag).AnyTimes()
	store.EXPECT().PushReference(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, expected ocispec.Descriptor, content io.Reader, reference string) error {
			pushes.Add(1)
			if land {
				if err := layout.Push(ctx, expected, content); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
					return err
				}
				if err := layout.Tag(ctx, expected, reference); err != nil {
					return err
				}
			}
			return pushErr
		},
	).AnyTimes()
	return store, &pushes
}

// movingSource returns a generated source whose tag resolves once and then fails, as if it moved.
func movingSource(t *testing.T, base oci.ReadOnlyStore) (*oci.MockReadOnlyStore, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	store := oci.NewMockReadOnlyStore(gomock.NewController(t))
	store.EXPECT().Fetch(gomock.Any(), gomock.Any()).DoAndReturn(base.Fetch).AnyTimes()
	store.EXPECT().Exists(gomock.Any(), gomock.Any()).DoAndReturn(base.Exists).AnyTimes()
	store.EXPECT().Resolve(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, ref string) (ocispec.Descriptor, error) {
		if calls.Add(1) > 1 {
			return ocispec.Descriptor{}, errors.New("source tag moved")
		}
		return base.Resolve(ctx, ref)
	}).AnyTimes()
	return store, &calls
}

func TestCopyReconcilesAmbiguousFailuresAgainstTheDestination(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "source")
	index := testIndex(t)
	require.NoError(t, oci.SaveIndex(directory, index))
	digest, err := index.Digest()
	require.NoError(t, err)
	source, err := oci.OpenLayoutReadOnly(t.Context(), directory)
	require.NoError(t, err)

	rejection := errors.Join(errors.New("rejected"), errcodeResponse(http.StatusForbidden))
	for _, tc := range []struct {
		name    string
		land    bool
		err     error
		wantErr bool
	}{
		{"landed despite unknown failure", true, errors.New("connection reset by peer"), false},
		{"did not land", false, errors.New("connection reset by peer"), true},
		{"landed but definitely rejected", true, rejection, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout, err := oci.OpenLayout(t.Context(), filepath.Join(t.TempDir(), "target"))
			require.NoError(t, err)
			destination, pushes := landingDestination(t, layout, tc.land, tc.err)
			moving, resolves := movingSource(t, source)
			root, err := oci.Copy(t.Context(), oci.Source{Store: moving, Reference: digest.String()}, oci.Destination{Store: destination, Reference: "1.0.0"})
			assert.Equal(t, int32(1), pushes.Load(), "the mutation is attempted exactly once")
			assert.Equal(t, int32(1), resolves.Load(), "reconciliation must use the selected root")
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, digest.String(), root.Digest.String())
		})
	}
}

func TestCopyReconcilesAnAttachmentOnlyOnceItIsRegisteredUnderItsSubject(t *testing.T) {
	ctx := t.Context()
	source, err := oci.OpenLayout(ctx, filepath.Join(t.TempDir(), "source"))
	require.NoError(t, err)
	config := oci.Descriptor(ocispec.MediaTypeEmptyJSON, []byte("{}"))
	require.NoError(t, source.Push(ctx, config, strings.NewReader("{}")))
	subject := oci.Descriptor(ocispec.MediaTypeImageManifest, []byte("subject manifest"))
	raw, err := json.Marshal(ocispec.Manifest{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: "application/vnd.example.signature",
		Config:       config,
		Layers:       []ocispec.Descriptor{},
		Subject:      &subject,
	})
	require.NoError(t, err)
	attachment := oci.Descriptor(ocispec.MediaTypeImageManifest, raw)
	require.NoError(t, source.Push(ctx, attachment, strings.NewReader(string(raw))))
	require.NoError(t, source.Tag(ctx, attachment, "sig"))

	for _, tc := range []struct {
		name      string
		referrers []ocispec.Descriptor
		wantErr   bool
	}{
		{"registered under the subject", []ocispec.Descriptor{attachment}, false},
		{"manifest exists but is not registered", nil, true},
		{"only another attachment is registered", []ocispec.Descriptor{oci.Descriptor(ocispec.MediaTypeImageManifest, []byte("other"))}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			destination := oci.NewMockRepositoryStore(gomock.NewController(t))
			destination.EXPECT().Exists(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
			destination.EXPECT().Push(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, _ ocispec.Descriptor, content io.Reader) error {
					_, err := io.Copy(io.Discard, content)
					return err
				},
			).AnyTimes()
			destination.EXPECT().PushReference(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("connection reset by peer")).AnyTimes()
			destination.EXPECT().Resolve(gomock.Any(), "1.0.0").Return(attachment, nil).AnyTimes()
			destination.EXPECT().Fetch(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader(string(raw))), nil
				},
			).AnyTimes()
			destination.EXPECT().Predecessors(gomock.Any(), subject).Return(tc.referrers, nil).AnyTimes()

			got, err := oci.Copy(ctx, oci.Source{Store: source, Reference: "sig"}, oci.Destination{Store: destination, Reference: "1.0.0"}, oci.WithoutSubjectTraversal())

			if tc.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, attachment.Digest, got.Digest)
		})
	}
}

func errcodeResponse(status int) error {
	return &errcode.ErrorResponse{Method: http.MethodPut, StatusCode: status}
}
