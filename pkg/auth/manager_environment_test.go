package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// sep is the OS-specific path list separator (: on Unix, ; on Windows).
var sep = string(os.PathListSeparator)

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
	pathA := filepath.Join("path", "a")
	pathB := filepath.Join("path", "b")
	base := map[string]string{"KUBECONFIG": pathA}
	additions := map[string]string{"KUBECONFIG": pathB}
	result := composeEnvironmentVariables(base, additions)
	assert.Equal(t, pathA+sep+pathB, result["KUBECONFIG"])
}

func TestComposeEnvironmentVariables_KubeconfigDedup(t *testing.T) {
	pathA := filepath.Join("path", "a")
	base := map[string]string{"KUBECONFIG": pathA}
	additions := map[string]string{"KUBECONFIG": pathA}
	result := composeEnvironmentVariables(base, additions)
	assert.Equal(t, pathA, result["KUBECONFIG"])
}

func TestComposeEnvironmentVariables_KubeconfigEmptyBase(t *testing.T) {
	pathA := filepath.Join("path", "a")
	base := map[string]string{}
	additions := map[string]string{"KUBECONFIG": pathA}
	result := composeEnvironmentVariables(base, additions)
	assert.Equal(t, pathA, result["KUBECONFIG"])
}

func TestComposeEnvironmentVariables_KubeConfigPathAppend(t *testing.T) {
	pathA := filepath.Join("path", "a")
	pathB := filepath.Join("path", "b")
	base := map[string]string{"KUBE_CONFIG_PATH": pathA}
	additions := map[string]string{"KUBE_CONFIG_PATH": pathB}
	result := composeEnvironmentVariables(base, additions)
	assert.Equal(t, pathA+sep+pathB, result["KUBE_CONFIG_PATH"])
}

func TestComposeEnvironmentVariables_KubeConfigPathDedup(t *testing.T) {
	pathA := filepath.Join("path", "a")
	base := map[string]string{"KUBE_CONFIG_PATH": pathA}
	additions := map[string]string{"KUBE_CONFIG_PATH": pathA}
	result := composeEnvironmentVariables(base, additions)
	assert.Equal(t, pathA, result["KUBE_CONFIG_PATH"])
}

func TestAppendPathList_EmptyExisting(t *testing.T) {
	newPath := filepath.Join("new", "path")
	result := appendPathList("", newPath)
	assert.Equal(t, newPath, result)
}

func TestAppendPathList_EmptyNew(t *testing.T) {
	existing := filepath.Join("existing", "path")
	result := appendPathList(existing, "")
	assert.Equal(t, existing, result)
}

func TestAppendPathList_Append(t *testing.T) {
	pathA := filepath.Join("path", "a")
	pathB := filepath.Join("path", "b")
	result := appendPathList(pathA, pathB)
	assert.Equal(t, pathA+sep+pathB, result)
}

func TestAppendPathList_Dedup(t *testing.T) {
	pathA := filepath.Join("path", "a")
	pathB := filepath.Join("path", "b")
	result := appendPathList(pathA+sep+pathB, pathA)
	assert.Equal(t, pathA+sep+pathB, result)
}

func TestAppendPathList_DedupMiddle(t *testing.T) {
	pathA := filepath.Join("path", "a")
	pathB := filepath.Join("path", "b")
	pathC := filepath.Join("path", "c")
	result := appendPathList(pathA+sep+pathB+sep+pathC, pathB)
	assert.Equal(t, pathA+sep+pathB+sep+pathC, result)
}
