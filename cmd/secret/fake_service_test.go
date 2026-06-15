package secret

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

// setCall records a single Set invocation on the fake service.
type setCall struct {
	name  string
	value any
}

// importCall records a single ImportFromStore invocation on the fake service.
type importCall struct {
	name   string
	src    secrets.ImportSource
	dryRun bool
}

// getCall records a single Get invocation on the fake service.
type getCall struct {
	name string
	opts secrets.ResolveOptions
}

// fakeSecretService is a configurable, call-recording implementation of secretService used to drive
// the secret command handlers without constructing real config/auth/backends.
type fakeSecretService struct {
	// Configurable return values.
	declarations   []secrets.Declaration
	statuses       []secrets.Status
	validation     secrets.ValidationResult
	declared       map[string]bool
	getValues      map[string]any
	getErrs        map[string]error
	missingVaults  []secrets.GenerableVault
	keygenResults  map[string]*providers.KeygenResult
	scopes         map[string]secrets.Scope
	deleteAllCount int

	// Configurable errors.
	setErr       error
	getErr       error
	deleteErr    error
	deleteAllErr error
	missingErr   error
	keygenErr    error
	importErr    error

	// Recorded calls.
	setCalls       []setCall
	getCalls       []getCall
	importCalls    []importCall
	deletedNames   []string
	deleteAllCalls int
	generatedVault []secrets.GenerableVault
}

// newFakeSecretService returns a fake with empty maps ready for configuration.
func newFakeSecretService() *fakeSecretService {
	return &fakeSecretService{
		declared:      map[string]bool{},
		getValues:     map[string]any{},
		getErrs:       map[string]error{},
		keygenResults: map[string]*providers.KeygenResult{},
	}
}

func (f *fakeSecretService) Declarations() []secrets.Declaration { return f.declarations }

func (f *fakeSecretService) Set(name string, value any) error {
	f.setCalls = append(f.setCalls, setCall{name: name, value: value})
	return f.setErr
}

func (f *fakeSecretService) Get(name string, opts secrets.ResolveOptions) (any, error) {
	f.getCalls = append(f.getCalls, getCall{name: name, opts: opts})
	if f.getErr != nil {
		return nil, f.getErr
	}
	if err, ok := f.getErrs[name]; ok {
		return nil, err
	}
	return f.getValues[name], nil
}

func (f *fakeSecretService) Delete(name string) error {
	f.deletedNames = append(f.deletedNames, name)
	return f.deleteErr
}

func (f *fakeSecretService) DeleteAll() (int, error) {
	f.deleteAllCalls++
	return f.deleteAllCount, f.deleteAllErr
}

func (f *fakeSecretService) ImportFromStore(name string, src secrets.ImportSource, dryRun bool) error {
	f.importCalls = append(f.importCalls, importCall{name: name, src: src, dryRun: dryRun})
	return f.importErr
}

func (f *fakeSecretService) Status() []secrets.Status { return f.statuses }

func (f *fakeSecretService) IsDeclared(name string) bool { return f.declared[name] }

// ScopeOf reports the configured scope for a name. An entry in `scopes` makes the name declared;
// a name present in `declared` but absent from `scopes` defaults to instance scope.
func (f *fakeSecretService) ScopeOf(name string) (secrets.Scope, bool) {
	if sc, ok := f.scopes[name]; ok {
		return sc, true
	}
	if f.declared[name] {
		return secrets.ScopeInstance, true
	}
	return "", false
}

func (f *fakeSecretService) Validate() secrets.ValidationResult { return f.validation }

func (f *fakeSecretService) VaultsMissingKeys() ([]secrets.GenerableVault, error) {
	if f.missingErr != nil {
		return nil, f.missingErr
	}
	return f.missingVaults, nil
}

func (f *fakeSecretService) GenerateKeyForVault(v secrets.GenerableVault) (*providers.KeygenResult, error) {
	f.generatedVault = append(f.generatedVault, v)
	if f.keygenErr != nil {
		return nil, f.keygenErr
	}
	if res, ok := f.keygenResults[v.Name]; ok {
		return res, nil
	}
	return &providers.KeygenResult{Vault: v.Name, Kind: "sops/age", Summary: "Generated an age key pair."}, nil
}

// installService overrides loadServiceFn to return the given fake (or loadErr when set) and restores
// the original via t.Cleanup. It is the common seam used by set/get/delete/list/init/validate/
// pull/push/import handlers.
func installService(t *testing.T, svc secretService, loadErr error) {
	t.Helper()

	orig := loadServiceFn
	loadServiceFn = func(_ secretScope) (secretService, error) {
		if loadErr != nil {
			return nil, loadErr
		}
		return svc, nil
	}
	t.Cleanup(func() { loadServiceFn = orig })
}

// installServiceAndConfig overrides loadServiceAndConfigFn (used by exec/shell) and restores the
// original via t.Cleanup. A nil atmosConfig is replaced with an empty configuration.
func installServiceAndConfig(t *testing.T, svc secretService, atmosConfig *schema.AtmosConfiguration, loadErr error) {
	t.Helper()

	if atmosConfig == nil {
		atmosConfig = &schema.AtmosConfiguration{}
	}
	orig := loadServiceAndConfigFn
	loadServiceAndConfigFn = func(_ secretScope) (secretService, *schema.AtmosConfiguration, error) {
		if loadErr != nil {
			return nil, nil, loadErr
		}
		return svc, atmosConfig, nil
	}
	t.Cleanup(func() { loadServiceAndConfigFn = orig })
}

// overrideEnumerateScopes overrides enumerateScopesFn and restores it via t.Cleanup. It lets tests
// that exercise commands which enumerate the stack (e.g. validate's SOPS collision guard, list)
// avoid real stack processing. The returned config is used as the atmosConfig for built services.
func overrideEnumerateScopes(t *testing.T, entries []scopeEntry, err error) {
	t.Helper()

	orig := enumerateScopesFn
	enumerateScopesFn = func(_ secretScope) ([]scopeEntry, *schema.AtmosConfiguration, error) {
		return entries, &schema.AtmosConfiguration{}, err
	}
	t.Cleanup(func() { enumerateScopesFn = orig })
}

// overridePromptForValue overrides promptForValueFn and restores it via t.Cleanup.
func overridePromptForValue(t *testing.T, value string, err error) {
	t.Helper()

	orig := promptForValueFn
	promptForValueFn = func() (string, error) { return value, err }
	t.Cleanup(func() { promptForValueFn = orig })
}

// overrideConfirmAction overrides confirmActionFn and restores it via t.Cleanup. It records the
// titles it was asked to confirm in the returned slice pointer.
func overrideConfirmAction(t *testing.T, confirmed bool, err error) *[]string {
	t.Helper()

	titles := &[]string{}
	orig := confirmActionFn
	confirmActionFn = func(title string) (bool, error) {
		*titles = append(*titles, title)
		return confirmed, err
	}
	t.Cleanup(func() { confirmActionFn = orig })
	return titles
}

// overrideRunCommand overrides runCommandFn and restores it via t.Cleanup. The captured args/env are
// recorded into the returned pointers.
func overrideRunCommand(t *testing.T, err error) (gotArgs *[]string, gotEnv *[]string) {
	t.Helper()

	gotArgs = &[]string{}
	gotEnv = &[]string{}
	orig := runCommandFn
	runCommandFn = func(args []string, env []string) error {
		*gotArgs = args
		*gotEnv = env
		return err
	}
	t.Cleanup(func() { runCommandFn = orig })
	return gotArgs, gotEnv
}

// overrideStartShell overrides startShellFn and restores it via t.Cleanup. The captured command,
// args, and env are recorded into the returned pointers.
func overrideStartShell(t *testing.T, err error) (gotCmd *string, gotArgs *[]string, gotEnv *[]string) {
	t.Helper()

	gotCmd = new(string)
	gotArgs = &[]string{}
	gotEnv = &[]string{}
	orig := startShellFn
	startShellFn = func(shellCommand string, shellArgs []string, env []string) error {
		*gotCmd = shellCommand
		*gotArgs = shellArgs
		*gotEnv = env
		return err
	}
	t.Cleanup(func() { startShellFn = orig })
	return gotCmd, gotArgs, gotEnv
}
