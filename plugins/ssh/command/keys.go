package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/inventory"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
)

// Settings page for adding an SSH key on GitHub.
const githubKeysURL = "https://github.com/settings/ssh/new"

// ErrNoSuchKey means no key has the name asked for.
var ErrNoSuchKey = errors.New("no such key")

// ErrAmbiguousKey means a name exists in more than one key store.
var ErrAmbiguousKey = errors.New("key name exists in more than one store")

// Output formats of "key show".
const (
	formatAuthorizedKey  = "authorized-key"
	formatAllowedSigners = "allowed-signers"
	formatGitHub         = "github"
	formatJSON           = "json"
)

// keyList prints the keys of every store.
func (c *call) keyList(words []string) error {
	if err := c.flags.only(); err != nil {
		return err
	}
	if err := exactly(words, 0, "key list"); err != nil {
		return err
	}
	keys, err := inventory.List(c.ctx, c.svc.deps.OpenStore, c.paths)
	if err != nil {
		fmt.Fprintf(c.err, "Warning: %v\n", err)
	}
	if len(keys) == 0 {
		c.printf("No keys on this machine.\n")
		return nil
	}
	w := tabwriter.NewWriter(c.out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "KEY\tSTORE\tREQUIRE\tFINGERPRINT\tCREATED")
	for _, k := range keys {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", k.Name, k.Store, requireText(k.Requirements), authority.Fingerprint(k.PublicKey), dateText(k.Created))
	}
	return w.Flush()
}

func requireText(terms []keystore.Requirement) string {
	if len(terms) == 0 {
		return "-"
	}
	out := make([]string, len(terms))
	for i, t := range terms {
		out[i] = string(t)
	}
	return strings.Join(out, ",")
}

func dateText(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02")
}

// find returns the key called name, in the store given by --store when there are several.
func (c *call) find(name string) (keystore.Info, error) {
	keys, err := inventory.List(c.ctx, c.svc.deps.OpenStore, c.paths)
	store := c.flags.values["store"]
	var matches []keystore.Info
	for _, k := range keys {
		if k.Name == name && (store == "" || k.Store == store) {
			matches = append(matches, k)
		}
	}
	switch {
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return keystore.Info{}, fmt.Errorf("%w: %q; choose one with --store", ErrAmbiguousKey, name)
	case err != nil:
		return keystore.Info{}, fmt.Errorf("%w: %q (some stores could not be listed: %v)", ErrNoSuchKey, name, err)
	default:
		return keystore.Info{}, fmt.Errorf("%w: %q", ErrNoSuchKey, name)
	}
}

// keyShow prints one key in the requested format.
func (c *call) keyShow(words []string) error {
	if err := c.flags.only("format", "store"); err != nil {
		return err
	}
	if err := exactly(words, 1, "key show <name> [--format authorized-key|allowed-signers|github|json]"); err != nil {
		return err
	}
	k, err := c.find(words[0])
	if err != nil {
		return err
	}
	format := c.flags.values["format"]
	if format == "" {
		format = formatAuthorizedKey
	}
	line := authority.AuthorizedKey(k.PublicKey)
	switch format {
	case formatAuthorizedKey:
		c.printf("%s %s\n", line, k.Name)
	case formatAllowedSigners:
		c.printf("* namespaces=\"git\" %s\n", line)
	case formatGitHub:
		c.showGitHub(k, line)
	case formatJSON:
		return c.showJSON(k, line)
	default:
		return usagef("unknown format %q (known: %s, %s, %s, %s)", format, formatAuthorizedKey, formatAllowedSigners, formatGitHub, formatJSON)
	}
	return nil
}

func (c *call) showGitHub(k keystore.Info, line string) {
	host, err := c.svc.deps.Hostname()
	if err != nil || host == "" {
		host = "this-machine"
	}
	title := fmt.Sprintf("%s@%s (Atmos)", k.Name, host)
	c.printf("Public key:\n  %s\n\n", line)
	c.printf("Add it on GitHub (choose \"Signing Key\" as the key type):\n  %s\n\n", githubKeysURL)
	c.printf("Or with the GitHub CLI:\n  echo '%s' | gh ssh-key add --type signing --title '%s'\n", line, title)
	c.printf("  echo '%s' | gh ssh-key add --type authentication --title '%s'   # to log in over SSH\n", line, title)
}

func (c *call) showJSON(k keystore.Info, line string) error {
	reqs := make([]string, len(k.Requirements))
	for i, r := range k.Requirements {
		reqs[i] = string(r)
	}
	data, err := json.MarshalIndent(map[string]any{
		"name": k.Name, "store": k.Store, "require": reqs,
		"fingerprint": authority.Fingerprint(k.PublicKey), "created": k.Created.UTC().Format(time.RFC3339),
		"public_key": line,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode key: %w", err)
	}
	c.printf("%s\n", data)
	return nil
}

// keyDelete deletes a local key and its agent entries. It leaves the receipt, so the next login or
// logout withdraws the enrollment.
func (c *call) keyDelete(words []string) error {
	if err := c.flags.only("yes", "store"); err != nil {
		return err
	}
	if err := exactly(words, 1, "key delete <name> --yes"); err != nil {
		return err
	}
	if !c.flags.bools["yes"] {
		return fmt.Errorf("%w: deleting key %q cannot be undone and commands cannot ask; run again with --yes", ErrRefused, words[0])
	}
	k, err := c.find(words[0])
	if err != nil {
		return err
	}
	store, err := c.svc.deps.OpenStore(k.Store, c.paths.StoreDir(k.Store))
	if err != nil {
		return err
	}
	if err := store.Delete(c.ctx, k.Name); err != nil {
		return fmt.Errorf("delete key %q: %w", k.Name, err)
	}
	removed, err := c.removeAgentEntries(k)
	if err != nil {
		return err
	}
	c.printf("Deleted key %q from the %s store.\n", k.Name, k.Store)
	if len(removed) > 0 {
		c.printf("Removed from the agent: %s.\n", strings.Join(removed, ", "))
	}
	c.printf("Its enrollment, if any, is withdrawn at the next \"atmos auth login\" or \"atmos auth logout\".\n")
	return nil
}

// removeAgentEntries takes every identity that used the key out of the agent configuration.
func (c *call) removeAgentEntries(k keystore.Info) ([]string, error) {
	cfg, err := agent.LoadConfig(c.paths.AgentConfigFile())
	if errors.Is(err, agent.ErrConfigNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, id := range cfg.Identities {
		if id.Store != k.Store || id.Key != k.Name {
			continue
		}
		if _, err := agent.RemoveIdentity(c.paths.AgentConfigFile(), id.Name); err != nil {
			return removed, err
		}
		removed = append(removed, id.Name)
	}
	return removed, nil
}
