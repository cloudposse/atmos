package sops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/getsops/sops/v3"
	sopsaes "github.com/getsops/sops/v3/aes"
	sopsage "github.com/getsops/sops/v3/age"
	sopsconfig "github.com/getsops/sops/v3/config"
	"github.com/getsops/sops/v3/keyservice"
	sopsyaml "github.com/getsops/sops/v3/stores/yaml"
	sopsversion "github.com/getsops/sops/v3/version"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/sopsauth"
)

// init self-registers the SOPS track (track 2) so backend selection is a registry
// lookup rather than a central switch. New already matches providers.Constructor.
func init() {
	providers.Register(providers.TrackSops, New)
}

const (
	// Format string wrapping a sentinel error, a file path, and an underlying error.
	errFmtWrapFile = "%w: %q: %w"
	// Format string wrapping a sentinel error and a quoted identifier.
	errFmtWrapQuoted = "%w: %q"
	// Permission for SOPS-encrypted files (owner read/write only).
	secretFileMode os.FileMode = 0o600
	// Permission for directories created to hold SOPS files.
	secretDirMode os.FileMode = 0o755
	// Default base directory for scope-derived SOPS files when `spec.file` is not set.
	defaultSopsPath = "secrets"
	// Extension for scope-derived SOPS files.
	sopsFileExt = ".enc.yaml"
	// Reserved `age_key.store` value selecting the local file backend.
	ageKeyStoreFile = "file"
	// Fixed component segment for the store key under which the age
	// private key is written/read (the store triple is stack=<vault>, component=<this>, key=<path>).
	ageKeyStoreComponent = "age-key"
)

// sopsProvider implements the providers.Provider interface over a SOPS-encrypted file (track 2). It uses
// the getsops/sops Go SDK in-process — it does NOT shell out to the `sops` binary. Reads use the
// stable `decrypt` package; mutations load the encrypted tree, decrypt the data key via the age
// keysource (SOPS_AGE_KEY_FILE/SOPS_AGE_KEY), edit a top-level key, and re-encrypt in place.
type sopsProvider struct {
	name string
	kind string
	// file is the advanced override: a Go-template file path (e.g.
	// `secrets/{{ .atmos_stack }}.enc.yaml`). When empty, the path is derived in code from the
	// coordinate's scope under `path` (see derivePath), which is collision-safe by construction.
	file string
	// path is the base directory for scope-derived files (`spec.path`, default `secrets`). Used
	// only when `file` is empty.
	path          string
	ageRecipients string // Optional explicit age recipients for creating a fresh file.
	ageKeyFile    string // Optional path to the age private key (supports `~`/`$ENV`); falls back to SOPS_AGE_KEY_FILE/SOPS_AGE_KEY when empty.
	// ageKey is inline age private-key material (from `age_key.value` or a bare-string `age_key`).
	// It is the key text itself, so it can be populated by any YAML function — `!env …` / `!exec …`
	// / a `!store.get <store> <KEY>` — keeping the key out of a plain env var. Highest precedence.
	ageKey string
	// ageKeyStore names a configured store (e.g. the `keychain` store) that holds the age private
	// key, from `age_key.store`. When set (and not the reserved value "file"), the key is read from
	// — and `atmos secret keygen` writes it to — that store at ageKeyPath. Takes precedence over ageKeyFile.
	ageKeyStore string
	// ageKeyPath is the location of the key within ageKeyStore (a store key) or, in file mode, the
	// key file path (from `age_key.path`). Optional; defaults to the vault name for stores.
	ageKeyPath string
	// recipientsFile is where `atmos secret keygen` records this vault's public recipient. Defaults
	// to `.sops.yaml` (a creation rule) at the Atmos base path. Read at keygen time only.
	recipientsFile string
	// stores is the configured store registry (atmosConfig.Stores), used to resolve ageKeyStore.
	stores store.StoreRegistry
	// authResolver authenticates an Atmos identity and returns cloud credentials for cloud-KMS kinds
	// (sops/aws-kms, sops/gcp-kms, sops/azure-kv). Nil when no identity context was injected, in which
	// case cloud KMS falls back to the ambient credential chain (backward-compatible).
	authResolver store.AuthContextResolver
	// effectiveIdentity is the identity used for cloud-KMS auth, resolved with precedence:
	// per-provider `identity` > --identity/ATMOS_IDENTITY > stack/component default.
	effectiveIdentity string
}

// New builds a SOPS provider. The provider definition is resolved from the
// stack/component `secrets.providers` map first (so providers can be declared in a stack),
// then from the top-level `secrets.providers` in atmos.yaml.
func New(atmosConfig *schema.AtmosConfiguration, name string, sectionProviders map[string]any) (providers.Provider, error) {
	defer perf.Track(atmosConfig, "sops.New")()

	def, ok := lookupSopsProvider(sectionProviders, name)
	if !ok {
		cfg, found := atmosConfig.Secrets.Providers[name]
		if !found {
			return nil, fmt.Errorf(errFmtWrapQuoted, providers.ErrProviderNotFound, name)
		}
		def = sopsProviderDef{kind: cfg.Kind, identity: cfg.Identity, spec: cfg.Spec}
	}

	// Resolve the auth context for cloud-KMS kinds. Identity precedence: per-provider `identity`,
	// then the injected default (--identity/ATMOS_IDENTITY or the stack/component effective identity).
	var authResolver store.AuthContextResolver
	effectiveIdentity := def.identity
	if atmosConfig.SecretsAuth != nil {
		authResolver = atmosConfig.SecretsAuth.Resolver
		if effectiveIdentity == "" {
			effectiveIdentity = atmosConfig.SecretsAuth.DefaultIdentity
		}
	}

	kind := def.kind
	spec := def.spec
	// `spec.file` is the advanced template override. When it is omitted, the file path is derived
	// in code from each secret's scope under `spec.path` (default `secrets`), which is
	// collision-safe by construction (stack and instance secrets land in distinct files).
	file, _ := spec["file"].(string)
	path, _ := spec["path"].(string)
	if file == "" && path == "" {
		path = defaultSopsPath
	}

	ageRecipients, _ := spec["age_recipients"].(string)
	recipientsFile, _ := spec["recipients_file"].(string)

	// age_key may be a nested object ({store,path,value}) or a bare string (inline material).
	// age_key_file remains a back-compat shorthand for file-mode at an explicit path.
	ak := parseAgeKeySpec(spec)

	return &sopsProvider{
		name:              name,
		kind:              kind,
		file:              file,
		path:              path,
		ageRecipients:     ageRecipients,
		ageKeyFile:        ak.file,
		ageKey:            ak.value,
		ageKeyStore:       ak.storeName,
		ageKeyPath:        ak.path,
		recipientsFile:    recipientsFile,
		stores:            atmosConfig.Stores,
		authResolver:      authResolver,
		effectiveIdentity: effectiveIdentity,
	}, nil
}

// sopsProviderDef is a resolved SOPS provider definition (kind, optional auth identity, and spec).
type sopsProviderDef struct {
	kind     string
	identity string
	spec     map[string]any
}

// lookupSopsProvider reads a provider definition from a stack/component `secrets.providers`
// map: `{ providers: { <name>: { kind: ..., identity: ..., spec: { ... } } } }`.
func lookupSopsProvider(sectionProviders map[string]any, name string) (sopsProviderDef, bool) {
	if sectionProviders == nil {
		return sopsProviderDef{}, false
	}
	raw, found := sectionProviders[name].(map[string]any)
	if !found {
		return sopsProviderDef{}, false
	}
	def := sopsProviderDef{}
	def.kind, _ = raw["kind"].(string)
	def.identity, _ = raw["identity"].(string)
	def.spec, _ = raw["spec"].(map[string]any)
	if def.spec == nil {
		def.spec = map[string]any{}
	}
	return def, true
}

func (p *sopsProvider) Kind() string {
	defer perf.Track(nil, "providers.sopsProvider.Kind")()

	return p.kind
}

// SupportsScope reports the scopes SOPS can represent. Instance and stack work: the backing file
// path encodes the scope (a stack file shared by every instance, or a per-instance file). Global
// is NOT supported yet — file placement is scope-keyed and a stack/component-less file location
// has no derivation rule; revisit if demand appears.
func (p *sopsProvider) SupportsScope(scope providers.Scope) bool {
	defer perf.Track(nil, "providers.sopsProvider.SupportsScope")()

	return scope == "" || scope == providers.ScopeStack || scope == providers.ScopeInstance
}

// FilePath reports the backing file a coordinate resolves to, satisfying providers.FilePathProvider so
// `describe affected` can treat it as an implicit dependency.
func (p *sopsProvider) FilePath(coord providers.Coordinate) (string, error) {
	defer perf.Track(nil, "providers.sopsProvider.FilePath")()

	return p.resolveFile(coord)
}

// resolveFile returns the backing file for a coordinate. When `spec.file` is empty the path is
// derived in code from the coordinate's scope (collision-safe); otherwise the configured
// `spec.file` Go template is rendered, exposing `{{ .atmos_stack }}` / `{{ .atmos_component }}`
// (consistent with the rest of Atmos templating) plus sprig functions.
func (p *sopsProvider) resolveFile(coord providers.Coordinate) (string, error) {
	if p.file == "" {
		return p.derivePath(coord), nil
	}

	tmpl, err := template.New("sops-file").Funcs(sprig.FuncMap()).Option("missingkey=error").Parse(p.file)
	if err != nil {
		return "", fmt.Errorf("%w: invalid `spec.file` template %q: %w", ErrSopsFilePathTemplate, p.file, err)
	}

	data := map[string]any{
		"atmos_stack":     coord.Stack,
		"atmos_component": coord.Component,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("%w: rendering `spec.file` %q: %w", ErrSopsFilePathTemplate, p.file, err)
	}
	return buf.String(), nil
}

// derivePath computes the scope-aware file path under `spec.path` (default `secrets`):
//
//	stack scope    → <path>/<stack>.enc.yaml          (shared by every instance in the stack)
//	instance scope → <path>/<stack>.<instance>.enc.yaml
//
// The instance id may contain `/` (e.g. `vpc/primary`), which renders as nested subdirs. The two
// scopes can never collide because a stack file has exactly the stack segment while an instance
// file always carries an additional component segment.
func (p *sopsProvider) derivePath(coord providers.Coordinate) string {
	base := p.path
	if base == "" {
		base = defaultSopsPath
	}
	name := coord.Stack
	if coord.Scope != providers.ScopeStack && coord.Component != "" {
		name = coord.Stack + "." + coord.Component
	}
	return filepath.Join(base, name+sopsFileExt)
}

func (p *sopsProvider) Set(coord providers.Coordinate, value any) error {
	defer perf.Track(nil, "providers.sopsProvider.Set")()

	file, err := p.resolveFile(coord)
	if err != nil {
		return err
	}
	return p.editFile(file, true, func(branch sops.TreeBranch) sops.TreeBranch {
		return setBranchValue(branch, coord.Key, value)
	})
}

func (p *sopsProvider) Get(coord providers.Coordinate) (any, error) {
	defer perf.Track(nil, "providers.sopsProvider.Get")()

	file, err := p.resolveFile(coord)
	if err != nil {
		return nil, err
	}
	doc, err := p.decryptDoc(file)
	if err != nil {
		return nil, err
	}
	value, ok := doc[coord.Key]
	if !ok {
		return nil, fmt.Errorf(errFmtWrapQuoted, ErrSecretNotInitialized, coord.Key)
	}
	return value, nil
}

func (p *sopsProvider) Delete(coord providers.Coordinate) error {
	defer perf.Track(nil, "providers.sopsProvider.Delete")()

	file, err := p.resolveFile(coord)
	if err != nil {
		return err
	}
	// Deleting from a non-existent file (or a missing key) is idempotently "done".
	if _, statErr := os.Stat(file); errors.Is(statErr, fs.ErrNotExist) {
		return nil
	}
	return p.editFile(file, false, func(branch sops.TreeBranch) sops.TreeBranch {
		return removeBranchKey(branch, coord.Key)
	})
}

func (p *sopsProvider) Status(coord providers.Coordinate) (bool, error) {
	defer perf.Track(nil, "providers.sopsProvider.Status")()

	// "Initialized" means the secret's key is present in the encrypted file. SOPS YAML keeps
	// keys in cleartext (only values are ENC[...]), so we can answer this by parsing the
	// encrypted tree alone — no key service, no decryption, no authenticated identity. This
	// keeps `atmos secret list`/`validate` fast and credential-free; only `Get` decrypts.
	file, err := p.resolveFile(coord)
	if err != nil {
		return false, nil
	}
	present, err := p.keyExists(file, coord.Key)
	if err != nil {
		// Treat a missing or unreadable file as "not initialized" rather than a hard error,
		// matching the prior Get-based behavior.
		return false, nil
	}
	return present, nil
}

// LocalStatusCheck reports that SOPS Status() is credential-free: it answers "is the key set?"
// by reading the cleartext key names from the local encrypted file — no age key, no decryption,
// no authenticated identity. Implements the LocalStatus capability.
func (p *sopsProvider) LocalStatusCheck() bool {
	defer perf.Track(nil, "providers.sopsProvider.LocalStatusCheck")()

	return true
}

// keyExists reports whether the encrypted SOPS file has the given top-level key, without
// decrypting any values. It returns an error only when the file cannot be read or parsed.
func (p *sopsProvider) keyExists(file, key string) (bool, error) {
	enc, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, fileNotFoundErr(file)
		}
		return false, fmt.Errorf(errFmtWrapFile, ErrSopsDecrypt, file, err)
	}

	store := sopsyaml.NewStore(&sopsconfig.YAMLStoreConfig{})
	tree, err := store.LoadEncryptedFile(enc)
	if err != nil {
		return false, fmt.Errorf(errFmtWrapFile, ErrSopsDecrypt, file, err)
	}
	if len(tree.Branches) == 0 {
		return false, nil
	}
	return branchHasKey(tree.Branches[0], key), nil
}

// Reset overwrites the encrypted file with a clean, empty document (creating it if missing),
// re-using the file's recipients (from `spec.age_recipients` or the matching `.sops.yaml`
// creation rule). It implements the providers.FileResettable capability.
func (p *sopsProvider) Reset(coord providers.Coordinate) error {
	defer perf.Track(nil, "providers.sopsProvider.Reset")()

	file, err := p.resolveFile(coord)
	if err != nil {
		return err
	}
	return p.writeNewFile(file, sops.TreeBranch{})
}

// decryptDoc decrypts the whole SOPS file and returns its top-level document as a map. It loads
// the encrypted tree, decrypts the data key via the provider's key service (which honors
// `spec.age_key_file` when set, otherwise SOPS_AGE_KEY_FILE/SOPS_AGE_KEY), verifies the file MAC,
// and emits the cleartext YAML to unmarshal — mirroring the structure of the on-disk document.
func (p *sopsProvider) decryptDoc(file string) (map[string]any, error) {
	enc, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fileNotFoundErr(file)
		}
		return nil, fmt.Errorf(errFmtWrapFile, ErrSopsDecrypt, file, err)
	}

	store := sopsyaml.NewStore(&sopsconfig.YAMLStoreConfig{})
	tree, err := store.LoadEncryptedFile(enc)
	if err != nil {
		return nil, fmt.Errorf(errFmtWrapFile, ErrSopsDecrypt, file, err)
	}

	client, err := p.keyClient()
	if err != nil {
		return nil, err
	}
	if _, err := decryptTree(&tree, sopsaes.NewCipher(), client); err != nil {
		return nil, p.decryptErr(file, &tree, err)
	}

	clear, err := store.EmitPlainFile(tree.Branches)
	if err != nil {
		return nil, fmt.Errorf(errFmtWrapFile, ErrSopsDecrypt, file, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(clear, &doc); err != nil {
		return nil, fmt.Errorf(errFmtWrapFile, ErrSopsDecrypt, file, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

// editFile loads an encrypted file, decrypts it, applies mutate to the top-level branch, and
// re-encrypts in place with the same data key. When createIfMissing is set and the file does not
// exist, it is created fresh from the mutation applied to an empty document.
func (p *sopsProvider) editFile(file string, createIfMissing bool, mutate func(sops.TreeBranch) sops.TreeBranch) error {
	enc, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		if !createIfMissing {
			return fileNotFoundErr(file)
		}
		return p.writeNewFile(file, mutate(sops.TreeBranch{}))
	}
	if err != nil {
		return fmt.Errorf(errFmtWrapFile, ErrSopsDecrypt, file, err)
	}

	store := sopsyaml.NewStore(&sopsconfig.YAMLStoreConfig{})
	tree, err := store.LoadEncryptedFile(enc)
	if err != nil {
		return fmt.Errorf(errFmtWrapFile, ErrSopsDecrypt, file, err)
	}

	client, err := p.keyClient()
	if err != nil {
		return err
	}
	cipher := sopsaes.NewCipher()
	dataKey, err := decryptTree(&tree, cipher, client)
	if err != nil {
		return p.decryptErr(file, &tree, err)
	}

	if len(tree.Branches) == 0 {
		tree.Branches = sops.TreeBranches{sops.TreeBranch{}}
	}
	tree.Branches[0] = mutate(tree.Branches[0])

	if err := encryptTree(&tree, dataKey, cipher); err != nil {
		return fmt.Errorf(errFmtWrapFile, ErrSopsEncrypt, file, err)
	}

	out, err := store.EmitEncryptedFile(tree)
	if err != nil {
		return fmt.Errorf(errFmtWrapFile, ErrSopsEncrypt, file, err)
	}
	log.Debug("Wrote SOPS-encrypted file in-process", "file", file)
	return os.WriteFile(file, out, secretFileMode)
}

// writeNewFile encrypts a brand-new document (one top-level branch) and writes it to file,
// generating a fresh data key from the resolved recipients.
func (p *sopsProvider) writeNewFile(file string, branch sops.TreeBranch) error {
	keyGroups, err := p.keyGroups(file)
	if err != nil {
		return err
	}

	tree := sops.Tree{
		Branches: sops.TreeBranches{branch},
		Metadata: sops.Metadata{
			KeyGroups: keyGroups,
			Version:   sopsversion.Version,
		},
		FilePath: file,
	}

	// Generate the data key via the provider's key service so cloud-KMS recipients (AWS/GCP/Azure)
	// are encrypted using the Atmos identity's credentials rather than the ambient credential chain.
	dataKey, errs := tree.GenerateDataKeyWithKeyServices([]keyservice.KeyServiceClient{p.encryptKeyClient()})
	if len(errs) > 0 {
		return fmt.Errorf(errFmtWrapFile, ErrSopsEncrypt, file, errors.Join(errs...))
	}
	if err := encryptTree(&tree, dataKey, sopsaes.NewCipher()); err != nil {
		return fmt.Errorf(errFmtWrapFile, ErrSopsEncrypt, file, err)
	}

	store := sopsyaml.NewStore(&sopsconfig.YAMLStoreConfig{})
	out, err := store.EmitEncryptedFile(tree)
	if err != nil {
		return fmt.Errorf(errFmtWrapFile, ErrSopsEncrypt, file, err)
	}
	if err := os.MkdirAll(filepath.Dir(file), secretDirMode); err != nil {
		return fmt.Errorf(errFmtWrapFile, ErrSopsEncrypt, file, err)
	}
	log.Debug("Created SOPS-encrypted file in-process", "file", file)
	return os.WriteFile(file, out, secretFileMode)
}

// keyGroups resolves the encryption recipients for a fresh file: explicit `spec.age_recipients`
// first, otherwise the matching creation rule in the nearest `.sops.yaml`.
func (p *sopsProvider) keyGroups(file string) ([]sops.KeyGroup, error) {
	if p.ageRecipients != "" {
		masterKeys, err := sopsage.MasterKeysFromRecipients(p.ageRecipients)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSopsRecipients, err)
		}
		var group sops.KeyGroup
		for _, mk := range masterKeys {
			group = append(group, mk)
		}
		return []sops.KeyGroup{group}, nil
	}

	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSopsRecipients, err)
	}
	confPath, err := sopsconfig.FindConfigFile(filepath.Dir(abs))
	if err != nil {
		return nil, fmt.Errorf("%w: no `spec.age_recipients` and no .sops.yaml found for %q: %w", ErrSopsRecipients, file, err)
	}
	conf, err := sopsconfig.LoadCreationRuleForFile(confPath, abs, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSopsRecipients, err)
	}
	if conf == nil || len(conf.KeyGroups) == 0 {
		return nil, fmt.Errorf("%w: no creation rule in %q matches %q", ErrSopsRecipients, confPath, file)
	}
	return conf.KeyGroups, nil
}

// fileNotFoundErr reports a missing SOPS file as ErrSecretFileNotFound with actionable hints on how
// to initialize it. The errors.Is(result, ErrSecretFileNotFound) check still holds.
func fileNotFoundErr(file string) error {
	return errUtils.Build(ErrSecretFileNotFound).
		WithCause(fmt.Errorf(errFmtWrapQuoted, ErrSecretFileNotFound, file)).
		WithHint("Initialize secrets to create the encrypted file: `atmos secret set <NAME>=<value> --stack <stack> --component <component>` (or `atmos secret init`).").
		WithHint("On first write Atmos creates and encrypts the file in-process using `spec.age_recipients` or a matching .sops.yaml creation rule — no `sops` binary required.").
		Err()
}

// ageKeyFileErr reports a `spec.age_key_file` that could not be read/parsed as ErrSopsAgeKeyFile with
// actionable hints on how to provide/generate the key. The errors.Is(result, ErrSopsAgeKeyFile) check holds.
func ageKeyFileErr(path string, cause error) error {
	return errUtils.Build(ErrSopsAgeKeyFile).
		WithCause(fmt.Errorf("%q: %w", path, cause)).
		WithHint("Point `spec.age_key_file` at your age PRIVATE key file (the line starting with `AGE-SECRET-KEY-1...`).").
		WithHint("No key yet? Generate one with `age-keygen -o keys.txt`; its `# public key:` line is the `spec.age_recipients` used to encrypt.").
		Err()
}

// ageKeyErr reports inline `spec.age_key` material that could not be parsed as ErrSopsAgeKey, with
// actionable hints. The errors.Is(result, ErrSopsAgeKey) check holds.
func ageKeyErr(cause error) error {
	return errUtils.Build(ErrSopsAgeKey).
		WithCause(cause).
		WithHint("`spec.age_key` must be age PRIVATE key material (the line starting with `AGE-SECRET-KEY-1...`).").
		WithHint("Populate it from a source instead of inline plaintext, e.g. `age_key: !exec atmos secret get SOPS_AGE_KEY` or a `!store.get <store> <KEY>` reference.").
		Err()
}

// decryptErr wraps a SOPS data-key/MAC decryption failure as ErrSopsDecrypt with actionable hints.
// Hints are derived from the file's actual master-key types (not a declared kind): a cloud-KMS file
// gets identity/permission hints, an age file gets age-key hints. The errors.Is(result,
// ErrSopsDecrypt) check still holds.
func (p *sopsProvider) decryptErr(file string, tree *sops.Tree, cause error) error {
	b := errUtils.Build(ErrSopsDecrypt).WithCause(fmt.Errorf("%q: %w", file, cause))

	cloudType, hasAge := sopsKeyTypes(tree)

	// Cloud-KMS files fail on credentials/permissions, not age keys — give identity-oriented hints.
	if cloudType != "" {
		if p.effectiveIdentity != "" {
			b = b.WithHintf("Ensure the identity %q is allowed to decrypt with this file's %s key (e.g. kms:Decrypt / equivalent IAM permission).", p.effectiveIdentity, cloudKeyTypeName(cloudType))
			b = b.WithHintf("Verify the identity is correct via `secrets.providers.%s.identity`, `--identity`, or `ATMOS_IDENTITY`.", p.name)
		} else {
			b = b.WithHintf("Set an identity so Atmos can authenticate the %s decrypt: `secrets.providers.%s.identity`, `--identity`, or `ATMOS_IDENTITY` (or run inside `atmos auth exec`).", cloudKeyTypeName(cloudType), p.name)
		}
	}

	// Age files (or files whose key types could not be determined) get age-key hints.
	if hasAge || cloudType == "" {
		if p.ageKeyFile != "" {
			b = b.WithHintf("Ensure `spec.age_key_file` (%s) holds the age private key matching this file's recipients.", p.ageKeyFile)
		} else {
			b = b.WithHint("Provide the age private key: set `spec.age_key_file` in the provider config, or export SOPS_AGE_KEY_FILE / SOPS_AGE_KEY.")
		}
		b = b.WithHint("Generate an age key with `age-keygen -o keys.txt`; its `# public key:` line is the `spec.age_recipients` used to encrypt.")
	}
	return b.Err()
}

// sopsKeyTypes inspects an encrypted tree's master keys and reports the first cloud-KMS key-type
// identifier present (kms/gcp_kms/azure_kv, empty if none) and whether an age key is present. It lets
// decryptErr tailor hints to the file's real recipients instead of a declared provider kind.
func sopsKeyTypes(tree *sops.Tree) (cloudType string, hasAge bool) {
	if tree == nil {
		return "", false
	}
	for _, group := range tree.Metadata.KeyGroups {
		for _, mk := range group {
			switch mk.TypeToIdentifier() {
			case keyTypeAWSKMS, keyTypeGCPKMS, keyTypeAzureKV:
				if cloudType == "" {
					cloudType = mk.TypeToIdentifier()
				}
			case keyTypeAge:
				hasAge = true
			}
		}
	}
	return cloudType, hasAge
}

// keyClient returns the key service used to decrypt the SOPS data key. The age private key is
// sourced, in precedence order, from inline material (`spec.age_key` — populated by any YAML
// function such as `!env`/`!exec`/a lazily-resolved `!store.get`), a key file (`spec.age_key_file`),
// or — when neither is set — the default local client, which resolves the key from
// SOPS_AGE_KEY_FILE/SOPS_AGE_KEY (unchanged, backward-compatible behavior). The key text is
// injected via an identity-aware key service (no process-environment mutation).
func (p *sopsProvider) keyClient() (keyservice.KeyServiceClient, error) {
	// Decryption needs the age PRIVATE key (when age recipients are used), so the base client resolves
	// it from configured material / SOPS_AGE_KEY*. Cloud-KMS key types are wrapped to authenticate via
	// the identity. Everything else delegates to the base client.
	base, err := p.ageKeyClient()
	if err != nil {
		return nil, err
	}
	return p.wrapCloudKeyService(base), nil
}

// encryptKeyClient returns the key service used to encrypt a fresh data key (writeNewFile). Unlike
// decryption, encryption never needs the age PRIVATE key — age encrypts to its public recipients —
// so the base is the plain local client (no private-key resolution, which would fail when only the
// recipients are configured). Cloud-KMS key types are still wrapped to encrypt via the identity.
func (p *sopsProvider) encryptKeyClient() keyservice.KeyServiceClient {
	return p.wrapCloudKeyService(keyservice.NewLocalClient())
}

// wrapCloudKeyService wraps a base key service so cloud-KMS key types (AWS/GCP/Azure, inferred from
// the file's recipients at runtime — not from a declared kind) authenticate as the resolved identity
// and inject its credentials. Without a resolvable identity the base client is returned unchanged,
// preserving the ambient-credential fallback (issue #2637).
func (p *sopsProvider) wrapCloudKeyService(base keyservice.KeyServiceClient) keyservice.KeyServiceClient {
	if p.authResolver == nil || p.effectiveIdentity == "" {
		return base
	}
	return &sopsKeyServiceClient{
		builder:  sopsauth.NewBuilder(p.authResolver),
		identity: p.effectiveIdentity,
		fallback: base,
	}
}

// ageKeyClient returns the key service for age and other local key types. The age private key is
// sourced, in precedence order, from inline material (`spec.age_key`), a configured store
// (`age_key.store`), a key file (`spec.age_key_file`), or — when none is set — the default local
// client, which resolves the key from SOPS_AGE_KEY_FILE/SOPS_AGE_KEY. Keys are injected via an
// identity-aware key service (no process-environment mutation).
func (p *sopsProvider) ageKeyClient() (keyservice.KeyServiceClient, error) {
	switch {
	case p.ageKey != "":
		return ageClientFromKeyMaterial(p.ageKey, ageKeyErr)
	case p.ageKeyStore != "":
		return p.ageKeyStoreClient()
	case p.ageKeyFile != "":
		return p.ageKeyFileClient()
	default:
		return keyservice.NewLocalClient(), nil
	}
}

// ageKeyFileClient builds a key service from the age private key in `spec.age_key_file`.
func (p *sopsProvider) ageKeyFileClient() (keyservice.KeyServiceClient, error) {
	path, err := expandKeyPath(p.ageKeyFile)
	if err != nil {
		return nil, ageKeyFileErr(p.ageKeyFile, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, ageKeyFileErr(path, err)
	}
	return ageClientFromKeyMaterial(string(content), func(e error) error { return ageKeyFileErr(path, e) })
}

// ageClientFromKeyMaterial parses age private-key text into an identity-injecting key service.
func ageClientFromKeyMaterial(material string, wrapErr func(error) error) (keyservice.KeyServiceClient, error) {
	var identities sopsage.ParsedIdentities
	if err := identities.Import(material); err != nil {
		return nil, wrapErr(err)
	}
	return ageKeyServiceClient{fallback: keyservice.NewLocalClient(), identities: identities}, nil
}

// expandKeyPath expands `$ENV`/`${ENV}` references and a leading `~` in the configured key path.
func expandKeyPath(path string) (string, error) {
	expanded, err := homedir.Expand(os.ExpandEnv(path))
	if err != nil {
		return "", err
	}
	return expanded, nil
}

// ageKeyServiceClient is a keyservice client that injects config-provided age identities for age
// decrypt requests and delegates everything else (encrypt, and non-age key types) to the default
// local key service. This is the getsops-recommended, thread-safe way to supply identities without
// mutating process environment variables.
type ageKeyServiceClient struct {
	fallback   keyservice.KeyServiceClient
	identities sopsage.ParsedIdentities
}

func (c ageKeyServiceClient) Decrypt(ctx context.Context, req *keyservice.DecryptRequest, opts ...grpc.CallOption) (*keyservice.DecryptResponse, error) {
	defer perf.Track(nil, "providers.ageKeyServiceClient.Decrypt")()

	if k, ok := req.Key.KeyType.(*keyservice.Key_AgeKey); ok {
		mk := sopsage.MasterKey{Recipient: k.AgeKey.Recipient}
		mk.EncryptedKey = string(req.Ciphertext)
		c.identities.ApplyToMasterKey(&mk)
		plaintext, err := mk.Decrypt()
		if err != nil {
			return nil, err
		}
		return &keyservice.DecryptResponse{Plaintext: plaintext}, nil
	}
	return c.fallback.Decrypt(ctx, req, opts...)
}

func (c ageKeyServiceClient) Encrypt(ctx context.Context, req *keyservice.EncryptRequest, opts ...grpc.CallOption) (*keyservice.EncryptResponse, error) {
	defer perf.Track(nil, "providers.ageKeyServiceClient.Encrypt")()

	return c.fallback.Encrypt(ctx, req, opts...)
}

// decryptTree decrypts the data key (via the supplied key service) and the tree's values in place,
// verifying the file MAC. Inlined from getsops/sops to avoid importing the heavy cmd/sops/common
// package (which pulls in urfave/cli and every store format). The key service honors
// `spec.age_key_file` when configured, otherwise the age keysource's env vars.
func decryptTree(tree *sops.Tree, cipher sops.Cipher, client keyservice.KeyServiceClient) ([]byte, error) {
	dataKey, err := tree.Metadata.GetDataKeyWithKeyServices(
		[]keyservice.KeyServiceClient{client}, nil,
	)
	if err != nil {
		return nil, err
	}
	computedMac, err := tree.Decrypt(dataKey, cipher)
	if err != nil {
		return nil, err
	}
	fileMac, err := cipher.Decrypt(
		tree.Metadata.MessageAuthenticationCode, dataKey, tree.Metadata.LastModified.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt MAC: %w", err)
	}
	if fileMac != computedMac {
		return nil, fmt.Errorf("%w: file has %v, computed %v", ErrSopsMacMismatch, fileMac, computedMac)
	}
	return dataKey, nil
}

// encryptTree encrypts the tree's values in place and recomputes the file MAC. Inlined from
// getsops/sops (see decryptTree).
func encryptTree(tree *sops.Tree, dataKey []byte, cipher sops.Cipher) error {
	unencryptedMac, err := tree.Encrypt(dataKey, cipher)
	if err != nil {
		return err
	}
	tree.Metadata.LastModified = time.Now().UTC()
	tree.Metadata.MessageAuthenticationCode, err = cipher.Encrypt(
		unencryptedMac, dataKey, tree.Metadata.LastModified.Format(time.RFC3339),
	)
	return err
}

// setBranchValue sets (or inserts) a top-level key in a tree branch.
func setBranchValue(branch sops.TreeBranch, key string, value any) sops.TreeBranch {
	for i := range branch {
		if fmt.Sprint(branch[i].Key) == key {
			branch[i].Value = value
			return branch
		}
	}
	return append(branch, sops.TreeItem{Key: key, Value: value})
}

// branchHasKey reports whether the branch contains the given top-level key. It compares the
// cleartext key only — it never inspects the (encrypted) value — so it is safe to call on an
// undecrypted SOPS tree.
func branchHasKey(branch sops.TreeBranch, key string) bool {
	for i := range branch {
		if fmt.Sprint(branch[i].Key) == key {
			return true
		}
	}
	return false
}

// removeBranchKey returns a branch with the given top-level key removed.
func removeBranchKey(branch sops.TreeBranch, key string) sops.TreeBranch {
	result := make(sops.TreeBranch, 0, len(branch))
	for _, item := range branch {
		if fmt.Sprint(item.Key) != key {
			result = append(result, item)
		}
	}
	return result
}
