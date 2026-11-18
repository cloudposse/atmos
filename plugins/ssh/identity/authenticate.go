package identity

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/daemon"
	"github.com/cloudposse/atmos/plugins/ssh/inventory"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
	"github.com/cloudposse/atmos/plugins/ssh/statusfile"
)

// fallbackHostname titles keys when the machine's name cannot be read.
const fallbackHostname = "unknown-host"

// run is one Authenticate call.
type run struct {
	svc      *Service
	name     string
	upstream map[string]string
	secrets  []string
	spec     Spec
	paths    *paths.Paths
	keyName  string
	pub      ssh.PublicKey
	status   statusfile.Status
}

// Authenticate brings the identity up: the key exists, the authority trusts it, the agent serves
// it, and the environment points tools at it. It writes the identity's status file whatever the
// outcome. The upstream map holds the provider's credentials; the token is the entry "token".
func (s *Service) Authenticate(ctx context.Context, name string, raw map[string]any, upstream map[string]string) (Result, error) {
	r := &run{svc: s, name: name, upstream: upstream, secrets: secretValues(upstream)}
	r.status = statusfile.Status{Identity: name, Operation: statusfile.OperationAuthenticate}
	res, err := r.authenticate(ctx, raw)
	return res, r.finish(err)
}

func secretValues(upstream map[string]string) []string {
	var out []string
	for _, v := range upstream {
		out = append(out, v)
	}
	return out
}

func (r *run) authenticate(ctx context.Context, raw map[string]any) (Result, error) {
	if err := r.resolve(raw); err != nil {
		return Result{}, err
	}
	if err := r.ensureKey(ctx); err != nil {
		return Result{}, err
	}
	if err := r.enroll(ctx); err != nil {
		return Result{}, err
	}
	r.rememberToken(ctx)
	if err := r.serve(ctx); err != nil {
		return Result{}, err
	}
	gitConfig, err := r.gitConfig()
	if err != nil {
		return Result{}, err
	}
	return r.result(gitConfig), nil
}

// resolve parses the spec and locates the data root. When the spec is invalid it still locates
// the root if it can, so the failure reaches the status file.
func (r *run) resolve(raw map[string]any) error {
	spec, specErr := ParseSpec(raw)
	root := spec.Root
	if specErr != nil {
		root, _ = raw["root"].(string)
		if !filepath.IsAbs(root) {
			root = ""
		}
	}
	if p, err := r.svc.deps.Paths(root); err == nil {
		r.paths = p
	} else if specErr == nil {
		return err
	}
	if err := paths.ValidateIdentity(r.name); err != nil {
		r.paths = nil
		return err
	}
	if specErr != nil {
		return specErr
	}
	r.spec, r.keyName = spec, spec.KeyName(r.name)
	r.status.Key, r.status.Store, r.status.Authority = r.keyName, spec.Store, spec.Authority
	r.status.Allow, r.status.Require = allowStrings(spec.Allow), requireStrings(spec.Require)
	return r.paths.EnsureDirs()
}

func allowStrings(in []agent.Allow) []string {
	out := make([]string, len(in))
	for i, a := range in {
		out[i] = string(a)
	}
	return out
}

func requireStrings(in []keystore.Requirement) []string {
	out := make([]string, len(in))
	for i, a := range in {
		out[i] = string(a)
	}
	return out
}

// ensureKey loads the key or creates it. A key that exists with other requirements is an error:
// the requirement is fixed at creation, so the fix is to rotate the key.
func (r *run) ensureKey(ctx context.Context) error {
	store, err := r.svc.deps.OpenStore(r.spec.Store, r.paths.StoreDir(r.spec.Store))
	if err != nil {
		return err
	}
	want, err := keystore.Validate(store, r.spec.Require)
	if err != nil {
		return err
	}
	key, err := store.Load(ctx, r.keyName)
	switch {
	case errors.Is(err, keystore.ErrKeyNotFound):
		key, err = store.Create(ctx, r.keyName, want)
		if err != nil {
			return fmt.Errorf("create key %q: %w", r.keyName, err)
		}
	case err != nil:
		return fmt.Errorf("load key %q: %w", r.keyName, err)
	default:
		if err := keystore.CheckRequirements(key.Requirements(), want); err != nil {
			_ = key.Close()
			return fmt.Errorf("key %q: %w", r.keyName, err)
		}
	}
	r.pub = key.SSHPublicKey()
	return key.Close()
}

// enroll has the authority vouch for the key. With a matching receipt on disk it makes no call.
func (r *run) enroll(ctx context.Context) error {
	d := r.svc.deps
	auth, err := d.Authorities.New(r.spec.Authority, authority.Options{HTTPClient: d.HTTPClient})
	if err != nil {
		return err
	}
	receipts, err := authority.NewStore(r.paths.ReceiptsDir())
	if err != nil {
		return err
	}
	keys, listErr := inventory.List(ctx, d.OpenStore, r.paths)
	existing := inventory.Fingerprints(keys, listErr)
	if listErr != nil {
		r.status.Warnings = append(r.status.Warnings, "stale enrollments were not cleaned up: "+listErr.Error())
	}
	res, err := authority.Reconcile(ctx, authority.Params{
		Authority: auth, Credentials: r.upstream, Store: receipts,
		KeyName: r.keyName, Key: r.pub, Uses: uses(r.spec.Allow), Title: r.title(),
		Existing: existing, Now: d.Now,
	})
	r.record(res)
	if res.Outcome == authority.OutcomeFailed {
		if err == nil {
			err = fmt.Errorf("%w: %s", authority.ErrInvalidRequest, "enrollment failed")
		}
		return err
	}
	if err != nil {
		r.status.Warnings = append(r.status.Warnings, err.Error())
	}
	return nil
}

func uses(allow []agent.Allow) []authority.Use {
	out := make([]authority.Use, len(allow))
	for i, a := range allow {
		out[i] = authority.Use(a)
	}
	return out
}

func (r *run) title() string {
	host, err := r.svc.deps.Hostname()
	if err != nil || host == "" {
		host = fallbackHostname
	}
	return fmt.Sprintf("%s@%s (Atmos)", r.keyName, host)
}

// record copies what Reconcile did into the status.
func (r *run) record(res authority.Result) {
	r.status.Outcome = string(res.Outcome)
	r.status.Subject = res.Receipt.Subject
	for _, u := range res.Added {
		r.status.Added = append(r.status.Added, string(u))
	}
	for _, u := range res.Removed {
		r.status.Removed = append(r.status.Removed, string(u))
	}
	for _, rec := range res.Revoked {
		r.status.Revoked = append(r.status.Revoked, rec.KeyName)
	}
}

// rememberToken keeps the upstream token in the identity's private state, so logout can revoke
// enrollments later. The provider's own state is not visible to the identity.
func (r *run) rememberToken(ctx context.Context) {
	token := r.upstream[authority.CredentialToken]
	if token == "" {
		return
	}
	have, found, err := r.svc.deps.State.Get(ctx, StateTokenKey)
	if err == nil && found && string(have) == token {
		return
	}
	if err := r.svc.deps.State.Put(ctx, StateTokenKey, []byte(token)); err != nil {
		r.status.Warnings = append(r.status.Warnings, "the token could not be saved, so logout cannot revoke enrollments: "+err.Error())
	}
}

// serve registers the identity with the agent and makes sure the daemon serves its socket.
func (r *run) serve(ctx context.Context) error {
	err := agent.UpsertIdentity(r.paths.AgentConfigFile(), agent.Identity{
		Name: r.name, Store: r.spec.Store, Key: r.keyName, Allow: r.spec.Allow,
	})
	if err != nil {
		return err
	}
	if _, err := daemon.Ensure(ctx, r.svc.deps.Daemon, r.paths); err != nil {
		return err
	}
	socket, err := r.paths.SocketPath(r.name)
	if err != nil {
		return err
	}
	return r.waitForSocket(ctx, socket)
}

func (r *run) waitForSocket(ctx context.Context, socket string) error {
	var last error
	for range socketAttempts {
		if last = r.svc.deps.CheckSocket(ctx, socket, r.pub); last == nil {
			return nil
		}
		if err := r.svc.deps.Sleep(ctx, socketInterval); err != nil {
			return err
		}
	}
	return fmt.Errorf("%w: %v", ErrAgentUnavailable, last)
}

// gitConfig writes the identity's Git configuration, or removes a stale one. It returns the file's
// path, or "" when none is needed.
func (r *run) gitConfig() (string, error) {
	signing := r.spec.Uses(agent.AllowSigning)
	if !signing && len(r.spec.RewriteHTTPS) == 0 {
		return writeGitConfig(r.paths, r.name, nil)
	}
	home, err := r.svc.deps.HomeDir()
	if err != nil {
		return "", err
	}
	g := GitConfig{
		Includes:     []string{filepath.Join(home, ".gitconfig"), filepath.Join(home, ".config", "git", "config")},
		Signing:      signing,
		RewriteHTTPS: r.spec.RewriteHTTPS,
	}
	if signing {
		if g.SSHKeygen, err = r.svc.deps.FindSSHKeygen(); err != nil {
			return "", err
		}
		g.SigningKey = authority.AuthorizedKey(r.pub)
	}
	return writeGitConfig(r.paths, r.name, &g)
}

// result builds the environment credential.
func (r *run) result(gitConfig string) Result {
	socket, _ := r.paths.SocketPath(r.name)
	env := map[string]string{EnvAuthSock: socket}
	if gitConfig != "" {
		env[EnvGitConfig] = gitConfig
	}
	if r.spec.User != "" && r.spec.Uses(agent.AllowAuthentication) {
		env[EnvAnsibleUsr] = r.spec.User
	}
	return Result{Environment: env, ExpiresAt: r.svc.deps.Now().Add(CredentialLifetime)}
}

// finish records the outcome in the status file and returns the failure, if any.
func (r *run) finish(err error) error {
	r.status.Time = r.svc.deps.Now().UTC()
	var failure *Error
	if err != nil {
		failure = classify(r.name, r.keyName, err, r.secrets...)
		r.status.Success, r.status.Outcome = false, "failed"
		r.status.Code, r.status.Error = string(failure.Code), failure.Message
	} else {
		r.status.Success = true
	}
	for i, w := range r.status.Warnings {
		r.status.Warnings[i] = scrub(w, r.secrets)
	}
	if r.paths != nil {
		// The status file is a diagnostic aid; failing to write it must not fail the login.
		_ = statusfile.Write(r.paths, r.status)
	}
	if failure == nil {
		return nil
	}
	return failure
}
