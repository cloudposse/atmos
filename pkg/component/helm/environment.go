package helm

import (
	"context"
	"fmt"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

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
