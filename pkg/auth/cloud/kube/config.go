package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"k8s.io/client-go/tools/clientcmd"
)

const (
	// defaultFileMode is the default kubeconfig file permission.
	defaultFileMode = 0o600

	// defaultDirMode is the default kubeconfig directory permission.
	defaultDirMode = 0o700

	// defaultUpdateMode is the default kubeconfig update strategy.
	defaultUpdateMode = "merge"

	// execAPIVersion is the Kubernetes exec credential plugin API version.
	execAPIVersion = "client.authentication.k8s.io/v1beta1"

	// atmosCommand is the command used in the exec credential plugin.
	atmosCommand = "atmos"
)

// KubeconfigManager manages kubeconfig files for EKS clusters.
type KubeconfigManager struct {
	path string
	mode os.FileMode
}

// NewKubeconfigManager creates a manager with the given path and permissions.
// If customPath is empty, uses XDG default (~/.config/atmos/kube/config).
// If modeStr is empty, defaults to "0600".
func NewKubeconfigManager(customPath, modeStr string) (*KubeconfigManager, error) {
	defer perf.Track(nil, "kube.NewKubeconfigManager")()

	// Resolve path.
	path := customPath
	if path == "" {
		defaultPath, err := DefaultKubeconfigPath()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrKubeconfigPath, err)
		}
		path = defaultPath
	}

	// Parse file mode.
	mode := os.FileMode(defaultFileMode)
	if modeStr != "" {
		parsed, err := strconv.ParseUint(modeStr, 8, 32)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid mode %q: %w", errUtils.ErrKubeconfigWrite, modeStr, err)
		}
		mode = os.FileMode(parsed)
	}

	return &KubeconfigManager{
		path: path,
		mode: mode,
	}, nil
}

// GetPath returns the kubeconfig file path.
func (m *KubeconfigManager) GetPath() string {
	return m.path
}

// WriteClusterConfig generates and writes kubeconfig for an EKS cluster.
func (m *KubeconfigManager) WriteClusterConfig(info *awsCloud.EKSClusterInfo, alias, identityName, updateMode string) error {
	defer perf.Track(nil, "kube.KubeconfigManager.WriteClusterConfig")()

	if updateMode == "" {
		updateMode = defaultUpdateMode
	}

	// Build the kubeconfig for this cluster.
	newConfig := BuildClusterConfig(info, alias, identityName)

	// Ensure parent directory exists.
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, defaultDirMode); err != nil {
		return fmt.Errorf("%w: failed to create directory %s: %w", errUtils.ErrKubeconfigWrite, dir, err)
	}

	switch updateMode {
	case "replace":
		return m.writeConfig(newConfig)

	case "error":
		if _, err := os.Stat(m.path); err == nil {
			// File exists, check if cluster is already configured.
			existing, loadErr := clientcmd.LoadFromFile(m.path)
			if loadErr == nil {
				if _, exists := existing.Clusters[info.ARN]; exists {
					return fmt.Errorf("%w: cluster %s already exists in %s", errUtils.ErrKubeconfigMerge, info.ARN, m.path)
				}
			}
		}
		return m.mergeConfig(newConfig)

	default: // "merge"
		return m.mergeConfig(newConfig)
	}
}

// RemoveClusterConfig removes a cluster, context, and user from the kubeconfig.
// Idempotent: returns nil if entries do not exist.
func (m *KubeconfigManager) RemoveClusterConfig(clusterARN, contextName, userName string) error {
	defer perf.Track(nil, "kube.KubeconfigManager.RemoveClusterConfig")()

	// If the file doesn't exist, nothing to clean up.
	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		return nil
	}

	existing, err := clientcmd.LoadFromFile(m.path)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrKubeconfigMerge, err)
	}

	// Remove entries.
	delete(existing.Clusters, clusterARN)
	delete(existing.Contexts, contextName)
	delete(existing.AuthInfos, userName)

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

// ListClusterARNs returns all cluster ARN keys from the kubeconfig file.
// Returns nil if the file does not exist.
func (m *KubeconfigManager) ListClusterARNs() ([]string, error) {
	defer perf.Track(nil, "kube.KubeconfigManager.ListClusterARNs")()

	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		return nil, nil
	}

	existing, err := clientcmd.LoadFromFile(m.path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrKubeconfigMerge, err)
	}

	arns := make([]string, 0, len(existing.Clusters))
	for k := range existing.Clusters {
		arns = append(arns, k)
	}

	return arns, nil
}

// BuildClusterConfig creates a kubeconfig api.Config for a single EKS cluster.
func BuildClusterConfig(info *awsCloud.EKSClusterInfo, alias, identityName string) *clientcmdapi.Config {
	defer perf.Track(nil, "kube.BuildClusterConfig")()

	// Context name defaults to cluster ARN, or alias if provided.
	contextName := info.ARN
	if alias != "" {
		contextName = alias
	}

	// User name derived from cluster name.
	userName := "user-" + info.Name

	// Build exec plugin env vars.
	execEnv := []clientcmdapi.ExecEnvVar{
		{
			Name:  "ATMOS_IDENTITY",
			Value: identityName,
		},
	}

	// Build exec plugin args.
	execArgs := []string{
		"auth",
		"eks-token",
		"--cluster-name",
		info.Name,
		"--region",
		info.Region,
	}
	if identityName != "" {
		execArgs = append(execArgs, "--identity", identityName)
	}

	config := clientcmdapi.NewConfig()
	config.CurrentContext = contextName

	config.Clusters[info.ARN] = &clientcmdapi.Cluster{
		Server:                   info.Endpoint,
		CertificateAuthorityData: []byte(info.CertificateAuthorityData),
	}

	config.Contexts[contextName] = &clientcmdapi.Context{
		Cluster:  info.ARN,
		AuthInfo: userName,
	}

	config.AuthInfos[userName] = &clientcmdapi.AuthInfo{
		Exec: &clientcmdapi.ExecConfig{
			APIVersion:      execAPIVersion,
			Command:         atmosCommand,
			Args:            execArgs,
			Env:             execEnv,
			InteractiveMode: clientcmdapi.NeverExecInteractiveMode,
		},
	}

	return config
}

// DefaultKubeconfigPath returns the XDG-compliant default kubeconfig path.
// Returns ~/.config/atmos/kube/config.
func DefaultKubeconfigPath() (string, error) {
	defer perf.Track(nil, "kube.DefaultKubeconfigPath")()

	kubeDir, err := xdg.GetXDGConfigDir("kube", defaultDirMode)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrKubeconfigPath, err)
	}

	return filepath.Join(kubeDir, "config"), nil
}

// writeConfig writes a kubeconfig to the file, replacing any existing content.
func (m *KubeconfigManager) writeConfig(config *clientcmdapi.Config) error {
	if err := clientcmd.WriteToFile(*config, m.path); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrKubeconfigWrite, err)
	}

	return os.Chmod(m.path, m.mode)
}

// mergeConfig merges a kubeconfig into the existing file.
func (m *KubeconfigManager) mergeConfig(newConfig *clientcmdapi.Config) error {
	// Load existing config if file exists.
	existing := clientcmdapi.NewConfig()
	if _, err := os.Stat(m.path); err == nil {
		loaded, loadErr := clientcmd.LoadFromFile(m.path)
		if loadErr != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrKubeconfigMerge, loadErr)
		}
		existing = loaded
	}

	// Merge new entries into existing config.
	for k, v := range newConfig.Clusters {
		existing.Clusters[k] = v
	}
	for k, v := range newConfig.Contexts {
		existing.Contexts[k] = v
	}
	for k, v := range newConfig.AuthInfos {
		existing.AuthInfos[k] = v
	}

	// Set current-context to the new config's current-context.
	if newConfig.CurrentContext != "" {
		existing.CurrentContext = newConfig.CurrentContext
	}

	return m.writeConfig(existing)
}
