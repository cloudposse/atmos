package emulator

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// ProfileResolver resolves a running emulator's live endpoint and connection
// profile for the `!emulator` YAML function, by component reference and stack.
//
// It is implemented ABOVE this layer (it needs stack processing and the container
// runtime) and registered at init via RegisterProfileResolver, keeping pkg/emulator
// free of any stack-processing import (no cycle).
type ProfileResolver func(atmosConfig *schema.AtmosConfiguration, ref, currentStack string, stackInfo *schema.ConfigAndStacksInfo) (*Endpoint, *Profile, error)

var profileResolver ProfileResolver

// RegisterProfileResolver registers the process-wide emulator profile resolver used
// by the `!emulator` YAML function. Called at init by pkg/component/emulator.
func RegisterProfileResolver(r ProfileResolver) {
	defer perf.Track(nil, "emulator.RegisterProfileResolver")()

	profileResolver = r
}

const (
	// The kubeconfigKey materializes the harvested kubeconfig to a file and returns its path.
	kubeconfigKey = "kubeconfig"
	// The envKeyPrefix selects a single SDK environment variable: `env.<VAR>`.
	envKeyPrefix = "env."
	// The yamlFuncCacheSubdir holds kubeconfigs materialized by the YAML function.
	yamlFuncCacheSubdir = "emulator"

	// Permission for the YAML-function kubeconfig cache directory.
	permCacheDir os.FileMode = 0o700
	// Permission for a materialized kubeconfig (contains the admin credential).
	permKubeconfigFile os.FileMode = 0o600
)

// ResolveYAMLFunc evaluates `!emulator <ref> <key>` where args is the string after
// the tag. <ref> is the emulator component name (resolved in currentStack); <key> is
// one of: endpoint|url, host, port, region, project, kubeconfig, or env.<VAR>.
func ResolveYAMLFunc(atmosConfig *schema.AtmosConfiguration, args, currentStack string, stackInfo *schema.ConfigAndStacksInfo) (any, error) {
	defer perf.Track(atmosConfig, "emulator.ResolveYAMLFunc")()

	ref, key, err := parseYAMLFuncArgs(args)
	if err != nil {
		return nil, err
	}
	if profileResolver == nil {
		return nil, fmt.Errorf("%w: `!emulator` requires the emulator component to be loaded", errUtils.ErrEmulatorResolverUnavailable)
	}

	endpoint, profile, err := profileResolver(atmosConfig, ref, currentStack, stackInfo)
	if err != nil {
		return nil, fmt.Errorf("resolve emulator %q: %w", ref, err)
	}

	return valueForKey(endpoint, profile, ref, currentStack, key)
}

// parseYAMLFuncArgs splits `<ref> <key>` (whitespace-separated, like `!store`).
func parseYAMLFuncArgs(args string) (ref, key string, err error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) != 2 {
		return "", "", fmt.Errorf("%w: `!emulator` expects `<ref> <key>`, got %q", errUtils.ErrEmulatorConfigInvalid, args)
	}
	return fields[0], fields[1], nil
}

// valueForKey maps a resolved endpoint/profile to the requested key's value.
func valueForKey(endpoint *Endpoint, profile *Profile, ref, currentStack, key string) (any, error) {
	if strings.HasPrefix(key, envKeyPrefix) {
		return envValue(profile, strings.TrimPrefix(key, envKeyPrefix)), nil
	}
	switch key {
	case "port":
		port, ok := endpoint.PrimaryHostPort()
		if !ok {
			return nil, fmt.Errorf("%w: emulator %q has no bound host port", errUtils.ErrEmulatorNotRunning, ref)
		}
		return strconv.Itoa(port), nil
	case kubeconfigKey:
		return materializeKubeconfig(profile, ref, currentStack)
	}
	if v, ok := scalarValue(endpoint, key); ok {
		return v, nil
	}
	return nil, fmt.Errorf("%w: unknown `!emulator` key %q (want endpoint|url|host|port|region|project|kubeconfig|env.<VAR>)",
		errUtils.ErrEmulatorConfigInvalid, key)
}

// envValue returns a single SDK environment variable from the profile (empty if absent).
func envValue(profile *Profile, name string) string {
	if profile == nil || profile.Env == nil {
		return ""
	}
	return profile.Env[name]
}

// scalarValue returns the value for a string-valued endpoint key, and whether the key is known.
func scalarValue(endpoint *Endpoint, key string) (string, bool) {
	switch key {
	case "endpoint", "url":
		return endpoint.URL("http"), true
	case "host":
		return endpoint.Host, true
	case "region":
		return endpoint.Region, true
	case "project":
		return endpoint.Project, true
	default:
		return "", false
	}
}

// materializeKubeconfig writes the harvested kubeconfig to a stack-scoped cache file
// and returns its path, for `KUBECONFIG: !emulator <k3s> kubeconfig`.
func materializeKubeconfig(profile *Profile, ref, currentStack string) (any, error) {
	if profile == nil || len(profile.Kubeconfig) == 0 {
		return nil, fmt.Errorf("%w: emulator %q exposes no kubeconfig (not a kubernetes target?)", errUtils.ErrEmulatorConfigInvalid, ref)
	}
	dir, err := xdg.GetXDGCacheDir(yamlFuncCacheSubdir, permCacheDir)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve emulator kubeconfig cache dir: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	// Stack-scoped name keeps concurrent stacks from clobbering each other.
	replacer := strings.NewReplacer("/", "_", string(os.PathSeparator), "_", "..", "__")
	safeStack := replacer.Replace(currentStack)
	safeRef := replacer.Replace(ref)
	path := filepath.Join(dir, safeStack+"-"+safeRef+".kubeconfig")
	if err := os.WriteFile(path, profile.Kubeconfig, permKubeconfigFile); err != nil {
		return nil, fmt.Errorf("%w: write emulator kubeconfig: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	return path, nil
}
