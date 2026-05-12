package kube

import (
	"bytes"
	"encoding/base64"
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
// Returns changed=true when the on-disk file was modified, and changed=false
// when the existing kubeconfig already matches the desired state (no write
// performed). Callers can use this to suppress noisy success messages on
// repeated invocations that produce identical output.
func (m *KubeconfigManager) WriteClusterConfig(info *awsCloud.EKSClusterInfo, alias, identityName, updateMode string) (bool, error) {
	defer perf.Track(nil, "kube.KubeconfigManager.WriteClusterConfig")()

	if updateMode == "" {
		updateMode = defaultUpdateMode
	}

	// Build the kubeconfig for this cluster.
	newConfig := BuildClusterConfig(info, alias, identityName)

	// Ensure parent directory exists.
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, defaultDirMode); err != nil {
		return false, fmt.Errorf("%w: failed to create directory %s: %w", errUtils.ErrKubeconfigWrite, dir, err)
	}

	switch updateMode {
	case "replace":
		return m.writeIfChanged(newConfig)

	case "error":
		if _, err := os.Stat(m.path); err == nil {
			// File exists, check for cluster, context, and auth info collisions.
			existing, loadErr := clientcmd.LoadFromFile(m.path)
			if loadErr == nil {
				if _, exists := existing.Clusters[info.ARN]; exists {
					return false, fmt.Errorf("%w: cluster %s already exists in %s", errUtils.ErrKubeconfigMerge, info.ARN, m.path)
				}
				// Check context name collision.
				contextName := info.ARN
				if alias != "" {
					contextName = alias
				}
				if _, exists := existing.Contexts[contextName]; exists {
					return false, fmt.Errorf("%w: context %s already exists in %s", errUtils.ErrKubeconfigMerge, contextName, m.path)
				}
			}
		}
		return m.mergeIfChanged(newConfig)

	case "merge":
		return m.mergeIfChanged(newConfig)

	default:
		return false, fmt.Errorf("%w: invalid update mode %q", errUtils.ErrKubeconfigMerge, updateMode)
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

	// User name includes cluster name and region for uniqueness when multiple
	// clusters share the same identity.
	userName := "atmos-eks-" + info.Name + "-" + info.Region

	// Build exec plugin env vars. Only set ATMOS_IDENTITY when identity is specified.
	var execEnv []clientcmdapi.ExecEnvVar
	if identityName != "" {
		execEnv = append(execEnv, clientcmdapi.ExecEnvVar{
			Name:  "ATMOS_IDENTITY",
			Value: identityName,
		})
	}

	// Build exec plugin args.
	execArgs := []string{
		"aws",
		"eks",
		"token",
		"--cluster-name",
		info.Name,
		"--region",
		info.Region,
	}
	if identityName != "" {
		execArgs = append(execArgs, "--identity="+identityName)
	}

	config := clientcmdapi.NewConfig()
	config.CurrentContext = contextName

	// The EKS API returns CertificateAuthority.Data as base64-encoded PEM.
	// clientcmdapi.Cluster.CertificateAuthorityData expects raw PEM bytes
	// (client-go base64-encodes them when writing the YAML).
	caData, err := base64.StdEncoding.DecodeString(info.CertificateAuthorityData)
	if err != nil {
		// If decoding fails, assume the data is already raw PEM.
		caData = []byte(info.CertificateAuthorityData)
	}

	config.Clusters[info.ARN] = &clientcmdapi.Cluster{
		Server:                   info.Endpoint,
		CertificateAuthorityData: caData,
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

	if err := os.Chmod(m.path, m.mode); err != nil {
		return fmt.Errorf("%w: failed to set permissions on %s: %w", errUtils.ErrKubeconfigWrite, m.path, err)
	}

	return nil
}

// writeIfChanged writes newConfig only if it differs from the file on disk.
// Returns changed=false when the file already serializes to the same bytes
// and the on-disk mode already matches the configured mode. Always reconciles
// permissions on the no-op path so a file left at a weaker mode (e.g., 0644
// when 0600 was configured) is brought back into compliance.
func (m *KubeconfigManager) writeIfChanged(newConfig *clientcmdapi.Config) (bool, error) {
	if _, err := os.Stat(m.path); err == nil {
		existing, loadErr := clientcmd.LoadFromFile(m.path)
		if loadErr == nil {
			same, cmpErr := configsEqual(existing, newConfig)
			if cmpErr == nil && same {
				return m.reconcileMode()
			}
		}
	}
	if err := m.writeConfig(newConfig); err != nil {
		return false, err
	}
	return true, nil
}

// mergeIfChanged merges newConfig into the existing file. Returns changed=false
// when the merge result would equal the existing on-disk content.
func (m *KubeconfigManager) mergeIfChanged(newConfig *clientcmdapi.Config) (bool, error) {
	// Load existing config if file exists.
	existing := clientcmdapi.NewConfig()
	fileExists := false
	if _, err := os.Stat(m.path); err == nil {
		loaded, loadErr := clientcmd.LoadFromFile(m.path)
		if loadErr != nil {
			return false, fmt.Errorf("%w: %w", errUtils.ErrKubeconfigMerge, loadErr)
		}
		existing = loaded
		fileExists = true
	}

	// Snapshot the existing serialized form before mutating, so we can compare
	// against the post-merge state without a deep-copy dance.
	var beforeBytes []byte
	if fileExists {
		b, err := clientcmd.Write(*existing)
		if err == nil {
			beforeBytes = b
		}
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

	if beforeBytes != nil {
		afterBytes, err := clientcmd.Write(*existing)
		if err == nil && bytes.Equal(beforeBytes, afterBytes) {
			return m.reconcileMode()
		}
	}

	if err := m.writeConfig(existing); err != nil {
		return false, err
	}
	return true, nil
}

// reconcileMode enforces the configured file mode on the kubeconfig without
// touching its contents. Used by the no-op paths so a kubeconfig left at a
// weaker permission (e.g., manually chmod-ed or created by another tool) is
// brought back into compliance even when content is unchanged. Returns
// changed=true only when an actual chmod was performed.
func (m *KubeconfigManager) reconcileMode() (bool, error) {
	stat, err := os.Stat(m.path)
	if err != nil {
		return false, fmt.Errorf("%w: failed to stat %s: %w", errUtils.ErrKubeconfigWrite, m.path, err)
	}
	if stat.Mode().Perm() == m.mode.Perm() {
		return false, nil
	}
	if err := os.Chmod(m.path, m.mode); err != nil {
		return false, fmt.Errorf("%w: failed to set permissions on %s: %w", errUtils.ErrKubeconfigWrite, m.path, err)
	}
	return true, nil
}

// configsEqual returns true when two kubeconfigs serialize to identical bytes.
func configsEqual(a, b *clientcmdapi.Config) (bool, error) {
	aBytes, err := clientcmd.Write(*a)
	if err != nil {
		return false, err
	}
	bBytes, err := clientcmd.Write(*b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(aBytes, bBytes), nil
}
