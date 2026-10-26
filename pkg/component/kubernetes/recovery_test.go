package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/policy/recovery/recoverytest"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestApplyRetryDoesNotRepeatSuccessfulObjects(t *testing.T) {
	recoverytest.Install(t, "kubernetes.apply.error", "input.error.message.contains('service unavailable')")
	first := kubernetesObject("v1", "ConfigMap", "first", "default")
	second := kubernetesObject("v1", "ConfigMap", "second", "default")
	client, fake := newFakeSDKClientWithFake()
	attempts := map[string]int{}
	fake.PrependReactor("patch", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
		name := action.(k8stesting.PatchAction).GetName()
		attempts[name]++
		if name == "second" && attempts[name] == 1 {
			return true, nil, apierrors.NewServiceUnavailable("service unavailable")
		}
		return true, kubernetesObject("v1", "ConfigMap", name, "default"), nil
	})
	original := newKubernetesSDKClient
	newKubernetesSDKClient = func() (*sdkClient, error) { return client, nil }
	t.Cleanup(func() { newKubernetesSDKClient = original })
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev", ComponentSection: map[string]any{}}
	results, err := executeKubernetesOperation(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, info, OperationApply, []*unstructured.Unstructured{first, second})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "first", results[0].Name)
	assert.Equal(t, "second", results[1].Name)
	assert.Equal(t, map[string]int{"first": 1, "second": 2}, attempts)
	// A later successful call has a fresh budget and no stale error output.
	_, err = client.Apply(context.Background(), []*unstructured.Unstructured{first})
	require.NoError(t, err)
	assert.Equal(t, 2, attempts["first"])
}

func TestApplyRetryHonorsCallerCancellation(t *testing.T) {
	recoverytest.Install(t, "kubernetes.apply.error", "true")
	client, fake := newFakeSDKClientWithFake()
	original := newKubernetesSDKClient
	newKubernetesSDKClient = func() (*sdkClient, error) { return client, nil }
	t.Cleanup(func() { newKubernetesSDKClient = original })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev"}
	_, err := executeKubernetesOperation(&component.ExecutionContext{Context: ctx}, &schema.AtmosConfiguration{}, info, OperationApply,
		[]*unstructured.Unstructured{kubernetesObject("v1", "ConfigMap", "app", "default")})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, fake.Actions())
}
