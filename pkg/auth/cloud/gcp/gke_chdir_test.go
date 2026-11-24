package gcp

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
)

// TestBuildKubeClusterInfo_WorkingDirectory mirrors the EKS fix (#3675): the
// kubeconfig exec entry must carry the effective project directory as an absolute
// ATMOS_CHDIR, so kubectl run from any other directory still finds atmos.yaml.
func TestBuildKubeClusterInfo_WorkingDirectory(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project with spaces")
	require.NoError(t, os.Mkdir(projectDir, 0o700))
	t.Chdir(projectDir)
	t.Setenv("ATMOS_CHDIR", "relative-original-value")
	workingDir, err := os.Getwd()
	require.NoError(t, err)

	info := BuildKubeClusterInfo(&GKEClusterInfo{
		Name:                     "example-cluster",
		ProjectID:                "example-project",
		Location:                 "us-central1",
		ID:                       ClusterResourceName("example-project", "us-central1", "example-cluster"),
		Endpoint:                 "https://gke.example.invalid",
		CertificateAuthorityData: testCAData,
	}, "example-deployer")
	path := filepath.Join(t.TempDir(), "config")
	mgr, err := kube.NewKubeconfigManager(path, "")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(info, "", "merge")
	require.NoError(t, err)

	// Reading from an unrelated directory must retain the original project directory.
	t.Chdir(t.TempDir())
	config, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	user := config.AuthInfos[kube.UserName(info)]
	require.NotNil(t, user)
	require.NotNil(t, user.Exec)
	env := map[string]string{}
	for _, entry := range user.Exec.Env {
		env[entry.Name] = entry.Value
	}
	assert.Equal(t, workingDir, env["ATMOS_CHDIR"])
	assert.True(t, filepath.IsAbs(env["ATMOS_CHDIR"]))
	assert.Equal(t, "example-deployer", env["ATMOS_IDENTITY"])
}

// TestAppendWorkingDirExecEnv_WorkingDirectoryUnavailable verifies the entry is
// omitted (not an empty or relative ATMOS_CHDIR) when the directory is gone.
func TestAppendWorkingDirExecEnv_WorkingDirectoryUnavailable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Only Linux reliably fails Getwd after removing the current directory")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.Remove(dir))

	env := appendWorkingDirExecEnv([]clientcmdapi.ExecEnvVar{{Name: "ATMOS_IDENTITY", Value: "dev"}})

	assert.Equal(t, []clientcmdapi.ExecEnvVar{{Name: "ATMOS_IDENTITY", Value: "dev"}}, env)
}
