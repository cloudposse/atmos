package gcp

import (
	"os"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// appendWorkingDirExecEnv adds ATMOS_CHDIR with the effective working directory
// (after Atmos applied --chdir/ATMOS_CHDIR) to a kubeconfig exec-plugin env, so
// kubectl can invoke Atmos from any directory and it still finds atmos.yaml. This
// mirrors the EKS kubeconfig (#3675). The directory is absolute because os.Getwd
// always returns an absolute path. If it cannot be resolved the entry is omitted:
// kubectl then behaves as before, which requires running it from the project.
func appendWorkingDirExecEnv(env []clientcmdapi.ExecEnvVar) []clientcmdapi.ExecEnvVar {
	workingDir, err := os.Getwd()
	if err != nil {
		log.Debug("Could not resolve the working directory; kubeconfig exec entry will not set ATMOS_CHDIR", "error", err)
		return env
	}
	return append(env, clientcmdapi.ExecEnvVar{Name: "ATMOS_CHDIR", Value: workingDir})
}
