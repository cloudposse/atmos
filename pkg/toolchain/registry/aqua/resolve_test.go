package aqua

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

// makeIndexServer returns an httptest server that serves the given index YAML at
// /registry.yaml and returns 404 for everything else. ResolveShortName only needs
// the index — it never fetches per-package files.
func makeIndexServer(t *testing.T, indexYAML string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/registry.yaml" {
			if _, err := w.Write([]byte(indexYAML)); err != nil {
				t.Errorf("unexpected write error: %v", err)
			}
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestResolveShortName_Canonical(t *testing.T) {
	// terraform is canonical: binary == repo_name == "terraform" under hashicorp.
	const indexYAML = `packages:
  - type: github_release
    repo_owner: hashicorp
    repo_name: terraform
  - type: github_release
    repo_owner: cloudposse
    repo_name: atmos
`
	srv := makeIndexServer(t, indexYAML)
	ar := newTestAquaRegistry(t, srv.URL)

	owner, repo, err := ar.ResolveShortName("terraform")
	require.NoError(t, err)
	assert.Equal(t, "hashicorp", owner)
	assert.Equal(t, "terraform", repo)
}

func TestResolveShortName_ThreeSegmentBinaryOnly(t *testing.T) {
	// kubectl lives at kubernetes/kubernetes/kubectl — binary differs from repo.
	// The canonical owner/repo is (kubernetes, kubernetes), and the resolver must
	// return that — not (kubernetes, kubectl) as the old URL-probe code did.
	const indexYAML = `packages:
  - type: github_release
    repo_owner: kubernetes
    repo_name: kubernetes
    name: kubernetes/kubernetes/kubectl
  - type: github_release
    repo_owner: openbao
    repo_name: openbao
    name: openbao/openbao/bao
`
	srv := makeIndexServer(t, indexYAML)
	ar := newTestAquaRegistry(t, srv.URL)

	owner, repo, err := ar.ResolveShortName("kubectl")
	require.NoError(t, err)
	assert.Equal(t, "kubernetes", owner)
	assert.Equal(t, "kubernetes", repo)

	owner, repo, err = ar.ResolveShortName("bao")
	require.NoError(t, err)
	assert.Equal(t, "openbao", owner)
	assert.Equal(t, "openbao", repo)
}

func TestResolveShortName_CanonicalBeatsBinaryOnly(t *testing.T) {
	// If a canonical match exists, it must outrank a binary-only match of the same name.
	const indexYAML = `packages:
  - type: github_release
    repo_owner: hashicorp
    repo_name: terraform
  - type: github_release
    repo_owner: someone
    repo_name: tooling
    name: someone/tooling/terraform
`
	srv := makeIndexServer(t, indexYAML)
	ar := newTestAquaRegistry(t, srv.URL)

	owner, repo, err := ar.ResolveShortName("terraform")
	require.NoError(t, err)
	assert.Equal(t, "hashicorp", owner)
	assert.Equal(t, "terraform", repo)
}

func TestResolveShortName_AmbiguousBinaryOnly(t *testing.T) {
	// Two packages where neither is canonical and both have the same binary name.
	const indexYAML = `packages:
  - type: github_release
    repo_owner: orga
    repo_name: bundle-a
    name: orga/bundle-a/foo
  - type: github_release
    repo_owner: orgb
    repo_name: bundle-b
    name: orgb/bundle-b/foo
`
	srv := makeIndexServer(t, indexYAML)
	ar := newTestAquaRegistry(t, srv.URL)

	_, _, err := ar.ResolveShortName("foo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAmbiguousShortName)
	// Both candidates must be surfaced so the user can pick.
	assert.Contains(t, err.Error(), "orga/bundle-a")
	assert.Contains(t, err.Error(), "orgb/bundle-b")
}

func TestResolveShortName_NotFound(t *testing.T) {
	const indexYAML = `packages:
  - type: github_release
    repo_owner: hashicorp
    repo_name: terraform
`
	srv := makeIndexServer(t, indexYAML)
	ar := newTestAquaRegistry(t, srv.URL)

	_, _, err := ar.ResolveShortName("does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrToolNotFound)
}

func TestResolveShortName_EmptyName(t *testing.T) {
	ar := NewAquaRegistry()
	_, _, err := ar.ResolveShortName("")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrInvalidToolSpec)
}

func TestResolveShortName_IndexFetchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "simulated outage", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	ar := newTestAquaRegistry(t, srv.URL)
	_, _, err := ar.ResolveShortName("kubectl")
	require.Error(t, err)
	// Failure wraps ErrToolNotFound so callers can treat it uniformly with "no match".
	assert.True(t, errors.Is(err, registry.ErrToolNotFound),
		"expected wrap of ErrToolNotFound, got %v", err)
}

func TestBinaryFromPath(t *testing.T) {
	cases := []struct {
		name         string
		path         string
		fallbackRepo string
		want         string
	}{
		{"three segments use last", "kubernetes/kubernetes/kubectl", "kubernetes", "kubectl"},
		{"two segments fall back to repo", "hashicorp/terraform", "terraform", "terraform"},
		{"single segment falls back to repo", "terraform", "terraform", "terraform"},
		{"empty path falls back to repo", "", "terraform", "terraform"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, binaryFromPath(tc.path, tc.fallbackRepo))
		})
	}
}

func TestSplitOwnerRepo(t *testing.T) {
	cases := []struct {
		key       string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{"hashicorp/terraform", "hashicorp", "terraform", true},
		{"kubernetes/kubernetes", "kubernetes", "kubernetes", true},
		{"missing-slash", "", "", false},
		{"/empty-owner", "", "", false},
		{"empty-repo/", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			o, r, ok := splitOwnerRepo(tc.key)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantOwner, o)
			assert.Equal(t, tc.wantRepo, r)
		})
	}
}

// TestResolveShortName_MonorepoAlias verifies the load-bearing fix: aqua's per-package
// `aliases` field is parsed and indexed, so monorepo binaries (kubectl, kubeadm, …)
// resolve through their 2-segment alias instead of colliding on the canonical
// owner/repo. Without alias expansion, the 11 kubernetes/kubernetes binaries all
// share one pathIndex key and only one survives — breaking install for the others.
func TestResolveShortName_MonorepoAlias(t *testing.T) {
	const indexYAML = `packages:
  - type: github_release
    repo_owner: kubernetes
    repo_name: kubernetes
    name: kubernetes/kubernetes/kubectl
    aliases:
      - name: kubernetes/kubectl
  - type: github_release
    repo_owner: kubernetes
    repo_name: kubernetes
    name: kubernetes/kubernetes/kubeadm
    aliases:
      - name: kubernetes/kubeadm
  - type: github_release
    repo_owner: kubernetes
    repo_name: kubernetes
    name: kubernetes/kubernetes/kubelet
`
	srv := makeIndexServer(t, indexYAML)
	ar := newTestAquaRegistry(t, srv.URL)

	// kubectl has an alias → resolves to the 2-segment alias form, which is
	// addressable in pathIndex without colliding with the other monorepo siblings.
	owner, repo, err := ar.ResolveShortName("kubectl")
	require.NoError(t, err)
	assert.Equal(t, "kubernetes", owner)
	assert.Equal(t, "kubectl", repo)

	owner, repo, err = ar.ResolveShortName("kubeadm")
	require.NoError(t, err)
	assert.Equal(t, "kubernetes", owner)
	assert.Equal(t, "kubeadm", repo)

	// kubelet has no alias → falls back to the canonical (owner, repo). GetTool
	// will pick this entry from pathIndex when asked for kubernetes/kubernetes, but
	// note that monorepo collisions on the un-aliased entries are by design — users
	// of those tools need the full 3-segment form.
	owner, repo, err = ar.ResolveShortName("kubelet")
	require.NoError(t, err)
	assert.Equal(t, "kubernetes", owner)
	assert.Equal(t, "kubernetes", repo)
}

// TestResolveShortName_ErrorMessage smoke-checks the ambiguity error mentions the
// "owner/repo" remediation hint so users know what to do.
func TestResolveShortName_ErrorMessage(t *testing.T) {
	const indexYAML = `packages:
  - type: github_release
    repo_owner: a
    repo_name: x
    name: a/x/foo
  - type: github_release
    repo_owner: b
    repo_name: y
    name: b/y/foo
`
	srv := makeIndexServer(t, indexYAML)
	ar := newTestAquaRegistry(t, srv.URL)

	_, _, err := ar.ResolveShortName("foo")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "owner/repo"),
		"ambiguity error should suggest the full owner/repo form, got %q", err.Error())
}
