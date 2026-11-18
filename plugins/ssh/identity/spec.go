package identity

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

// ErrInvalidSpec means an identity's spec is malformed.
var ErrInvalidSpec = errors.New("invalid ssh/key identity spec")

// AuthoritySelf is the authority used when the spec names none.
const AuthoritySelf = "self"

var (
	storeRE    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	userRE     = regexp.MustCompile(`^[^\s\x00-\x1f\x7f]{1,64}$`)
	hostRE     = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?(:[0-9]{1,5})?$`)
	knownSpecs = []string{"authority", "store", "allow", "require", "user", "key", "rewrite_https", "root"}
)

// Spec is the parsed spec of an ssh/key identity.
type Spec struct {
	// Authority names who vouches for the key.
	Authority string
	// Store names the key store.
	Store string
	// Allow lists what the key may do, in canonical order and without duplicates.
	Allow []agent.Allow
	// Require lists the requirement terms, sorted and without duplicates.
	Require []keystore.Requirement
	// User is the SSH login name on the remote side.
	User string
	// Key is the key's name in the store. It is empty when it defaults to the identity's name.
	Key string
	// RewriteHTTPS lists hosts whose HTTPS Git URLs are rewritten to SSH.
	RewriteHTTPS []string
	// Root overrides the plugin's data root.
	Root string
}

// KeyName returns the key's name for identity.
func (s Spec) KeyName(identity string) string {
	if s.Key != "" {
		return s.Key
	}
	return identity
}

// Uses reports whether the spec allows a.
func (s Spec) Uses(a agent.Allow) bool { return slices.Contains(s.Allow, a) }

// ParseSpec reads and validates an identity's spec. It rejects unknown fields.
func ParseSpec(raw map[string]any) (Spec, error) {
	for name := range raw {
		if !slices.Contains(knownSpecs, name) {
			return Spec{}, specError("unknown field %q (known: %s)", name, strings.Join(knownSpecs, ", "))
		}
	}
	var s Spec
	var err error
	if s.Authority, err = optionalString(raw, "authority"); err != nil {
		return Spec{}, err
	}
	if s.Authority == "" {
		s.Authority = AuthoritySelf
	}
	if err := s.parseStore(raw); err != nil {
		return Spec{}, err
	}
	if err := s.parseAllowAndRequire(raw); err != nil {
		return Spec{}, err
	}
	if err := s.parseOptional(raw); err != nil {
		return Spec{}, err
	}
	return s, nil
}

func specError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSpec, fmt.Sprintf(format, args...))
}

func optionalString(raw map[string]any, field string) (string, error) {
	v, ok := raw[field]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", specError("%s must be a non-empty string", field)
	}
	return s, nil
}

func (s *Spec) parseStore(raw map[string]any) error {
	store, err := optionalString(raw, "store")
	if err != nil {
		return err
	}
	if store == "" {
		return specError("store is required")
	}
	if !storeRE.MatchString(store) {
		return specError("store %q is not a key store name", store)
	}
	s.Store = store
	return nil
}

func (s *Spec) parseAllowAndRequire(raw map[string]any) error {
	allow, err := stringList(raw, "allow")
	if err != nil {
		return err
	}
	if len(allow) == 0 {
		return specError("allow is required and must list signing, authentication, or both")
	}
	for _, a := range []agent.Allow{agent.AllowSigning, agent.AllowAuthentication} {
		if slices.Contains(allow, string(a)) {
			s.Allow = append(s.Allow, a)
		}
	}
	for _, a := range allow {
		if a != string(agent.AllowSigning) && a != string(agent.AllowAuthentication) {
			return specError("allow contains unknown use %q (known: signing, authentication)", a)
		}
	}
	terms, err := stringList(raw, "require")
	if err != nil {
		return err
	}
	for _, t := range terms {
		if t != string(keystore.Unlocked) {
			return specError("require contains unknown term %q (known: %s)", t, keystore.Unlocked)
		}
		s.Require = append(s.Require, keystore.Requirement(t))
	}
	if len(s.Require) > 0 {
		s.Require = keystore.Normalize(s.Require)
	}
	return nil
}

func (s *Spec) parseOptional(raw map[string]any) error {
	var err error
	if s.User, err = optionalString(raw, "user"); err != nil {
		return err
	}
	if s.User != "" && !userRE.MatchString(s.User) {
		return specError("user %q is not a valid login name", s.User)
	}
	if s.Key, err = optionalString(raw, "key"); err != nil {
		return err
	}
	if s.Key != "" {
		if err := paths.ValidateIdentity(s.Key); err != nil {
			return specError("key: %v", err)
		}
	}
	if s.Root, err = optionalString(raw, "root"); err != nil {
		return err
	}
	if s.Root != "" && !filepath.IsAbs(s.Root) {
		return specError("root must be an absolute path")
	}
	hosts, err := stringList(raw, "rewrite_https")
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if !hostRE.MatchString(h) {
			return specError("rewrite_https host %q is not a host name", h)
		}
	}
	s.RewriteHTTPS = uniqueSorted(hosts)
	return nil
}

// stringList reads a field that is a string or a list of strings. An absent field is empty.
func stringList(raw map[string]any, field string) ([]string, error) {
	v, ok := raw[field]
	if !ok || v == nil {
		return nil, nil
	}
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil, specError("%s must not be empty", field)
		}
		return []string{t}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok || s == "" {
				return nil, specError("%s must contain only non-empty strings", field)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, specError("%s must be a string or a list of strings", field)
	}
}

func uniqueSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := slices.Clone(in)
	sort.Strings(out)
	return slices.Compact(out)
}
