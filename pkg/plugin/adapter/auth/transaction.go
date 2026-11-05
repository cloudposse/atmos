package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/perf"
)

const sessionDirectoryMode = 0o700

type (
	leaseKey     string
	sessionLease struct{ active atomic.Bool }
)

func (d *Definition) sessionNamespace(name, realm string, settings map[string]any) (string, error) {
	fingerprint, err := json.Marshal([]any{d.Package, name, realm, settings})
	if err != nil {
		return "", errUtils.ErrPluginInvalidConfig
	}
	digest := sha256.Sum256(fingerprint)
	return hex.EncodeToString(digest[:]), nil
}

func (d *Definition) transaction(ctx context.Context, namespace string, operation func(context.Context) error) error {
	if d.Services.LockDirectory == "" {
		return errUtils.ErrPluginInvalidConfig
	}
	key := leaseKey(filepath.Join(d.Services.LockDirectory, namespace+".lock"))
	if lease, ok := ctx.Value(key).(*sessionLease); ok && lease.active.Load() {
		return operation(ctx)
	}
	if err := os.MkdirAll(d.Services.LockDirectory, sessionDirectoryMode); err != nil {
		return errUtils.ErrPluginAPI
	}
	return filelock.New(string(key)).WithExclusive(ctx, func() error {
		lease := &sessionLease{}
		lease.active.Store(true)
		defer lease.active.Store(false)
		return operation(context.WithValue(ctx, key, lease))
	})
}

// WithCredentialTransaction holds the private session lock through the complete
// auth chain, including cloud-file publication and dependent logout cleanup.
func (p *provider) WithCredentialTransaction(ctx context.Context, operation func(context.Context) error) error {
	defer perf.Track(nil, "pluginauth.provider.WithCredentialTransaction")()

	namespace, err := p.definition.sessionNamespace(p.name, p.realm, p.sessionSettings())
	if err != nil {
		return err
	}
	return p.definition.transaction(ctx, namespace, operation)
}
