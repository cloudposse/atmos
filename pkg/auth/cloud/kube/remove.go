package kube

import (
	"fmt"
	"os"
	"slices"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RemoveClusterConfig removes a context from the kubeconfig together with the
// cluster and users it wrote. Idempotent: returns nil if entries do not exist.
//
// The context is removed only when it points at userName or one of
// extraUserNames, so cleaning up one integration never removes a context of
// the same name that another realm, identity or tool pointed at its own user.
//
// The cluster and each user are removed only when no remaining context still
// references them, so cleaning up one integration never breaks another context
// that shares the cluster entry (for example, two identities for one cluster)
// or a user name written by an older version. The extraUserNames argument lists additional
// users to remove under the same rule, such as a user name in a format that
// older versions wrote.
func (m *KubeconfigManager) RemoveClusterConfig(clusterARN, contextName, userName string, extraUserNames ...string) error {
	return m.RemoveOwnedClusterConfig(clusterARN, []string{contextName}, append([]string{userName}, extraUserNames...))
}

// RemoveOwnedClusterConfig removes each of contextNames that points at one of
// userNames, then the cluster and each of userNames that no remaining context
// references. A context that points at any other user is left alone, together
// with its cluster and user. Pass ContextNames as contextNames so a context
// written under the older, unprefixed default name is cleaned up too.
// Idempotent: returns nil if entries do not exist. Empty names are ignored.
//
// A context is removed only when it also points at clusterID, so a same-named
// context of another cluster that shares a user name is never removed. With
// WithIdentity, a context or user whose exec plugin authenticates as another
// identity is left alone too.
func (m *KubeconfigManager) RemoveOwnedClusterConfig(clusterID string, contextNames, userNames []string, opts ...RemoveOption) error {
	defer perf.Track(nil, "kube.KubeconfigManager.RemoveOwnedClusterConfig")()

	// If the file doesn't exist, nothing to clean up.
	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		return nil
	}

	existing, err := clientcmd.LoadFromFile(m.path)
	if err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrKubeconfigMerge, err)
	}

	var cfg removeOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	removeOwnedEntries(existing, clusterID, contextNames, userNames, cfg.identity)

	// If the config is now empty, remove the file.
	if len(existing.Clusters) == 0 && len(existing.Contexts) == 0 && len(existing.AuthInfos) == 0 {
		if err := os.Remove(m.path); err != nil {
			return fmt.Errorf(errWrapFormat, errUtils.ErrKubeconfigWrite, err)
		}
		return nil
	}

	if err := clientcmd.WriteToFile(*existing, m.path); err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrKubeconfigWrite, err)
	}
	return nil
}

// RemoveOption configures RemoveOwnedClusterConfig.
type RemoveOption func(*removeOptions)

type removeOptions struct {
	identity string
}

// WithIdentity restricts removal to entries written for identity or for no
// identity: a context or user whose exec plugin authenticates as a different
// identity (its ATMOS_IDENTITY env value) is left alone. Entries that name no
// identity, such as those older versions wrote, still count as owned.
func WithIdentity(identity string) RemoveOption {
	return func(o *removeOptions) { o.identity = identity }
}

// removeOwnedEntries removes the owned contexts from config, then the cluster
// and users that no remaining context references.
func removeOwnedEntries(config *clientcmdapi.Config, clusterID string, contextNames, userNames []string, identity string) {
	// Remove the owned contexts first, so references are counted without them.
	for _, contextName := range contextNames {
		if !ownedContextRemovable(config, contextName, clusterID, userNames, identity) {
			continue
		}
		delete(config.Contexts, contextName)
		// If current-context points to the removed context, clear it.
		if config.CurrentContext == contextName {
			config.CurrentContext = ""
		}
	}
	if !clusterReferenced(config, clusterID) {
		delete(config.Clusters, clusterID)
	}
	for _, name := range userNames {
		if name != "" && !userReferenced(config, name) && userIdentityMatches(config, name, identity) {
			delete(config.AuthInfos, name)
		}
	}
}

// ownedContextRemovable reports whether the named context is one of the owned
// contexts for the cluster: it points at one of userNames, targets clusterID
// and authenticates as identity (or as no identity).
func ownedContextRemovable(config *clientcmdapi.Config, contextName, clusterID string, userNames []string, identity string) bool {
	return contextOwnedBy(config, contextName, userNames) &&
		contextTargets(config, contextName, clusterID) &&
		userIdentityMatches(config, config.Contexts[contextName].AuthInfo, identity)
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
		return nil, fmt.Errorf(errWrapFormat, errUtils.ErrKubeconfigMerge, err)
	}

	return existing.Contexts[contextName], nil
}

// contextOwnedBy reports whether the named context exists in config and points
// at one of userNames.
func contextOwnedBy(config *clientcmdapi.Config, contextName string, userNames []string) bool {
	kubeContext, ok := config.Contexts[contextName]
	return ok && kubeContext != nil && kubeContext.AuthInfo != "" && slices.Contains(userNames, kubeContext.AuthInfo)
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

// contextTargets reports whether the named context points at clusterID.
func contextTargets(config *clientcmdapi.Config, contextName, clusterID string) bool {
	kubeContext := config.Contexts[contextName]
	return kubeContext != nil && kubeContext.Cluster == clusterID
}

// userIdentityMatches reports whether the user may be removed for identity:
// identity is empty (no restriction), or the user's exec plugin names no
// identity or the same one.
func userIdentityMatches(config *clientcmdapi.Config, userName, identity string) bool {
	if identity == "" {
		return true
	}
	user := config.AuthInfos[userName]
	if user == nil || user.Exec == nil {
		return true
	}
	for _, env := range user.Exec.Env {
		if env.Name == identityExecEnv {
			return env.Value == "" || env.Value == identity
		}
	}
	return true
}
