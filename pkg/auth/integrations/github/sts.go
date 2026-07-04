// Package github implements the github/sts auth integration: a just-in-time,
// least-privilege GitHub token broker for CI. It is the Git-credentials analog of the
// aws/ecr integration (Execute persists secret material to a deterministic on-disk
// location; Environment returns a pointer to it; Cleanup removes and revokes it).
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	proapi "github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/xdg"
)

func init() {
	integrations.Register(integrations.KindGitHubSTS, NewGitHubSTSIntegration)
}

const (
	// GitConfigModeEnv injects per-owner insteadOf rewrites inline via GIT_CONFIG_KEY_n/VALUE_n.
	GitConfigModeEnv = "env"
	// GitConfigModeFile writes a 0600 gitconfig and emits include.path to it (tokens stay off the env).
	GitConfigModeFile = "file"

	defaultPolicyName = "default"
	gitTokenUser      = "x-access-token"

	// Default env var the minted token is exported under when token_env is unset. It bridges the
	// single-owner token to non-git consumers (gh/REST) and to Atmos's in-process git detector
	// (pkg/downloader resolveToken reads ATMOS_PRO_GITHUB_TOKEN live), so github/sts composes with
	// the default settings.inject_github_token: true without a workaround.
	defaultTokenEnv = "ATMOS_PRO_GITHUB_TOKEN"

	stateFileName  = "state.json"
	configFileName = "git.config"
	dirPerms       = 0o700
	filePerms      = 0o600

	httpTimeoutSecs = 30

	logKeyIntegration = "integration"
	// Safe replacement char for filesystem path segments.
	fsReplacement = "_"
)

// githubAPIBaseURL is the GitHub REST API base used for token revocation. Overridable in tests.
var githubAPIBaseURL = "https://api.github.com"

// stsHTTPClient is the HTTP client used for the STS mint request. Overridable in tests.
var stsHTTPClient = &http.Client{Timeout: httpTimeoutSecs * time.Second}

// revokeHTTPClient is the HTTP client used for token revocation. Overridable in tests.
var revokeHTTPClient = &http.Client{Timeout: httpTimeoutSecs * time.Second}

// stsDataSubdir is the XDG data subdirectory root for github/sts state.
var stsDataSubdir = filepath.Join("auth", "github-sts")

// stsRequest is the POST /api/v1/sts request body. Both fields are optional.
// Identity is derived server-side — never send owner/repo.
type stsRequest struct {
	Sources    []string `json:"sources,omitempty"`
	PolicyName string   `json:"policyName,omitempty"`
}

// stsToken is one minted token (one per installation/permission-set).
type stsToken struct {
	Host         string            `json:"host"`
	Owner        string            `json:"owner"`
	Token        string            `json:"token"`
	ExpiresAt    string            `json:"expiresAt"`
	Repositories []string          `json:"repositories,omitempty"`
	Permissions  map[string]string `json:"permissions,omitempty"`
}

// stsExclusion is one denied repo/owner with a verbatim reason.
type stsExclusion struct {
	Repo   string `json:"repo"`
	Reason string `json:"reason"`
}

// stsResponse is the POST /api/v1/sts response body.
type stsResponse struct {
	Tokens   []stsToken     `json:"tokens"`
	Excluded []stsExclusion `json:"excluded,omitempty"`
}

// gitSTSState is the persisted, realm-scoped state used by Environment and Cleanup.
type gitSTSState struct {
	Tokens []stsToken `json:"tokens"`
}

// GitHubSTSIntegration implements the github/sts integration type.
type GitHubSTSIntegration struct {
	name          string
	identity      string
	provider      string
	repos         []string
	policyName    string
	gitConfigMode string
	tokenEnv      string
	realm         string
}

// NewGitHubSTSIntegration creates a github/sts integration from config.
func NewGitHubSTSIntegration(config *integrations.IntegrationConfig) (integrations.Integration, error) {
	defer perf.Track(nil, "github.NewGitHubSTSIntegration")()

	if config == nil || config.Config == nil {
		return nil, fmt.Errorf("%w: integration config is nil", errUtils.ErrIntegrationNotFound)
	}

	identity, provider, err := resolveVia(config.Config.Via)
	if err != nil {
		return nil, fmt.Errorf("%w: integration %q: %w", errUtils.ErrIntegrationFailed, config.Name, err)
	}

	parsed, err := parseSTSSpec(config)
	if err != nil {
		return nil, err
	}

	return &GitHubSTSIntegration{
		name:          config.Name,
		identity:      identity,
		provider:      provider,
		repos:         parsed.repos,
		policyName:    parsed.policyName,
		gitConfigMode: parsed.gitConfigMode,
		tokenEnv:      parsed.tokenEnv,
		realm:         config.Realm,
	}, nil
}

// stsSpec holds the parsed github/sts spec fields with defaults applied.
type stsSpec struct {
	repos         []string
	policyName    string
	gitConfigMode string
	tokenEnv      string
}

// parseSTSSpec extracts and validates the github/sts spec fields, applying defaults.
func parseSTSSpec(config *integrations.IntegrationConfig) (*stsSpec, error) {
	parsed := &stsSpec{policyName: defaultPolicyName, gitConfigMode: GitConfigModeEnv, tokenEnv: defaultTokenEnv}

	spec := config.Config.Spec
	if spec == nil {
		return parsed, nil
	}

	if spec.PolicyName != "" {
		parsed.policyName = spec.PolicyName
	}
	parsed.repos = spec.Repos
	// An explicit token_env (including "{{ .owner }}" templates) overrides the default; empty keeps the default.
	if spec.TokenEnv != "" {
		parsed.tokenEnv = spec.TokenEnv
	}

	if spec.GitConfigMode != "" {
		if spec.GitConfigMode != GitConfigModeEnv && spec.GitConfigMode != GitConfigModeFile {
			return nil, fmt.Errorf("%w: integration %q has invalid git_config_mode %q (must be %q or %q)",
				errUtils.ErrIntegrationFailed, config.Name, spec.GitConfigMode, GitConfigModeEnv, GitConfigModeFile)
		}
		parsed.gitConfigMode = spec.GitConfigMode
	}

	return parsed, nil
}

// resolveVia validates that exactly one of via.identity / via.provider is set.
func resolveVia(via *schema.IntegrationVia) (identity, provider string, err error) {
	if via == nil || (via.Identity == "" && via.Provider == "") {
		return "", "", errUtils.ErrIntegrationViaMissing
	}
	if via.Identity != "" && via.Provider != "" {
		return "", "", errUtils.ErrIntegrationViaAmbiguous
	}
	return via.Identity, via.Provider, nil
}

// Kind returns "github/sts".
func (g *GitHubSTSIntegration) Kind() string { return integrations.KindGitHubSTS }

// tokenExpirySkew is the safety margin applied when judging whether persisted tokens are
// still fresh enough to reuse, so a token does not expire mid-operation just after the check.
const tokenExpirySkew = 60 * time.Second

// hasFreshState reports whether the persisted state holds tokens that are all still valid
// (every token present, parseable, and expiring more than tokenExpirySkew in the future).
// In file mode the on-disk git.config must also exist (Environment emits include.path to it).
// When true, Execute reuses the existing tokens instead of minting again.
func (g *GitHubSTSIntegration) hasFreshState() bool {
	defer perf.Track(nil, "github.GitHubSTSIntegration.hasFreshState")()

	state, err := g.readState()
	if err != nil || state == nil || len(state.Tokens) == 0 {
		return false
	}

	cutoff := time.Now().Add(tokenExpirySkew)
	for _, t := range state.Tokens {
		if t.Token == "" {
			return false
		}
		exp, err := time.Parse(time.RFC3339, t.ExpiresAt)
		if err != nil || !exp.After(cutoff) {
			return false
		}
	}

	if g.gitConfigMode == GitConfigModeFile {
		if _, err := os.Stat(filepath.Join(g.stateDir(), configFileName)); err != nil {
			return false
		}
	}

	return true
}

// Execute mints GitHub STS tokens and persists them for consumption and revocation.
func (g *GitHubSTSIntegration) Execute(ctx context.Context, creds types.ICredentials) error {
	defer perf.Track(nil, "github.GitHubSTSIntegration.Execute")()

	// Reuse persisted tokens when they are still fresh. This avoids a network mint on every
	// CI invocation: ambient brokers re-export GIT_CONFIG_* from the existing state instead.
	if g.hasFreshState() {
		log.Debug("github/sts: reusing fresh persisted tokens", logKeyIntegration, g.name)
		return nil
	}

	pro, err := proCredentials(creds)
	if err != nil {
		return err
	}

	resp, err := g.mint(ctx, pro)
	if err != nil {
		return err
	}

	return g.persistAndReport(resp)
}

// proCredentials validates and extracts the atmos/pro credentials that github/sts requires.
func proCredentials(creds types.ICredentials) (*types.ProCredentials, error) {
	pro, ok := creds.(*types.ProCredentials)
	if !ok || pro == nil {
		return nil, fmt.Errorf("%w: github/sts requires atmos/pro credentials, got %T", errUtils.ErrProCredentialsType, creds)
	}
	if pro.Token == "" {
		return nil, fmt.Errorf("%w: empty Atmos Pro session token", errUtils.ErrSTSMintFailed)
	}
	return pro, nil
}

// persistAndReport persists the minted tokens, writes the gitconfig file when in file mode,
// and reports the deny-by-default outcome (never logging token values).
func (g *GitHubSTSIntegration) persistAndReport(resp *stsResponse) error {
	// Surface deny-by-default exclusions verbatim (never log token values).
	for _, ex := range resp.Excluded {
		log.Warn("GitHub STS excluded a repository", logKeyIntegration, g.name, "repo", ex.Repo, "reason", ex.Reason)
	}

	if err := g.writeState(&gitSTSState{Tokens: resp.Tokens}); err != nil {
		return err
	}

	if g.gitConfigMode == GitConfigModeFile {
		if err := g.writeGitConfigFile(resp.Tokens); err != nil {
			return err
		}
	}

	// Empty tokens with everything excluded is a normal deny-by-default outcome.
	if len(resp.Tokens) == 0 {
		ui.Info(fmt.Sprintf("GitHub STS: no tokens granted (%d excluded)", len(resp.Excluded)))
		return nil
	}

	ui.Success(fmt.Sprintf("GitHub STS: minted %d token(s) for %s", len(resp.Tokens), ownersSummary(resp.Tokens)))
	return nil
}

// mint performs the POST /api/v1/sts request.
func (g *GitHubSTSIntegration) mint(ctx context.Context, pro *types.ProCredentials) (*stsResponse, error) {
	endpoint := pro.Endpoint
	if endpoint == "" {
		endpoint = "api/v1"
	}
	url := fmt.Sprintf("%s/%s/sts", strings.TrimRight(pro.BaseURL, "/"), strings.Trim(endpoint, "/"))

	body, err := json.Marshal(&stsRequest{Sources: g.repos, PolicyName: g.policyName})
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrSTSMintFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrSTSMintFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+pro.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// URL host comes from trusted Atmos Pro config (provider spec / settings.pro / ATMOS_PRO_BASE_URL), not user input.
	resp, err := stsHTTPClient.Do(req) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrSTSMintFailed, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		// Continue.
	case http.StatusBadRequest:
		return nil, errUtils.Build(errUtils.ErrNotGitHubActionsSession).
			WithHint("github/sts requires a GitHub Actions session (a ws:gh:action Atmos Pro token). Ensure the workflow has 'permissions: id-token: write' and authenticates the atmos/pro provider.").
			Err()
	case http.StatusForbidden:
		return nil, errUtils.Build(errUtils.ErrSTSNoEntitlement).
			WithHint("This workspace is not entitled to GitHub STS, or the feature is disabled. Check your Atmos Pro plan and workspace settings.").
			Err()
	default:
		return nil, fmt.Errorf("%w: STS endpoint returned status %s", errUtils.ErrSTSMintFailed, resp.Status)
	}

	// Every Atmos Pro API route returns the canonical envelope ({success, status, data}),
	// so the tokens are nested under `data` — decode through the shared Pro envelope
	// helper instead of a flat decode that would silently drop them.
	env, err := proapi.DecodeEnvelope[stsResponse](resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode STS response: %w", errUtils.ErrSTSMintFailed, err)
	}
	// The server returns HTTP 200 with success=false on application-level failures;
	// surface the message instead of silently treating it as "no tokens granted".
	if !env.Success {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrSTSMintFailed, env.EffectiveErrorMessage())
	}
	return &env.Data, nil
}

// Environment returns the GIT_CONFIG_* variables that route git over the minted tokens.
func (g *GitHubSTSIntegration) Environment() (map[string]string, error) {
	defer perf.Track(nil, "github.GitHubSTSIntegration.Environment")()

	if g.gitConfigMode == GitConfigModeFile {
		return g.environmentFileMode(), nil
	}
	return g.environmentEnvMode(), nil
}

// environmentEnvMode emits inline per-owner insteadOf rewrites via GIT_CONFIG_*.
func (g *GitHubSTSIntegration) environmentEnvMode() map[string]string {
	state, err := g.readState()
	if err != nil || state == nil || len(state.Tokens) == 0 {
		return map[string]string{}
	}

	env := map[string]string{}
	idx := 0
	for _, t := range state.Tokens {
		if t.Token == "" || t.Host == "" || t.Owner == "" {
			continue
		}
		base := fmt.Sprintf("https://%s:%s@%s/%s/", gitTokenUser, t.Token, t.Host, t.Owner)
		key := "url." + base + ".insteadOf"
		for _, replaced := range insteadOfTargets(t.Host, t.Owner) {
			env["GIT_CONFIG_KEY_"+strconv.Itoa(idx)] = key
			env["GIT_CONFIG_VALUE_"+strconv.Itoa(idx)] = replaced
			idx++
		}
	}
	if idx > 0 {
		env["GIT_CONFIG_COUNT"] = strconv.Itoa(idx)
	}

	// Optionally surface the raw token(s) as named env var(s) for consumers beyond git
	// (gh CLI, actions/checkout, REST API).
	g.addTokenEnv(env, state.Tokens)
	return env
}

// environmentFileMode emits an additive include.path pointing at the on-disk gitconfig.
func (g *GitHubSTSIntegration) environmentFileMode() map[string]string {
	configPath := filepath.Join(g.stateDir(), configFileName)
	if _, err := os.Stat(configPath); err != nil {
		return map[string]string{}
	}
	env := map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "include.path",
		"GIT_CONFIG_VALUE_0": configPath,
	}

	// File mode keeps tokens off the env by default, but an explicit token_env is an
	// opt-in override: read the persisted tokens and surface them as named env var(s).
	if g.tokenEnv != "" {
		if state, err := g.readState(); err == nil && state != nil {
			g.addTokenEnv(env, state.Tokens)
		}
	}
	return env
}

// Cleanup revokes each minted token directly against GitHub and removes state files.
func (g *GitHubSTSIntegration) Cleanup(ctx context.Context) error {
	defer perf.Track(nil, "github.GitHubSTSIntegration.Cleanup")()

	state, err := g.readState()
	if err != nil {
		// Nothing to clean up (no state file) — idempotent success.
		return nil
	}
	if state != nil {
		for _, t := range state.Tokens {
			if t.Token == "" {
				continue
			}
			if revErr := g.revokeToken(ctx, t.Token); revErr != nil {
				// Non-fatal: log and continue (token may already be expired/revoked).
				log.Warn("GitHub STS token revocation failed", logKeyIntegration, g.name, "owner", t.Owner, "error", revErr)
			} else {
				log.Debug("GitHub STS token revoked", logKeyIntegration, g.name, "owner", t.Owner)
			}
		}
	}

	g.removeState()
	return nil
}

// revokeToken revokes a single installation token via DELETE /installation/token.
func (g *GitHubSTSIntegration) revokeToken(ctx context.Context, token string) error {
	url := strings.TrimRight(githubAPIBaseURL, "/") + "/installation/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGitHubTokenRevokeFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	// URL is the constant GitHub REST API base (overridable only in tests), not user input.
	resp, err := revokeHTTPClient.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGitHubTokenRevokeFailed, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusUnauthorized, http.StatusNotFound:
		// 204 = revoked; 401/404 = already invalid/expired — treat as revoked.
		return nil
	default:
		return fmt.Errorf("%w: status %s", errUtils.ErrGitHubTokenRevokeFailed, resp.Status)
	}
}

// Environment helper: GetIdentity returns the identity name (empty for provider-bound).
func (g *GitHubSTSIntegration) GetIdentity() string { return g.identity }

// GetProvider returns the provider name (empty for identity-bound).
func (g *GitHubSTSIntegration) GetProvider() string { return g.provider }

// stateDir returns the realm-scoped state directory for this integration.
func (g *GitHubSTSIntegration) stateDir() string {
	subpath := filepath.Join(stsDataSubdir, sanitizePathSegment(g.realm), sanitizePathSegment(g.name))
	dir, err := xdg.GetXDGDataDir(subpath, dirPerms)
	if err != nil {
		log.Debug("Failed to resolve github/sts state dir", logKeyIntegration, g.name, "error", err)
		return ""
	}
	return dir
}

// writeState persists the minted tokens (0600).
func (g *GitHubSTSIntegration) writeState(state *gitSTSState) error {
	dir := g.stateDir()
	if dir == "" {
		return fmt.Errorf("%w: could not resolve state directory", errUtils.ErrGitSTSStateWrite)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGitSTSStateWrite, err)
	}
	if err := os.WriteFile(filepath.Join(dir, stateFileName), data, filePerms); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGitSTSStateWrite, err)
	}
	return nil
}

// readState reads the persisted tokens. Returns an error when no state file exists.
func (g *GitHubSTSIntegration) readState() (*gitSTSState, error) {
	dir := g.stateDir()
	if dir == "" {
		return nil, fmt.Errorf("%w: could not resolve state directory", errUtils.ErrGitSTSStateRead)
	}
	data, err := os.ReadFile(filepath.Join(dir, stateFileName))
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGitSTSStateRead, err)
	}
	var state gitSTSState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGitSTSStateRead, err)
	}
	return &state, nil
}

// removeState deletes the state and gitconfig files (best-effort).
func (g *GitHubSTSIntegration) removeState() {
	dir := g.stateDir()
	if dir == "" {
		return
	}
	_ = os.Remove(filepath.Join(dir, stateFileName))
	_ = os.Remove(filepath.Join(dir, configFileName))
}

// writeGitConfigFile writes a 0600 gitconfig with per-owner insteadOf rewrites.
func (g *GitHubSTSIntegration) writeGitConfigFile(tokens []stsToken) error {
	dir := g.stateDir()
	if dir == "" {
		return fmt.Errorf("%w: could not resolve state directory", errUtils.ErrGitSTSStateWrite)
	}

	var b strings.Builder
	for _, t := range tokens {
		if t.Token == "" || t.Host == "" || t.Owner == "" {
			continue
		}
		base := fmt.Sprintf("https://%s:%s@%s/%s/", gitTokenUser, t.Token, t.Host, t.Owner)
		fmt.Fprintf(&b, "[url %q]\n", base)
		for _, replaced := range insteadOfTargets(t.Host, t.Owner) {
			fmt.Fprintf(&b, "\tinsteadOf = %s\n", replaced)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(b.String()), filePerms); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGitSTSStateWrite, err)
	}
	return nil
}

// templateMarker signals that a token_env value is a Go template (e.g. "GH_TOKEN_{{ .owner }}")
// to be rendered per token, rather than a literal env var name.
const templateMarker = "{{"

// addTokenEnv surfaces the raw minted token(s) as named env var(s) per the configured
// token_env name. A literal name requires exactly one token (multi-owner mints log and are
// skipped — the GIT_CONFIG_* rewrites still cover them); a Go-template name (referencing
// `.owner`/`.host`, e.g. "GH_TOKEN_{{ .owner }}") is rendered per token. Token values are never logged.
func (g *GitHubSTSIntegration) addTokenEnv(env map[string]string, tokens []stsToken) {
	if g.tokenEnv == "" {
		return
	}

	valid := make([]stsToken, 0, len(tokens))
	for _, t := range tokens {
		if t.Token != "" && t.Owner != "" {
			valid = append(valid, t)
		}
	}
	if len(valid) == 0 {
		return
	}

	if strings.Contains(g.tokenEnv, templateMarker) {
		for i := range valid {
			name, err := renderTokenEnvName(g.tokenEnv, &valid[i])
			if err != nil || name == "" {
				log.Debug("github/sts: skipping token_env export; could not render template",
					logKeyIntegration, g.name, "token_env", g.tokenEnv, "error", err)
				continue
			}
			env[name] = valid[i].Token
		}
		return
	}

	if len(valid) > 1 {
		// The GIT_CONFIG_* insteadOf rewrites still cover every owner; only the raw token export is
		// skipped. When token_env is the implicit default, log at debug to avoid noise on multi-org
		// runs; when the user explicitly set a literal name, warn so they can switch to a template.
		msg := "github/sts: token_env is a literal name but multiple tokens were minted; " +
			"skipping token export (use a '{{ .owner }}' template or narrow repos/policy)"
		if g.tokenEnv == defaultTokenEnv {
			log.Debug(msg, logKeyIntegration, g.name, "token_env", g.tokenEnv, "count", len(valid))
		} else {
			log.Warn(msg, logKeyIntegration, g.name, "token_env", g.tokenEnv, "count", len(valid))
		}
		return
	}
	env[g.tokenEnv] = valid[0].Token
}

// renderTokenEnvName renders a token_env Go template for one minted token (exposing `.owner` and
// `.host`) and sanitizes the result into a valid env var name. Uses the standard library template
// engine — the same `{{ .field }}` syntax used elsewhere in Atmos — not an ad-hoc placeholder.
func renderTokenEnvName(pattern string, t *stsToken) (string, error) {
	tmpl, err := template.New("token_env").Option("missingkey=error").Parse(pattern)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrIntegrationFailed, err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, map[string]string{"owner": t.Owner, "host": t.Host}); err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrIntegrationFailed, err)
	}
	return sanitizeEnvName(b.String()), nil
}

// sanitizeEnvName makes a value safe to embed in an environment variable name:
// uppercased, with every character outside [A-Z0-9_] replaced by an underscore.
func sanitizeEnvName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// insteadOfTargets returns the URL forms each owner-scoped token should rewrite:
// https and ssh, so it covers git::https://…, shorthand, and ssh:// references.
func insteadOfTargets(host, owner string) []string {
	return []string{
		fmt.Sprintf("https://%s/%s/", host, owner),
		fmt.Sprintf("ssh://git@%s/%s/", host, owner),
	}
}

// ownersSummary returns a comma-separated list of unique owners for display (no tokens).
func ownersSummary(tokens []stsToken) string {
	seen := make(map[string]struct{}, len(tokens))
	var owners []string
	for _, t := range tokens {
		if _, ok := seen[t.Owner]; ok {
			continue
		}
		seen[t.Owner] = struct{}{}
		owners = append(owners, t.Owner)
	}
	return strings.Join(owners, ", ")
}

// sanitizePathSegment makes a config value safe to use as a single path segment.
func sanitizePathSegment(s string) string {
	if s == "" {
		return ""
	}
	r := strings.NewReplacer("/", fsReplacement, "\\", fsReplacement, ":", fsReplacement, "..", fsReplacement)
	return r.Replace(s)
}
