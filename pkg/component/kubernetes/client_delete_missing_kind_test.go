package kubernetes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"

	errUtils "github.com/cloudposse/atmos/errors"
)

// newDiscoveryTestClient returns a real sdkClient (real discovery and RESTMapper)
// pointed at a fake API server. The server serves core/v1 ConfigMaps, advertises
// the down.example.com group but fails its discovery, advertises the
// moved.example.com group whose v1 serves Gadget, and returns 404 for
// everything else.
func newDiscoveryTestClient(t *testing.T) *sdkClient {
	t.Helper()

	responses := map[string]string{
		"/api": `{"kind":"APIVersions","versions":["v1"]}`,
		"/apis": `{"kind":"APIGroupList","apiVersion":"v1","groups":[{"name":"down.example.com",` +
			`"versions":[{"groupVersion":"down.example.com/v1","version":"v1"}],` +
			`"preferredVersion":{"groupVersion":"down.example.com/v1","version":"v1"}},` +
			`{"name":"moved.example.com","versions":[{"groupVersion":"moved.example.com/v1","version":"v1"}],` +
			`"preferredVersion":{"groupVersion":"moved.example.com/v1","version":"v1"}}]}`,
		"/apis/moved.example.com/v1": `{"kind":"APIResourceList","groupVersion":"moved.example.com/v1","resources":[` +
			`{"name":"gadgets","namespaced":true,"kind":"Gadget","verbs":["delete","get"]}]}`,
		"/api/v1": `{"kind":"APIResourceList","groupVersion":"v1","resources":[` +
			`{"name":"configmaps","namespaced":true,"kind":"ConfigMap","verbs":["delete","get"]}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/apis/down.example.com/v1" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		body, ok := responses[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			body = `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfig, []byte(`apiVersion: v1
kind: Config
current-context: test
clusters: [{name: test, cluster: {server: "`+server.URL+`"}}]
users: [{name: test, user: {}}]
contexts: [{name: test, context: {cluster: test, user: test}}]
`), filePerm))
	t.Setenv("KUBECONFIG", kubeconfig)

	client, err := newSDKClient()
	require.NoError(t, err)
	return client
}

func TestSDKClientDeleteTreatsUnservedKindAsNotFound(t *testing.T) {
	client := newDiscoveryTestClient(t)

	results, err := client.Delete(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("gone.example.com/v1", "Widget", "first", "apps"),
		kubernetesObject("v1", "Widget", "second", ""),
		kubernetesObject("v1", "ConfigMap", "settings", ""),
	})

	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, objectResult{Action: "not-found", Resource: "gone.example.com/v1/Widget", Namespace: "apps", Name: "first"}, results[0])
	assert.Equal(t, "not-found", results[1].Action)
	assert.Equal(t, "settings", results[2].Name)
}

func TestSDKClientDeleteFailsWhenKindDiscoveryFails(t *testing.T) {
	client := newDiscoveryTestClient(t)

	_, err := client.Delete(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("down.example.com/v1", "Widget", "first", "apps"),
	})

	require.ErrorIs(t, err, errUtils.ErrKubernetesResolveResource)
}

func TestSDKClientDiffStillFailsOnUnservedKind(t *testing.T) {
	client := newDiscoveryTestClient(t)

	_, err := client.Diff(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("gone.example.com/v1", "Widget", "first", "apps"),
	})

	require.ErrorIs(t, err, errUtils.ErrKubernetesResolveResource)
}

func TestSDKClientDeleteFailsWhenAnotherVersionServesKind(t *testing.T) {
	client := newDiscoveryTestClient(t)

	// The loader accepts any casing of the kind, so the discovery match must too.
	for _, kind := range []string{"Gadget", "gadget"} {
		_, err := client.Delete(context.Background(), []*unstructured.Unstructured{
			kubernetesObject("moved.example.com/v1beta1", kind, "first", "apps"),
		})

		require.ErrorIs(t, err, errUtils.ErrKubernetesResolveResource, kind)
	}
}

func TestSDKClientKindUnservedNeedsConfirmation(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "gone.example.com", Version: "v1", Kind: "Widget"}

	assert.False(t, (&sdkClient{}).kindUnserved(context.Background(), gvk), "no discovery client")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)

	assert.False(t, (&sdkClient{discovery: discoveryClient}).kindUnserved(context.Background(), gvk), "group list fails")
}
