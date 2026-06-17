package kubernetes

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestResolveValidateOptions(t *testing.T) {
	assert.Equal(t, validateOptions{}, resolveValidateOptions(nil))
	assert.Equal(t, validateOptions{}, resolveValidateOptions(map[string]any{"server": false}))
	assert.Equal(t, validateOptions{Server: true}, resolveValidateOptions(map[string]any{"server": true}))
}

func TestValidateObjectsStructuralPasses(t *testing.T) {
	objects := []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", ""),
		kubernetesObject("v1", "Service", "app", "default"),
	}

	require.NoError(t, validateObjectsStructural(objects))
}

func TestValidateObjectsStructuralReportsAllFailures(t *testing.T) {
	objects := []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", ""),                   // valid
		kubernetesObject("v1", "ConfigMap", "", ""),                           // missing name
		kubernetesObject("v1", "Service", "Bad_Name", ""),                     // invalid DNS-1123 name
		{Object: map[string]any{"metadata": map[string]any{"name": "thing"}}}, // missing GVK
	}

	err := validateObjectsStructural(objects)
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrKubernetesValidationFailed)

	// Every invalid object must be reported, not just the first.
	assert.ErrorContains(t, err, "is missing metadata.name")
	assert.ErrorContains(t, err, "not a valid DNS-1123 subdomain")
	assert.ErrorContains(t, err, "missing group/version/kind")
}

func TestRunValidateOfflinePasses(t *testing.T) {
	original := newKubernetesSDKClient
	t.Cleanup(func() { newKubernetesSDKClient = original })

	// Offline validation must never construct a cluster client.
	newKubernetesSDKClient = func() (*sdkClient, error) {
		t.Fatal("offline validate must not contact the cluster")
		return nil, nil
	}

	objects := []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", ""),
	}
	results, err := runValidate(objects, validateOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "valid", results[0].Action)
}

func TestRunValidateOfflineFails(t *testing.T) {
	objects := []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "", ""),
	}

	_, err := runValidate(objects, validateOptions{})
	require.ErrorIs(t, err, errUtils.ErrKubernetesValidationFailed)
}

func TestRunValidateServerReturnsClientInitError(t *testing.T) {
	original := newKubernetesSDKClient
	t.Cleanup(func() { newKubernetesSDKClient = original })

	newKubernetesSDKClient = func() (*sdkClient, error) {
		return nil, errors.New("client failed")
	}

	objects := []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", ""),
	}
	_, err := runValidate(objects, validateOptions{Server: true})
	require.ErrorContains(t, err, "client failed")
}

func TestRunValidateServerAggregatesDryRunErrors(t *testing.T) {
	original := newKubernetesSDKClient
	t.Cleanup(func() { newKubernetesSDKClient = original })

	// The fake dynamic client does not support server-side apply, so the
	// dry-run patch fails — exercising the aggregate failure path.
	newKubernetesSDKClient = func() (*sdkClient, error) {
		return newFakeSDKClient(), nil
	}

	objects := []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", "default"),
	}
	_, err := runValidate(objects, validateOptions{Server: true})
	require.ErrorIs(t, err, errUtils.ErrKubernetesValidationFailed)
}

func TestSDKClientValidateAggregatesPerObjectErrors(t *testing.T) {
	client := newFakeSDKClient()

	results, err := client.Validate(context.Background(), []*unstructured.Unstructured{
		kubernetesObject("v1", "ConfigMap", "settings", "default"),
		kubernetesObject("v1", "ConfigMap", "other", "default"),
	})

	require.Error(t, err)
	assert.Empty(t, results)
	// Both objects are reported in a single aggregate error.
	assert.ErrorContains(t, err, "ConfigMap/settings")
	assert.ErrorContains(t, err, "ConfigMap/other")
}
