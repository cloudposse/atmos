package auth

import (
	"context"
	"maps"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

type environmentMaterializer struct {
	mu          sync.Mutex
	credentials *types.PluginCredentials
}

// EnvironmentMaterializer keeps environment credentials scoped to this identity.
//
//nolint:lintroller // Factory without I/O.
func EnvironmentMaterializer(string, *schema.Identity) Materializer {
	return &environmentMaterializer{}
}

func (m *environmentMaterializer) Apply(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "auth.environmentMaterializer.Apply")()
	if err := ctx.Err(); err != nil {
		return err
	}
	if params == nil {
		return errUtils.ErrPluginProtocol
	}
	credentials, ok := params.Credentials.(*types.PluginCredentials)
	if !ok || credentials.Type != "environment/v1" || credentials.IsExpired() {
		return errUtils.ErrPluginProtocol
	}
	clone := *credentials
	clone.Data = maps.Clone(credentials.Data)
	clone.SensitiveKeys = append([]string(nil), credentials.SensitiveKeys...)
	for _, value := range clone.Data {
		text, ok := value.(string)
		if !ok {
			return errUtils.ErrPluginProtocol
		}
		atmosio.RegisterSecret(text)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.credentials = &clone
	return nil
}

func (m *environmentMaterializer) Environment() (map[string]string, error) {
	defer perf.Track(nil, "auth.environmentMaterializer.Environment")()
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]string)
	if m.credentials == nil || m.credentials.IsExpired() {
		return nil, errUtils.ErrAuthenticationFailed
	}
	for name, value := range m.credentials.Data {
		result[name] = value.(string)
	}
	return result, nil
}

func (m *environmentMaterializer) PrepareEnvironment(base map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "auth.environmentMaterializer.PrepareEnvironment")()
	values, err := m.Environment()
	if err != nil {
		return nil, err
	}
	result := maps.Clone(base)
	if result == nil {
		result = make(map[string]string)
	}
	// Environment names are case insensitive on Windows. Remove conflicting
	// spellings on every platform so the same configuration behaves consistently.
	for name, value := range values {
		for existing := range result {
			if strings.EqualFold(existing, name) {
				delete(result, existing)
			}
		}
		result[name] = value
	}
	return result, nil
}

func (m *environmentMaterializer) Logout(context.Context) error {
	defer perf.Track(nil, "auth.environmentMaterializer.Logout")()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.credentials = nil
	return nil
}

func (m *environmentMaterializer) Load(context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "auth.environmentMaterializer.Load")()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.credentials == nil || m.credentials.IsExpired() {
		return nil, errUtils.ErrAuthenticationFailed
	}
	clone := *m.credentials
	clone.Data = maps.Clone(m.credentials.Data)
	clone.SensitiveKeys = append([]string(nil), m.credentials.SensitiveKeys...)
	return &clone, nil
}

//nolint:lintroller // No credential files are created.
func (*environmentMaterializer) Paths() ([]types.Path, error) { return nil, nil }

//nolint:lintroller // Environment credentials have no realm-dependent files.
func (*environmentMaterializer) SetRealm(string) {}

//nolint:lintroller // Environment credentials have no provider-dependent files.
func (*environmentMaterializer) SetRootProvider(string) {}
