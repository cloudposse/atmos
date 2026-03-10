package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComposeEnvironmentVariables_Empty(t *testing.T) {
	result := composeEnvironmentVariables(nil, nil)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestComposeEnvironmentVariables_BaseOnly(t *testing.T) {
	base := map[string]string{"FOO": "bar"}
	result := composeEnvironmentVariables(base, nil)
	assert.Equal(t, "bar", result["FOO"])
}

func TestComposeEnvironmentVariables_AdditionsOnly(t *testing.T) {
	additions := map[string]string{"FOO": "bar"}
	result := composeEnvironmentVariables(nil, additions)
	assert.Equal(t, "bar", result["FOO"])
}

func TestComposeEnvironmentVariables_LastWriteWins(t *testing.T) {
	base := map[string]string{"FOO": "old"}
	additions := map[string]string{"FOO": "new"}
	result := composeEnvironmentVariables(base, additions)
	assert.Equal(t, "new", result["FOO"])
}

func TestComposeEnvironmentVariables_KubeconfigAppend(t *testing.T) {
	base := map[string]string{"KUBECONFIG": "/path/a"}
	additions := map[string]string{"KUBECONFIG": "/path/b"}
	result := composeEnvironmentVariables(base, additions)
	assert.Equal(t, "/path/a:/path/b", result["KUBECONFIG"])
}

func TestComposeEnvironmentVariables_KubeconfigDedup(t *testing.T) {
	base := map[string]string{"KUBECONFIG": "/path/a"}
	additions := map[string]string{"KUBECONFIG": "/path/a"}
	result := composeEnvironmentVariables(base, additions)
	assert.Equal(t, "/path/a", result["KUBECONFIG"])
}

func TestComposeEnvironmentVariables_KubeconfigEmptyBase(t *testing.T) {
	base := map[string]string{}
	additions := map[string]string{"KUBECONFIG": "/path/a"}
	result := composeEnvironmentVariables(base, additions)
	assert.Equal(t, "/path/a", result["KUBECONFIG"])
}

func TestComposeEnvironmentVariables_KubeConfigPathAppend(t *testing.T) {
	base := map[string]string{"KUBE_CONFIG_PATH": "/path/a"}
	additions := map[string]string{"KUBE_CONFIG_PATH": "/path/b"}
	result := composeEnvironmentVariables(base, additions)
	assert.Equal(t, "/path/a:/path/b", result["KUBE_CONFIG_PATH"])
}

func TestComposeEnvironmentVariables_KubeConfigPathDedup(t *testing.T) {
	base := map[string]string{"KUBE_CONFIG_PATH": "/path/a"}
	additions := map[string]string{"KUBE_CONFIG_PATH": "/path/a"}
	result := composeEnvironmentVariables(base, additions)
	assert.Equal(t, "/path/a", result["KUBE_CONFIG_PATH"])
}

func TestAppendPathList_EmptyExisting(t *testing.T) {
	result := appendPathList("", "/new/path")
	assert.Equal(t, "/new/path", result)
}

func TestAppendPathList_EmptyNew(t *testing.T) {
	result := appendPathList("/existing/path", "")
	assert.Equal(t, "/existing/path", result)
}

func TestAppendPathList_Append(t *testing.T) {
	result := appendPathList("/path/a", "/path/b")
	assert.Equal(t, "/path/a:/path/b", result)
}

func TestAppendPathList_Dedup(t *testing.T) {
	result := appendPathList("/path/a:/path/b", "/path/a")
	assert.Equal(t, "/path/a:/path/b", result)
}

func TestAppendPathList_DedupMiddle(t *testing.T) {
	result := appendPathList("/path/a:/path/b:/path/c", "/path/b")
	assert.Equal(t, "/path/a:/path/b:/path/c", result)
}
