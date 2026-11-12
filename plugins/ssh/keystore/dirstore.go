package keystore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	dirMode  os.FileMode = 0o700
	fileMode os.FileMode = 0o600

	keyExt  = ".key"
	metaExt = ".json"

	metaVersion = 1
	maxNameLen  = 128
)

var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// record is the sidecar metadata persisted beside each key. A Secure Enclave blob does not reveal
// which access class it was created with, so the requirements are recorded here.
type record struct {
	Version      int           `json:"version"`
	Name         string        `json:"name"`
	Store        string        `json:"store"`
	Requirements []Requirement `json:"requirements"`
	Created      time.Time     `json:"created"`
	PublicKey    string        `json:"public_key"`
}

// dirStore holds the on-disk layout shared by the stores: <name>.key (private material, mode 0600)
// and <name>.json (sidecar metadata, mode 0600) inside a directory with mode 0700.
type dirStore struct {
	kind string
	dir  string
}

func validName(name string) error {
	if len(name) > maxNameLen || !nameRE.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return nil
}

func (d dirStore) keyPath(name string) string  { return filepath.Join(d.dir, name+keyExt) }
func (d dirStore) metaPath(name string) string { return filepath.Join(d.dir, name+metaExt) }

func (d dirStore) ensureDir() error {
	if err := os.MkdirAll(d.dir, dirMode); err != nil {
		return fmt.Errorf("create key directory: %w", err)
	}
	if err := os.Chmod(d.dir, dirMode); err != nil {
		return fmt.Errorf("restrict key directory: %w", err)
	}
	return nil
}

// writeNew stores material and its metadata, failing with ErrKeyExists if the name is taken.
func (d dirStore) writeNew(rec record, material []byte) error {
	if err := d.ensureDir(); err != nil {
		return err
	}
	meta, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("encode key metadata: %w", err)
	}
	if err := writeExclusive(d.keyPath(rec.Name), material); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %q", ErrKeyExists, rec.Name)
		}
		return fmt.Errorf("write key: %w", err)
	}
	if err := writeExclusive(d.metaPath(rec.Name), meta); err != nil {
		_ = os.Remove(d.keyPath(rec.Name))
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %q", ErrKeyExists, rec.Name)
		}
		return fmt.Errorf("write key metadata: %w", err)
	}
	return nil
}

func writeExclusive(file string, data []byte) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(file)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(file)
		return err
	}
	return nil
}

// read returns a key's private material and metadata.
func (d dirStore) read(name string) ([]byte, record, error) {
	var rec record
	if err := validName(name); err != nil {
		return nil, rec, err
	}
	material, err := os.ReadFile(d.keyPath(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, rec, fmt.Errorf("%w: %q", ErrKeyNotFound, name)
	}
	if err != nil {
		return nil, rec, fmt.Errorf("read key: %w", err)
	}
	rec, err = d.readMeta(name)
	if err != nil {
		return nil, rec, err
	}
	return material, rec, nil
}

func (d dirStore) readMeta(name string) (record, error) {
	var rec record
	data, err := os.ReadFile(d.metaPath(name))
	if err != nil {
		return rec, fmt.Errorf("%w: %q has no readable metadata: %w", ErrInvalidKey, name, err)
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, fmt.Errorf("%w: %q metadata: %w", ErrInvalidKey, name, err)
	}
	if rec.Version != metaVersion || rec.Name != name || rec.Store != d.kind {
		return rec, fmt.Errorf("%w: %q metadata does not describe a %s key named %q", ErrInvalidKey, name, d.kind, name)
	}
	return rec, nil
}

func (d dirStore) remove(name string) error {
	if err := validName(name); err != nil {
		return err
	}
	err := os.Remove(d.keyPath(name))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %q", ErrKeyNotFound, name)
	}
	if err != nil {
		return fmt.Errorf("delete key: %w", err)
	}
	if err := os.Remove(d.metaPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete key metadata: %w", err)
	}
	return nil
}

func (d dirStore) list(ctx context.Context) ([]Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(d.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}
	var infos []Info
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), metaExt)
		if !ok || e.IsDir() || validName(name) != nil {
			continue
		}
		rec, err := d.readMeta(name)
		if err != nil {
			return nil, err
		}
		info, err := rec.info()
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos, nil
}

func newRecord(kind, name string, requirements []Requirement, pub ssh.PublicKey) record {
	return record{
		Version:      metaVersion,
		Name:         name,
		Store:        kind,
		Requirements: requirements,
		Created:      time.Now().UTC(),
		PublicKey:    strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))),
	}
}

func (r record) info() (Info, error) {
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(r.PublicKey))
	if err != nil {
		return Info{}, fmt.Errorf("%w: %q public key: %w", ErrInvalidKey, r.Name, err)
	}
	return Info{
		Name:         r.Name,
		Store:        r.Store,
		Requirements: Normalize(r.Requirements),
		Created:      r.Created,
		PublicKey:    pub,
	}, nil
}
