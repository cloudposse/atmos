package kube

import (
	"fmt"
	"os"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RemoveClusterConfig removes a context from the kubeconfig together with the
// cluster and users it wrote. Idempotent: returns nil if entries do not exist.
//
// The cluster and each user are removed only when no remaining context still
// references them, so cleaning up one integration never breaks another context
// that shares the cluster entry (for example, two identities for one cluster)
// or a user name written by an older version. extraUserNames lists additional
// users to remove under the same rule, such as a user name in a format that
// older versions wrote.
func (m *KubeconfigManager) RemoveClusterConfig(clusterARN, contextName, userName string, extraUserNames ...string) error {
	defer perf.Track(nil, "kube.KubeconfigManager.RemoveClusterConfig")()

	// If the file doesn't exist, nothing to clean up.
	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		return nil
	}

	existing, err := clientcmd.LoadFromFile(m.path)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrKubeconfigMerge, err)
	}

	// Remove the context first, so references are counted without it.
	delete(existing.Contexts, contextName)
	if !clusterReferenced(existing, clusterARN) {
		delete(existing.Clusters, clusterARN)
	}
	for _, name := range append([]string{userName}, extraUserNames...) {
		if name != "" && !userReferenced(existing, name) {
			delete(existing.AuthInfos, name)
		}
	}

	// If current-context points to the removed context, clear it.
	if existing.CurrentContext == contextName {
		existing.CurrentContext = ""
	}

	// If the config is now empty, remove the file.
	if len(existing.Clusters) == 0 && len(existing.Contexts) == 0 && len(existing.AuthInfos) == 0 {
		return os.Remove(m.path)
	}

	return clientcmd.WriteToFile(*existing, m.path)
}

// LookupContext returns the named context from the kubeconfig, or nil when the
// file or the context does not exist.
func (m *KubeconfigManager) LookupContext(contextName string) (*clientcmdapi.Context, error) {
	defer perf.Track(nil, "kube.KubeconfigManager.LookupContext")()

	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		return nil, nil
	}

	existing, err := clientcmd.LoadFromFile(m.path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrKubeconfigMerge, err)
	}

	return existing.Contexts[contextName], nil
}

// clusterReferenced reports whether any context in config uses the cluster.
func clusterReferenced(config *clientcmdapi.Config, clusterID string) bool {
	for _, kubeContext := range config.Contexts {
		if kubeContext != nil && kubeContext.Cluster == clusterID {
			return true
		}
	}
	return false
}

// userReferenced reports whether any context in config uses the user.
func userReferenced(config *clientcmdapi.Config, userName string) bool {
	for _, kubeContext := range config.Contexts {
		if kubeContext != nil && kubeContext.AuthInfo == userName {
			return true
		}
	}
	return false
}
