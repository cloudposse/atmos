package kubernetes

import (
	"context"
	"fmt"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// prepareComponentEnvironment applies the component's toolchain/stack environment and
// then the selected identity's prepared environment (notably KUBECONFIG, so Atmos Auth
// wins for the in-process client's cluster connection) to the process, returning a
// single restore function that reverts both in reverse order.
func prepareComponentEnvironment(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (func(), error) {
	defer perf.Track(atmosConfig, "kubernetes.prepareComponentEnvironment")()

	tenv, err := dependenciesForComponent(atmosConfig, cfg.KubernetesComponentType, info.StackSection, info.ComponentSection)
	if err != nil {
		return nil, err
	}
	envRestore := applyEnvironment(info.ComponentEnvSection, tenv.EnvVars())

	authEnvRestore, err := applyAuthEnvironment(info)
	if err != nil {
		envRestore()
		return nil, err
	}

	return func() {
		authEnvRestore()
		envRestore()
	}, nil
}

// applyAuthEnvironment applies the environment contributed by the selected identity and
// any linked integrations — notably KUBECONFIG for `kubernetes/emulator` identities and
// `aws/eks` integrations — to the process environment. The in-process Kubernetes client
// discovers its kubeconfig through client-go's default loading rules (the KUBECONFIG env
// var), so without this the cluster Atmos Auth resolved would be invisible to the client.
// Returns a restore function that reverts the changes. No-op when no identity is selected.
func applyAuthEnvironment(info *schema.ConfigAndStacksInfo) (func(), error) {
	noop := func() {}
	if info.Identity == "" {
		return noop, nil
	}
	authManager, ok := info.AuthManager.(auth.AuthManager)
	if !ok || authManager == nil {
		return noop, nil
	}

	preparedList, err := authManager.PrepareShellEnvironment(context.Background(), info.Identity, os.Environ())
	if err != nil {
		return noop, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Apply only the keys the identity added or changed relative to the current
	// environment, leaving unrelated process env untouched (and restorable).
	current := environToMap(os.Environ())
	identityEnv := make(map[string]any)
	for _, item := range preparedList {
		key, value, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		if existing, exists := current[key]; !exists || existing != value {
			identityEnv[key] = value
		}
	}

	return applyEnvironment(identityEnv, nil), nil
}

// environToMap converts a "KEY=VALUE" environment list into a map.
func environToMap(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, item := range environ {
		key, value, found := strings.Cut(item, "=")
		if found {
			out[key] = value
		}
	}
	return out
}

// applyEnvironment sets the given component env and toolchain env into the process
// environment, returning a restore function that reverts each touched key to its prior
// value (or unsets it if it was previously absent).
func applyEnvironment(componentEnv map[string]any, toolchainEnv []string) func() {
	original := make(map[string]*string)
	setEnv := func(key, value string) {
		if _, ok := original[key]; !ok {
			if existing, exists := os.LookupEnv(key); exists {
				existingCopy := existing
				original[key] = &existingCopy
			} else {
				original[key] = nil
			}
		}
		_ = os.Setenv(key, value)
	}

	for key, value := range componentEnv {
		setEnv(key, fmt.Sprintf("%v", value))
	}
	for _, item := range toolchainEnv {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			setEnv(key, value)
		}
	}

	return func() {
		for key, value := range original {
			if value == nil {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, *value)
			}
		}
	}
}
