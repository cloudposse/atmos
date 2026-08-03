package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/cloudposse/atmos/pkg/github/oidc"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/store"
)

// githubSecretNameRe matches a valid GitHub Actions secret name: uppercase letters, digits and
// underscores, not starting with a digit. GitHub additionally forbids the GITHUB_ prefix, checked
// separately so the error can explain it.
var githubSecretNameRe = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// GitHubActionsCIOptions gates value reads for a GitHub Actions store.
type GitHubActionsCIOptions struct {
	// Enabled forces value reads (Get) on even when GitHub Actions is not auto-detected. By
	// default reads are allowed only inside a GitHub Actions runner (see actions.IsGitHubActions).
	Enabled bool `mapstructure:"enabled"`
}

// GitHubActionsStoreOptions configures a GitHub Actions secrets store. Secrets are written, listed,
// and deleted through the GitHub API (anywhere a token is available), but their *values* can only
// be read back inside a GitHub Actions runner, where GitHub injects the secret into the
// environment. Addressing is flat: a secret is named [PREFIX_]KEY (uppercased), repo-global, so the
// same key resolves to the same GitHub secret across stacks/components.
type GitHubActionsStoreOptions struct {
	// Owner is the repository owner (org or user). Required.
	Owner string `mapstructure:"owner"`
	// Repo is the repository name. Required.
	Repo string `mapstructure:"repo"`
	// Environment optionally targets environment-level secrets instead of repository secrets.
	Environment string `mapstructure:"environment"`
	// Prefix is an optional name prefix applied before the key (e.g. prefix "atmos" + key
	// "db_password" → secret "ATMOS_DB_PASSWORD").
	Prefix string `mapstructure:"prefix"`
	// Token optionally overrides the GitHub token; when empty the standard Atmos resolution chain
	// is used (--github-token → ATMOS_GITHUB_TOKEN → GITHUB_TOKEN → `gh auth token`).
	Token string `mapstructure:"token"`
	// CI gates value reads (Get).
	CI GitHubActionsCIOptions `mapstructure:"ci"`
}

// GitHubActionsStore implements the store.Store interface backed by GitHub Actions secrets. It is a
// "native CI" store: Set/Has/Delete go through the GitHub API, while Get reads the value from the
// process environment (only populated inside a runner) and is gated by CI detection.
type GitHubActionsStore struct {
	options GitHubActionsStoreOptions
	prefix  string

	// isCI reports whether value reads are permitted by environment detection; overridable in tests.
	isCI func() bool

	// The client is built lazily on first API use so that merely declaring a store does not require
	// a token at config-load time. Tests inject a client directly, which the lazy initializer preserves.
	once   sync.Once
	client gitHubActionsClient

	// alignOnce guards the one-time, best-effort runner/store alignment warning emitted on first read.
	alignOnce sync.Once
}

// Ensure GitHubActionsStore implements the expected interfaces.
var (
	_ store.Store          = (*GitHubActionsStore)(nil)
	_ store.StatusStore    = (*GitHubActionsStore)(nil)
	_ store.DeletableStore = (*GitHubActionsStore)(nil)
)

func init() {
	store.Register(store.KindGitHubActions, buildGitHubActionsStore)
}

// buildGitHubActionsStore is the store.StoreFactory for GitHub Actions stores.
func buildGitHubActionsStore(key string, storeConfig store.StoreConfig) (store.Store, error) {
	var opts GitHubActionsStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errParseFmt, store.ErrParseGitHubActionsOptions, err)
	}
	store.WarnIdentityIgnored(key, storeConfig, "GitHub Actions")
	return NewGitHubActionsStore(&opts)
}

// NewGitHubActionsStore initializes a GitHub Actions secrets store. The GitHub client is built
// lazily on first API call, so this only validates required addressing options.
func NewGitHubActionsStore(options *GitHubActionsStoreOptions) (store.Store, error) {
	if options.Owner == "" || options.Repo == "" {
		return nil, store.ErrGitHubOwnerRepoRequired
	}
	return &GitHubActionsStore{
		options: *options,
		prefix:  options.Prefix,
		isCI:    isGitHubActionsRunner,
	}, nil
}

// isGitHubActionsRunner reports whether the process is running inside a GitHub Actions runner.
// GitHub sets GITHUB_ACTIONS=true in every workflow job. It is inlined here (rather than reusing
// pkg/github/actions) to avoid an import cycle (that package transitively imports pkg/store).
func isGitHubActionsRunner() bool {
	// GITHUB_ACTIONS is an external CI signal set by GitHub, not Atmos configuration.
	//nolint:forbidigo // GITHUB_ACTIONS is an external CI env var, not Atmos config.
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// getClient lazily builds the GitHub client, preserving an already-injected client (tests).
func (s *GitHubActionsStore) getClient() gitHubActionsClient {
	s.once.Do(func() {
		if s.client == nil {
			s.client = newGitHubActionsAPIClient(&s.options)
		}
	})
	return s.client
}

// readAllowed reports whether secret values may be read from the environment: true inside a GitHub
// Actions runner, or when explicitly forced via options.ci.enabled.
func (s *GitHubActionsStore) readAllowed() bool {
	return s.options.CI.Enabled || s.isCI()
}

// Set encrypts and writes a secret value via the GitHub API. The stack and component do not affect
// the secret name (GitHub secrets are a flat, repo-global namespace).
func (s *GitHubActionsStore) Set(_ string, _ string, key string, value any) error {
	if key == "" {
		return store.ErrEmptyKey
	}
	if value == nil {
		return fmt.Errorf("%w for key %s", store.ErrNilValue, key)
	}
	name, err := githubSecretName(s.prefix, key)
	if err != nil {
		return err
	}
	strValue, err := githubSecretStringValue(value)
	if err != nil {
		return err
	}
	return s.getClient().PutSecret(context.TODO(), name, strValue)
}

// Get returns the secret value from the process environment. This only works inside a GitHub
// Actions runner (where GitHub injects the secret), gated by CI detection; the GitHub API never
// exposes secret values.
func (s *GitHubActionsStore) Get(_ string, _ string, key string) (any, error) {
	return s.getByKey(key)
}

// GetKey returns the secret value for a raw key without stack/component context (same env-read
// semantics as Get).
func (s *GitHubActionsStore) GetKey(key string) (any, error) {
	return s.getByKey(key)
}

func (s *GitHubActionsStore) getByKey(key string) (any, error) {
	name, err := githubSecretName(s.prefix, key)
	if err != nil {
		return nil, err
	}
	if !s.readAllowed() {
		return nil, fmt.Errorf("%w: %q is only readable inside a GitHub Actions runner — %s, or set options.ci.enabled to override",
			store.ErrGitHubSecretValueCIOnly, name, s.envHint(name))
	}
	s.alignOnce.Do(s.verifyAlignment)
	raw, ok := lookupGitHubSecretEnv(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q — %s", store.ErrGitHubSecretNotInEnv, name, s.envHint(name))
	}
	return decodeGitHubSecretValue(raw), nil
}

// envHint describes how to make the secret available in the runner environment, naming the
// configured environment when one is set.
func (s *GitHubActionsStore) envHint(name string) string {
	if s.options.Environment != "" {
		return fmt.Sprintf("ensure the job declares `environment: %s` and maps `secrets.%s` into `env:`", s.options.Environment, name)
	}
	return fmt.Sprintf("map `secrets.%s` into the job `env:`", name)
}

// verifyAlignment emits warnings (never errors) when the runner context does not match the store
// config. It runs once per store (guarded by alignOnce): a cheap GITHUB_REPOSITORY check always,
// and — only when an environment is configured — a best-effort OIDC repository/environment check
// (which needs `id-token: write`; silently skipped when unavailable).
func (s *GitHubActionsStore) verifyAlignment() {
	wantRepo := s.options.Owner + "/" + s.options.Repo

	if repo := githubRepositoryEnv(); repo != "" && repo != wantRepo {
		log.Warn("github/actions store repository does not match the current GitHub Actions repository; injected secrets come from a different repo",
			"store_repo", wantRepo, "github_repository", repo)
	}

	if s.options.Environment != "" {
		s.verifyEnvironmentAlignment(wantRepo)
	}
}

// verifyEnvironmentAlignment best-effort verifies the OIDC token's repository/environment claims
// against the store config (used only when an environment is configured). It needs the job to grant
// `id-token: write`; when no token is available the check is skipped silently. Warn-only.
func (s *GitHubActionsStore) verifyEnvironmentAlignment(wantRepo string) {
	claims, available, err := oidc.RequestClaims(context.TODO())
	if err != nil {
		log.Debug("could not verify GitHub environment via OIDC", "error", err)
		return
	}
	if !available {
		log.Debug("skipping GitHub environment verification: no OIDC token (the job may lack `id-token: write`)",
			"environment", s.options.Environment)
		return
	}
	if claims.Repository != "" && claims.Repository != wantRepo {
		log.Warn("github/actions store repository does not match the OIDC token repository claim",
			"store_repo", wantRepo, "oidc_repository", claims.Repository)
	}
	switch claims.Environment {
	case s.options.Environment:
		// Aligned: nothing to warn about.
	case "":
		log.Warn("github/actions store targets an environment but the job is not bound to one; environment secrets will not be injected",
			"store_environment", s.options.Environment)
	default:
		log.Warn("github/actions store environment does not match the job's GitHub environment",
			"store_environment", s.options.Environment, "job_environment", claims.Environment)
	}
}

// githubRepositoryEnv returns the current GitHub Actions repository (owner/repo), if set.
func githubRepositoryEnv() string {
	// GITHUB_REPOSITORY is an external CI signal set by GitHub, not Atmos configuration.
	//nolint:forbidigo // GITHUB_REPOSITORY is an external CI env var, not Atmos config.
	return os.Getenv("GITHUB_REPOSITORY")
}

// Has reports whether the secret exists, via the GitHub API (metadata only, no value, no CI
// context). A missing secret returns (false, nil); auth/transport errors propagate.
func (s *GitHubActionsStore) Has(_ string, _ string, key string) (bool, error) {
	name, err := githubSecretName(s.prefix, key)
	if err != nil {
		return false, err
	}
	return s.getClient().HasSecret(context.TODO(), name)
}

// Delete removes the secret via the GitHub API. It is idempotent: a missing secret is not an error.
func (s *GitHubActionsStore) Delete(_ string, _ string, key string) error {
	name, err := githubSecretName(s.prefix, key)
	if err != nil {
		return err
	}
	return s.getClient().DeleteSecret(context.TODO(), name)
}

// lookupGitHubSecretEnv reads a GitHub-injected secret from the process environment. The secret
// arrives as a workflow-mapped environment variable (secrets.NAME → env NAME), not as Atmos config.
func lookupGitHubSecretEnv(name string) (string, bool) {
	return os.LookupEnv(name)
}

// githubSecretName builds a GitHub-valid secret name from an optional prefix and the key, then
// validates it. Lowercase becomes uppercase and disallowed characters become underscores; names
// that would start with a digit or the GITHUB_ prefix are rejected with a clear error.
func githubSecretName(prefix, key string) (string, error) {
	if key == "" {
		return "", store.ErrEmptyKey
	}
	raw := key
	if prefix != "" {
		raw = prefix + "_" + key
	}
	name := toEnvIdentifier(raw)
	if strings.HasPrefix(name, "GITHUB_") {
		return "", fmt.Errorf("%w: %q (names must not start with the GITHUB_ prefix)", store.ErrGitHubInvalidSecretName, name)
	}
	if !githubSecretNameRe.MatchString(name) {
		return "", fmt.Errorf("%w: %q (must match [A-Z_][A-Z0-9_]*)", store.ErrGitHubInvalidSecretName, name)
	}
	return name, nil
}

// toEnvIdentifier uppercases s and replaces any character outside [A-Z0-9_] with an underscore.
func toEnvIdentifier(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(unicode.ToUpper(r))
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// githubSecretStringValue converts a secret value to the string written to GitHub. Strings and byte
// slices pass through; other types are JSON-encoded.
func githubSecretStringValue(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		b, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf(errFormat, store.ErrSerializeJSON, err)
		}
		return string(b), nil
	}
}

// decodeGitHubSecretValue best-effort decodes an environment value as JSON (so structured secrets
// round-trip and support path extraction), falling back to the raw string for plain values.
func decodeGitHubSecretValue(raw string) any {
	var result any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return raw
	}
	return result
}
