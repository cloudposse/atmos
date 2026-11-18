package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/inventory"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
	"github.com/cloudposse/atmos/plugins/ssh/statusfile"
)

// Logout withdraws the enrollments whose key no longer exists on this machine, using the token the
// identity saved when it last authenticated, and takes the identity out of the agent so its socket
// goes away. A key that still exists stays enrolled: logging out never strips a working machine.
// The host clears the identity's private state afterwards.
func (s *Service) Logout(ctx context.Context, name string, raw map[string]any) error {
	r := &run{svc: s, name: name}
	r.status = statusfile.Status{Identity: name, Operation: statusfile.OperationLogout}
	err := r.logout(ctx, raw)
	return r.finish(err)
}

func (r *run) logout(ctx context.Context, raw map[string]any) error {
	if err := r.resolveLogout(raw); err != nil {
		return err
	}
	token := r.savedToken(ctx)
	if token != "" {
		r.upstream, r.secrets = map[string]string{authority.CredentialToken: token}, []string{token}
	}
	pruneErr := r.prune(ctx)
	var errs []error
	if pruneErr != nil {
		errs = append(errs, pruneErr)
	}
	if _, err := agent.RemoveIdentity(r.paths.AgentConfigFile(), r.name); err != nil {
		errs = append(errs, err)
	}
	if _, err := writeGitConfig(r.paths, r.name, nil); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// resolveLogout is resolve without the parts that create things. An invalid spec still allows the
// agent entry to be removed, so a bad edit cannot trap a stale socket.
func (r *run) resolveLogout(raw map[string]any) error {
	if err := paths.ValidateIdentity(r.name); err != nil {
		return err
	}
	spec, specErr := ParseSpec(raw)
	root := spec.Root
	if specErr != nil {
		root, _ = raw["root"].(string)
	}
	p, err := r.svc.deps.Paths(root)
	if err != nil {
		return err
	}
	r.paths = p
	if specErr != nil {
		// Without a valid spec the authority is unknown, so fall back to removing the agent entry.
		r.spec = Spec{Authority: ""}
		r.status.Warnings = append(r.status.Warnings, "enrollments were not checked: "+specErr.Error())
		return nil
	}
	r.spec, r.keyName = spec, spec.KeyName(r.name)
	r.status.Key, r.status.Store, r.status.Authority = r.keyName, spec.Store, spec.Authority
	return nil
}

func (r *run) savedToken(ctx context.Context) string {
	data, found, err := r.svc.deps.State.Get(ctx, StateTokenKey)
	if err != nil {
		r.status.Warnings = append(r.status.Warnings, "the saved token could not be read: "+err.Error())
		return ""
	}
	if !found {
		return ""
	}
	return string(data)
}

// prune withdraws the enrollments of keys that are gone.
func (r *run) prune(ctx context.Context) error {
	if r.spec.Authority == "" {
		r.status.Outcome = "skipped"
		return nil
	}
	d := r.svc.deps
	keys, listErr := inventory.List(ctx, d.OpenStore, r.paths)
	existing := inventory.Fingerprints(keys, listErr)
	if existing == nil {
		r.status.Outcome = "skipped"
		r.status.Warnings = append(r.status.Warnings, "enrollments were not checked because the key stores could not all be listed: "+listErr.Error())
		return nil
	}
	auth, err := d.Authorities.New(r.spec.Authority, authority.Options{HTTPClient: d.HTTPClient})
	if err != nil {
		return err
	}
	receipts, err := authority.NewStore(r.paths.ReceiptsDir())
	if err != nil {
		return err
	}
	res, err := authority.Prune(ctx, authority.Params{
		Authority: auth, Credentials: r.upstream, Store: receipts, Existing: existing, Now: d.Now,
	})
	r.record(res)
	if err != nil {
		return fmt.Errorf("withdraw enrollments of missing keys: %w", err)
	}
	return nil
}
