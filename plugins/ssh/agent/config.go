package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

// ConfigVersion is the only configuration file version this package reads and writes.
const ConfigVersion = 1

// configFileMode is the mode of the configuration file.
const configFileMode os.FileMode = 0o600

// Allow is one thing an identity's socket may be used for.
type Allow string

const (
	// AllowSigning permits Git (SSHSIG, namespace "git") signatures.
	AllowSigning Allow = "signing"
	// AllowAuthentication permits SSH login (userauth) signatures.
	AllowAuthentication Allow = "authentication"
)

// Sentinel errors. Wrap them with fmt.Errorf("%w ...") and test with errors.Is.
var (
	// ErrInvalidConfig means the agent configuration is malformed or inconsistent.
	ErrInvalidConfig = errors.New("invalid agent configuration")
	// ErrConfigNotFound means the agent configuration file does not exist.
	ErrConfigNotFound = errors.New("agent configuration not found")
)

// Identity is one entry in the agent configuration: a key, where it lives, and what it may do.
type Identity struct {
	// Name is the identity name. It also names the identity's socket.
	Name string `json:"name"`
	// Store is the key store kind that holds the key.
	Store string `json:"store"`
	// Key is the key's name within the store.
	Key string `json:"key"`
	// Allow lists what the identity's socket may sign.
	Allow []Allow `json:"allow"`
}

// Allows reports whether the identity permits a.
func (i Identity) Allows(a Allow) bool {
	for _, have := range i.Allow {
		if have == a {
			return true
		}
	}
	return false
}

// equal reports whether two identities have the same settings.
func (i Identity) equal(o Identity) bool {
	if i.Name != o.Name || i.Store != o.Store || i.Key != o.Key || len(i.Allow) != len(o.Allow) {
		return false
	}
	for n := range i.Allow {
		if i.Allow[n] != o.Allow[n] {
			return false
		}
	}
	return true
}

// Config is the agent configuration file.
type Config struct {
	// Version is the file format version.
	Version int `json:"version"`
	// Identities are the identities the agent serves, one socket each.
	Identities []Identity `json:"identities"`
}

// Validate checks the configuration and reports the first problem found.
func (c *Config) Validate() error {
	if c.Version != ConfigVersion {
		return fmt.Errorf("%w: unsupported version %d (want %d)", ErrInvalidConfig, c.Version, ConfigVersion)
	}
	seen := map[string]bool{}
	for n := range c.Identities {
		id := c.Identities[n]
		if err := id.Validate(); err != nil {
			return err
		}
		if seen[id.Name] {
			return fmt.Errorf("%w: duplicate identity %q", ErrInvalidConfig, id.Name)
		}
		seen[id.Name] = true
	}
	return nil
}

// Validate checks one identity entry.
func (i Identity) Validate() error {
	if err := paths.ValidateIdentity(i.Name); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	if i.Store == "" {
		return fmt.Errorf("%w: identity %q has no store", ErrInvalidConfig, i.Name)
	}
	if i.Key == "" {
		return fmt.Errorf("%w: identity %q has no key", ErrInvalidConfig, i.Name)
	}
	if len(i.Allow) == 0 {
		return fmt.Errorf("%w: identity %q allows nothing", ErrInvalidConfig, i.Name)
	}
	seen := map[Allow]bool{}
	for _, a := range i.Allow {
		if a != AllowSigning && a != AllowAuthentication {
			return fmt.Errorf("%w: identity %q has unknown allow value %q", ErrInvalidConfig, i.Name, a)
		}
		if seen[a] {
			return fmt.Errorf("%w: identity %q lists %q twice", ErrInvalidConfig, i.Name, a)
		}
		seen[a] = true
	}
	return nil
}

// find returns the index of the identity called name, or -1.
func (c *Config) find(name string) int {
	for n := range c.Identities {
		if c.Identities[n].Name == name {
			return n
		}
	}
	return -1
}

// LoadConfig reads and validates the configuration file. A missing file is ErrConfigNotFound.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, path)
		}
		return nil, fmt.Errorf("read agent configuration: %w", err)
	}
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalidConfig, path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

// loadOrEmpty is LoadConfig, but a missing file yields an empty configuration.
func loadOrEmpty(path string) (*Config, error) {
	cfg, err := LoadConfig(path)
	if errors.Is(err, ErrConfigNotFound) {
		return &Config{Version: ConfigVersion}, nil
	}
	return cfg, err
}

// SaveConfig validates cfg and writes it atomically with mode 0600. Identities are written sorted
// by name so the file is stable.
func SaveConfig(path string, cfg *Config) error {
	out := &Config{Version: cfg.Version, Identities: append([]Identity(nil), cfg.Identities...)}
	if err := out.Validate(); err != nil {
		return err
	}
	sort.Slice(out.Identities, func(a, b int) bool { return out.Identities[a].Name < out.Identities[b].Name })
	if out.Identities == nil {
		out.Identities = []Identity{}
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encode agent configuration: %w", err)
	}
	return writeFileAtomic(path, append(data, '\n'), configFileMode)
}

// UpsertIdentity adds id to the configuration file, or replaces the entry with the same name. The
// file is created if it does not exist.
func UpsertIdentity(path string, id Identity) error {
	if err := id.Validate(); err != nil {
		return err
	}
	cfg, err := loadOrEmpty(path)
	if err != nil {
		return err
	}
	if n := cfg.find(id.Name); n >= 0 {
		cfg.Identities[n] = id
	} else {
		cfg.Identities = append(cfg.Identities, id)
	}
	return SaveConfig(path, cfg)
}

// RemoveIdentity deletes the entry called name. It reports whether an entry was removed; removing
// a name that is not there, or from a file that does not exist, is not an error.
func RemoveIdentity(path, name string) (bool, error) {
	cfg, err := loadOrEmpty(path)
	if err != nil {
		return false, err
	}
	n := cfg.find(name)
	if n < 0 {
		return false, nil
	}
	cfg.Identities = append(cfg.Identities[:n], cfg.Identities[n+1:]...)
	return true, SaveConfig(path, cfg)
}
