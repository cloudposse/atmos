// Package statusfile records the outcome of each identity's last login or logout.
//
// Atmos discards the text of plugin errors during authentication, so this file is how a user finds
// out why a login failed: "ssh key status" reads it. A status holds no credential.
package statusfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

const (
	dirName  = "status"
	fileExt  = ".json"
	fileMode = os.FileMode(0o600)
)

// Operation names what was running when the status was written.
const (
	OperationAuthenticate = "authenticate"
	OperationLogout       = "logout"
)

// ErrNotFound means an identity has no status file.
var ErrNotFound = errors.New("no status recorded")

// Status is the result of an identity's last login or logout.
type Status struct {
	// Identity is the Atmos identity's name.
	Identity string `json:"identity"`
	// Operation is OperationAuthenticate or OperationLogout.
	Operation string `json:"operation"`
	// Time is when the operation finished.
	Time time.Time `json:"time"`
	// Success is false when the operation failed.
	Success bool `json:"success"`
	// Outcome is the reconcile outcome ("unchanged", "enrolled", ...) or "failed".
	Outcome string `json:"outcome"`
	// Key and Store name the identity's key.
	Key   string `json:"key,omitempty"`
	Store string `json:"store,omitempty"`
	// Require lists the requirement terms the identity asked for.
	Require []string `json:"require,omitempty"`
	// Authority and Subject say who vouches for the key.
	Authority string `json:"authority,omitempty"`
	Subject   string `json:"subject,omitempty"`
	// Allow lists the uses the identity asked for.
	Allow []string `json:"allow,omitempty"`
	// Added, Removed and Revoked say what changed at the authority.
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
	Revoked []string `json:"revoked,omitempty"`
	// Code is a short, stable name for the failure, and Error says what to do about it.
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
	// Warnings are problems that did not stop the operation.
	Warnings []string `json:"warnings,omitempty"`
}

// Dir is the directory that holds the status files.
func Dir(p *paths.Paths) string { return filepath.Join(p.Root(), dirName) }

// Path returns the status file of identity.
func Path(p *paths.Paths, identity string) (string, error) {
	if err := paths.ValidateIdentity(identity); err != nil {
		return "", err
	}
	return filepath.Join(Dir(p), identity+fileExt), nil
}

// Write stores s atomically with mode 0600, replacing the identity's previous status.
func Write(p *paths.Paths, s Status) error {
	path, err := Path(p, s.Identity)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode status: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), paths.DirMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".status-*")
	if err != nil {
		return fmt.Errorf("create temporary status file: %w", err)
	}
	name := tmp.Name()
	if err := writeAndClose(tmp, append(data, '\n')); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func writeAndClose(f *os.File, data []byte) error {
	// Chmod is a no-op error on Windows; the directory is private anyway.
	_ = f.Chmod(fileMode)
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", f.Name(), err)
	}
	return nil
}

// Read returns the identity's status, or ErrNotFound.
func Read(p *paths.Paths, identity string) (Status, error) {
	path, err := Path(p, identity)
	if err != nil {
		return Status{}, err
	}
	return readFile(path)
}

func readFile(path string) (Status, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, fmt.Errorf("%w: %s", ErrNotFound, path)
	}
	if err != nil {
		return Status{}, fmt.Errorf("read status: %w", err)
	}
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return Status{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return s, nil
}

// List returns every readable status, ordered by identity. Unreadable files are skipped and
// reported together in the returned error, so one bad file hides nothing else.
func List(p *paths.Paths) ([]Status, error) {
	entries, err := os.ReadDir(Dir(p))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list status files: %w", err)
	}
	var out []Status
	var errs []error
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), fileExt) || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		s, err := readFile(filepath.Join(Dir(p), e.Name()))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Identity < out[j].Identity })
	return out, errors.Join(errs...)
}

// Remove deletes the identity's status file. A missing file is success.
func Remove(p *paths.Paths, identity string) error {
	path, err := Path(p, identity)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}
