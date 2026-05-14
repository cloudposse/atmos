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
// Returns changed=false when the file already contains the same kubeconfig
// content and the on-disk mode already matches the configured mode. Always
// reconciles permissions on the no-op path so a file left at a weaker mode
// (e.g., 0644 when 0600 was configured) is brought back into compliance.
func (m *KubeconfigManager) writeIfChanged(newConfig *clientcmdapi.Config) (bool, error) {
	if _, err := os.Stat(m.path); err == nil {
		existing, loadErr := clientcmd.LoadFromFile(m.path)
		if loadErr == nil && configContentEqual(existing, newConfig) {
			return m.reconcileMode()
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

	// Short-circuit when the merge would be a no-op: every entry in newConfig
	// is already present in existing with the same fields, and current-context
	// already matches. We check this structurally rather than by serializing
	// YAML to bytes — clientcmd's YAML output can vary by platform (Windows
	// CRLF handling, internal LocationOfOrigin paths populated during load) in
	// ways that don't reflect actual content changes and would cause false
	// positives on the no-op path.
	if fileExists && !mergeWouldChange(existing, newConfig) {
		return m.reconcileMode()
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

// configContentEqual returns true when two kubeconfigs have the same
// user-visible content: same current-context, same clusters, same contexts,
// same auth infos. Used by the replace-mode no-op detection.
//
// Compares fields structurally rather than via YAML byte equality so the
// check is robust against platform-specific clientcmd serialization quirks
// (line endings, LocationOfOrigin populated during load on Windows, …) that
// don't reflect actual content changes.
//
// When ATMOS_KUBECONFIG_DEBUG_DIFF is set, emits a diagnostic line to
// stderr describing which entry/field caused the inequality.
func configContentEqual(a, b *clientcmdapi.Config) bool {
	if a == nil || b == nil {
		return a == b
	}
	debug := kubeconfigDebugDiff()
	if a.CurrentContext != b.CurrentContext {
		if debug {
			fmt.Fprintf(os.Stderr, "[kubeconfig-diff] CurrentContext differs: a=%q b=%q\n", a.CurrentContext, b.CurrentContext)
		}
		return false
	}
	if !clusterMapsEqualDebug(a.Clusters, b.Clusters, debug) {
		return false
	}
	if !contextMapsEqualDebug(a.Contexts, b.Contexts, debug) {
		return false
	}
	return authInfoMapsEqualDebug(a.AuthInfos, b.AuthInfos, debug)
}

func clusterMapsEqualDebug(a, b map[string]*clientcmdapi.Cluster, debug bool) bool {
	if len(a) != len(b) {
		if debug {
			fmt.Fprintf(os.Stderr, "[kubeconfig-diff] Clusters map length differs: a=%d b=%d\n", len(a), len(b))
		}
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] Clusters[%q] missing from b\n", k)
			}
			return false
		}
		if !clustersEqual(av, bv) {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] Clusters[%q] differs:\n  a=%#v\n  b=%#v\n", k, av, bv)
			}
			return false
		}
	}
	return true
}

func contextMapsEqualDebug(a, b map[string]*clientcmdapi.Context, debug bool) bool {
	if len(a) != len(b) {
		if debug {
			fmt.Fprintf(os.Stderr, "[kubeconfig-diff] Contexts map length differs: a=%d b=%d\n", len(a), len(b))
		}
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] Contexts[%q] missing from b\n", k)
			}
			return false
		}
		if !contextsEqual(av, bv) {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] Contexts[%q] differs:\n  a=%#v\n  b=%#v\n", k, av, bv)
			}
			return false
		}
	}
	return true
}

func authInfoMapsEqualDebug(a, b map[string]*clientcmdapi.AuthInfo, debug bool) bool {
	if len(a) != len(b) {
		if debug {
			fmt.Fprintf(os.Stderr, "[kubeconfig-diff] AuthInfos map length differs: a=%d b=%d\n", len(a), len(b))
		}
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] AuthInfos[%q] missing from b\n", k)
			}
			return false
		}
		if !authInfosEqual(av, bv) {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] AuthInfos[%q] differs:\n  a=%#v\n  b=%#v\n", k, av, bv)
				if av.Exec != nil || bv.Exec != nil {
					fmt.Fprintf(os.Stderr, "  a.Exec=%#v\n  b.Exec=%#v\n", av.Exec, bv.Exec)
				}
			}
			return false
		}
	}
	return true
}

// mergeWouldChange returns true when merging newConfig into existing would
// produce a different result. The merge is a no-op when every entry in
// newConfig is already present in existing with identical fields and
// current-context already matches. Entries in existing that aren't in
// newConfig are preserved by merge regardless, so they don't enter the
// comparison.
//
// When ATMOS_KUBECONFIG_DEBUG_DIFF is set in the environment, the function
// emits a diagnostic line to stderr describing which entry/field caused
// the "would change" result. Used to debug platform-specific divergences
// where a logically-identical kubeconfig appears to differ after a load+
// re-serialize cycle (most notably on Windows).
func mergeWouldChange(existing, newConfig *clientcmdapi.Config) bool {
	debug := kubeconfigDebugDiff()

	if newConfig.CurrentContext != "" && existing.CurrentContext != newConfig.CurrentContext {
		if debug {
			fmt.Fprintf(os.Stderr, "[kubeconfig-diff] CurrentContext differs: existing=%q new=%q\n",
				existing.CurrentContext, newConfig.CurrentContext)
		}
		return true
	}
	for k, v := range newConfig.Clusters {
		ev, ok := existing.Clusters[k]
		if !ok {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] Clusters[%q] missing from existing\n", k)
			}
			return true
		}
		if !clustersEqual(ev, v) {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] Clusters[%q] differs:\n  existing=%#v\n  new=%#v\n", k, ev, v)
			}
			return true
		}
	}
	for k, v := range newConfig.Contexts {
		ev, ok := existing.Contexts[k]
		if !ok {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] Contexts[%q] missing from existing\n", k)
			}
			return true
		}
		if !contextsEqual(ev, v) {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] Contexts[%q] differs:\n  existing=%#v\n  new=%#v\n", k, ev, v)
			}
			return true
		}
	}
	for k, v := range newConfig.AuthInfos {
		ev, ok := existing.AuthInfos[k]
		if !ok {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] AuthInfos[%q] missing from existing\n", k)
			}
			return true
		}
		if !authInfosEqual(ev, v) {
			if debug {
				fmt.Fprintf(os.Stderr, "[kubeconfig-diff] AuthInfos[%q] differs:\n  existing=%#v\n  new=%#v\n", k, ev, v)
				if ev.Exec != nil || v.Exec != nil {
					fmt.Fprintf(os.Stderr, "  existing.Exec=%#v\n  new.Exec=%#v\n", ev.Exec, v.Exec)
				}
			}
			return true
		}
	}
	return false
}

// kubeconfigDebugDiff returns true when the user has opted in to diagnostic
// output from the no-op detection helpers. Gate is an env var so prod paths
// stay silent and tests can flip it per-test via t.Setenv.
func kubeconfigDebugDiff() bool {
	return os.Getenv("ATMOS_KUBECONFIG_DEBUG_DIFF") != ""
}

// clusterMapsEqual returns true when two cluster maps have identical
// key sets and identical entry contents.
func clusterMapsEqual(a, b map[string]*clientcmdapi.Cluster) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !clustersEqual(av, bv) {
			return false
		}
	}
	return true
}

// contextMapsEqual returns true when two context maps have identical
// key sets and identical entry contents.
func contextMapsEqual(a, b map[string]*clientcmdapi.Context) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !contextsEqual(av, bv) {
			return false
		}
	}
	return true
}

// authInfoMapsEqual returns true when two auth-info maps have identical
// key sets and identical entry contents.
func authInfoMapsEqual(a, b map[string]*clientcmdapi.AuthInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !authInfosEqual(av, bv) {
			return false
		}
	}
	return true
}

// clustersEqual compares the fields of two Cluster entries that BuildClusterConfig
// populates plus the other fields a user might reasonably set in a kubeconfig.
// LocationOfOrigin and Extensions are intentionally excluded — they're load-time
// metadata, not user-visible content.
func clustersEqual(a, b *clientcmdapi.Cluster) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Server == b.Server &&
		a.TLSServerName == b.TLSServerName &&
		a.InsecureSkipTLSVerify == b.InsecureSkipTLSVerify &&
		a.CertificateAuthority == b.CertificateAuthority &&
		bytes.Equal(a.CertificateAuthorityData, b.CertificateAuthorityData) &&
		a.ProxyURL == b.ProxyURL &&
		a.DisableCompression == b.DisableCompression
}

// contextsEqual compares the meaningful fields of two Context entries.
func contextsEqual(a, b *clientcmdapi.Context) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Cluster == b.Cluster &&
		a.AuthInfo == b.AuthInfo &&
		a.Namespace == b.Namespace
}

// authInfosEqual compares the meaningful fields of two AuthInfo entries,
// including the embedded exec credential plugin config (which is what
// BuildClusterConfig populates). Impersonation and auth-provider fields are
// included so a user-edited kubeconfig with those set isn't incorrectly
// detected as "unchanged" against a freshly built exec-plugin AuthInfo.
func authInfosEqual(a, b *clientcmdapi.AuthInfo) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.ClientCertificate != b.ClientCertificate ||
		a.ClientKey != b.ClientKey ||
		!bytes.Equal(a.ClientCertificateData, b.ClientCertificateData) ||
		!bytes.Equal(a.ClientKeyData, b.ClientKeyData) ||
		a.Token != b.Token ||
		a.TokenFile != b.TokenFile ||
		a.Username != b.Username ||
		a.Password != b.Password ||
		a.Impersonate != b.Impersonate ||
		a.ImpersonateUID != b.ImpersonateUID {
		return false
	}
	if !stringSlicesEqual(a.ImpersonateGroups, b.ImpersonateGroups) {
		return false
	}
	if !impersonateUserExtraEqual(a.ImpersonateUserExtra, b.ImpersonateUserExtra) {
		return false
	}
	if !authProvidersEqual(a.AuthProvider, b.AuthProvider) {
		return false
	}
	return execConfigsEqual(a.Exec, b.Exec)
}

// stringSlicesEqual returns true when two []string slices have the same
// length and elements in the same order. Nil and empty slices compare equal.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// impersonateUserExtraEqual compares two impersonate-extra maps for equality.
// Element order within each value slice is treated as significant — matches
// how clientcmd round-trips the field.
func impersonateUserExtraEqual(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !stringSlicesEqual(av, bv) {
			return false
		}
	}
	return true
}

// authProvidersEqual compares two AuthProviderConfig values. Both nil is equal.
func authProvidersEqual(a, b *clientcmdapi.AuthProviderConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Name != b.Name || len(a.Config) != len(b.Config) {
		return false
	}
	for k, av := range a.Config {
		if bv, ok := b.Config[k]; !ok || bv != av {
			return false
		}
	}
	return true
}

// execConfigsEqual compares two ExecConfig values. Returns true when both are
// nil; otherwise compares APIVersion, Command, Args, Env, and InteractiveMode.
func execConfigsEqual(a, b *clientcmdapi.ExecConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.APIVersion != b.APIVersion ||
		a.Command != b.Command ||
		a.InteractiveMode != b.InteractiveMode {
		return false
	}
	if len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if a.Args[i] != b.Args[i] {
			return false
		}
	}
	if len(a.Env) != len(b.Env) {
		return false
	}
	for i := range a.Env {
		if a.Env[i].Name != b.Env[i].Name || a.Env[i].Value != b.Env[i].Value {
			return false
		}
	}
	return true
}
