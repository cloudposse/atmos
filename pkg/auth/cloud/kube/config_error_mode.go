package kube

import (
	"fmt"
	"os"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
)

// chdirExecEnv is the exec-plugin env var that carries the working directory
// of the checkout that wrote the entry. It differs between checkouts of the
// same repository, so "error" update mode ignores it when comparing entries.
const chdirExecEnv = "ATMOS_CHDIR"

// checkErrorModeCollisions checks the on-disk kubeconfig (if any) for
// cluster, context and user entries that newConfig would overwrite with
// different content, used by WriteClusterConfig in "error" update mode before
// merging. An existing entry that already equals the new one is not a
// collision, so re-running an integration is idempotent; for users, only the
// ATMOS_CHDIR exec env value is ignored. Returns nil when the file doesn't
// exist, can't be loaded, or has no conflicting entries.
func (m *KubeconfigManager) checkErrorModeCollisions(newConfig *clientcmdapi.Config) error {
	if _, err := os.Stat(m.path); err != nil {
		return nil //nolint:nilerr // No file yet means no collisions; nothing to check.
	}

	existing, loadErr := clientcmd.LoadFromFile(m.path)
	if loadErr != nil {
		return nil //nolint:nilerr // Unreadable existing file: best-effort check, fall through to merge.
	}

	for id, cluster := range newConfig.Clusters {
		if current, exists := existing.Clusters[id]; exists && !clustersEqual(current, cluster) {
			return m.collisionError("cluster", id)
		}
	}
	for name, kubeContext := range newConfig.Contexts {
		if current, exists := existing.Contexts[name]; exists && !contextsEqual(current, kubeContext) {
			return m.collisionError("context", name)
		}
	}
	for name, user := range newConfig.AuthInfos {
		if current, exists := existing.AuthInfos[name]; exists && !authInfosEqualIgnoringChdir(current, user) {
			return m.collisionError("user", name)
		}
	}

	return nil
}

// collisionError reports an existing kubeconfig entry that differs from the one being written.
func (m *KubeconfigManager) collisionError(kind, name string) error {
	return fmt.Errorf("%w: %s %s already exists in %s with different settings", errUtils.ErrKubeconfigMerge, kind, name, m.path)
}

// authInfosEqualIgnoringChdir compares two users like authInfosEqual, except
// that the ATMOS_CHDIR exec env var is ignored on both sides.
func authInfosEqualIgnoringChdir(a, b *clientcmdapi.AuthInfo) bool {
	return authInfosEqual(withoutChdirEnv(a), withoutChdirEnv(b))
}

// withoutChdirEnv returns a shallow copy of user whose exec env has no
// ATMOS_CHDIR entry. The input is not modified.
func withoutChdirEnv(user *clientcmdapi.AuthInfo) *clientcmdapi.AuthInfo {
	if user == nil || user.Exec == nil {
		return user
	}
	exec := *user.Exec
	exec.Env = nil
	for _, env := range user.Exec.Env {
		if env.Name != chdirExecEnv {
			exec.Env = append(exec.Env, env)
		}
	}
	copied := *user
	copied.Exec = &exec
	return &copied
}
