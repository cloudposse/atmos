package secret

import (
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
	"github.com/cloudposse/atmos/pkg/shell"
)

// secretService is the subset of *secrets.Service the command handlers depend on. It exists so
// handlers operate against an interface, letting tests inject a fake without constructing real
// config/auth. *secrets.Service satisfies it structurally — no change to pkg/secrets is required.
type secretService interface {
	Declarations() []secrets.Declaration
	Set(name string, value any) error
	Get(name string, opts secrets.ResolveOptions) (any, error)
	Delete(name string) error
	DeleteAll() (int, error)
	ImportFromStore(name string, src secrets.ImportSource, dryRun bool) error
	Status() []secrets.Status
	IsDeclared(name string) bool
	ScopeOf(name string) (secrets.Scope, bool)
	Validate() secrets.ValidationResult
	VaultsMissingKeys() ([]secrets.GenerableVault, error)
	GenerateKeyForVault(v secrets.GenerableVault) (*providers.KeygenResult, error)
}

// Seam variables wrap the real implementations so tests can override behavior. Each defaults to the
// production function; handlers call the variable, never the underlying function directly. Restore
// any override with t.Cleanup in tests.
var (
	// Load a scoped secrets service (config + auth + component resolution).
	loadServiceFn = func(scope secretScope) (secretService, error) {
		svc, err := loadService(scope)
		if err != nil {
			return nil, err
		}
		return svc, nil
	}

	// Load a scoped service plus the resolved AtmosConfiguration (for exec/shell, which merge
	// atmosConfig.Env into the child environment).
	loadServiceAndConfigFn = func(scope secretScope) (secretService, *schema.AtmosConfiguration, error) {
		svc, atmosConfig, err := loadServiceAndConfig(scope)
		if err != nil {
			return nil, nil, err
		}
		return svc, atmosConfig, nil
	}

	// Interactively read a secret value (masked input).
	promptForValueFn = promptForSecretValue

	// Interactively confirm a destructive action.
	confirmActionFn = confirmAction

	// Run a command with the resolved environment (used by `secret exec`).
	runCommandFn = shell.RunCommand

	// Launch an interactive shell with the resolved environment (used by `secret shell`).
	startShellFn = shell.StartInteractive
)
