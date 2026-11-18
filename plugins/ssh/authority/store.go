package authority

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	storeVersion = 1
	tempPrefix   = ".tmp-"
	receiptExt   = ".json"
)

// Store keeps receipts as JSON files in a directory: one file per authority and key name. The
// files hold no credential.
type Store struct {
	dir string
}

type receiptFile struct {
	Version int     `json:"version"`
	Receipt Receipt `json:"receipt"`
}

// NewStore returns a store rooted at dir. The directory is created on the first Save.
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("%w: receipt directory is empty", ErrInvalidRequest)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the store's directory.
func (s *Store) Dir() string { return s.dir }

// encodeName makes a name safe as a file name component on every platform. Letters, digits, '_'
// and '-' pass through and everything else, including '.', becomes %XX, so '.' can separate parts.
func encodeName(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func (s *Store) file(authorityName, keyName string) (string, error) {
	if authorityName == "" || keyName == "" {
		return "", fmt.Errorf("%w: receipts need an authority and a key name", ErrInvalidRequest)
	}
	return filepath.Join(s.dir, encodeName(authorityName)+"."+encodeName(keyName)+receiptExt), nil
}

// Load reads the receipt for the key and authority. It returns ErrReceiptNotFound when there is
// none and ErrReceiptCorrupt when the file cannot be understood.
func (s *Store) Load(authorityName, keyName string) (Receipt, error) {
	name, err := s.file(authorityName, keyName)
	if err != nil {
		return Receipt{}, err
	}
	data, err := os.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return Receipt{}, fmt.Errorf("%w: %s for key %q", ErrReceiptNotFound, authorityName, keyName)
	}
	if err != nil {
		return Receipt{}, fmt.Errorf("read receipt %s: %w", name, err)
	}
	r, err := decodeReceipt(name, data)
	if err != nil {
		return Receipt{}, err
	}
	if r.Authority != authorityName || r.KeyName != keyName {
		return Receipt{}, fmt.Errorf("%w: %s holds %s/%q", ErrReceiptCollision, name, r.Authority, r.KeyName)
	}
	return r, nil
}

func decodeReceipt(name string, data []byte) (Receipt, error) {
	var f receiptFile
	if err := json.Unmarshal(data, &f); err != nil {
		return Receipt{}, fmt.Errorf("%w: %s: %w", ErrReceiptCorrupt, name, err)
	}
	if f.Version != storeVersion {
		return Receipt{}, fmt.Errorf("%w: %s: unsupported version %d", ErrReceiptCorrupt, name, f.Version)
	}
	r := f.Receipt
	if r.Authority == "" || r.KeyName == "" || r.Fingerprint == "" {
		return Receipt{}, fmt.Errorf("%w: %s: missing authority, key name, or fingerprint", ErrReceiptCorrupt, name)
	}
	return r, nil
}

// Save writes the receipt atomically with mode 0600, replacing any receipt for the same key and
// authority. It refuses to replace a file that belongs to a different key.
func (s *Store) Save(r Receipt) error {
	name, err := s.file(r.Authority, r.KeyName)
	if err != nil {
		return err
	}
	if _, err := s.Load(r.Authority, r.KeyName); errors.Is(err, ErrReceiptCollision) {
		return err
	}
	data, err := json.MarshalIndent(receiptFile{Version: storeVersion, Receipt: r}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode receipt: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create receipt directory: %w", err)
	}
	return writeAtomic(s.dir, name, data)
}

func writeAtomic(dir, name string, data []byte) (err error) {
	tmp, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return fmt.Errorf("create temporary receipt: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	// CreateTemp already creates the file with mode 0600.
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write receipt: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync receipt: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close receipt: %w", err)
	}
	if err = os.Rename(tmp.Name(), name); err != nil {
		return fmt.Errorf("replace receipt: %w", err)
	}
	return nil
}

// Delete removes the receipt. A missing receipt is success.
func (s *Store) Delete(authorityName, keyName string) error {
	name, err := s.file(authorityName, keyName)
	if err != nil {
		return err
	}
	if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete receipt: %w", err)
	}
	return nil
}

// List returns every readable receipt, ordered by authority and key name. Unreadable files are
// skipped and reported together in the returned error, so one bad file hides nothing else.
func (s *Store) List() ([]Receipt, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list receipts: %w", err)
	}
	var (
		out  []Receipt
		errs []error
	)
	for _, entry := range entries {
		n := entry.Name()
		if entry.IsDir() || strings.HasPrefix(n, tempPrefix) || !strings.HasSuffix(n, receiptExt) {
			continue
		}
		full := filepath.Join(s.dir, n)
		data, err := os.ReadFile(full)
		if err != nil {
			errs = append(errs, fmt.Errorf("read receipt %s: %w", full, err))
			continue
		}
		r, err := decodeReceipt(full, bytes.TrimSpace(data))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Authority != out[j].Authority {
			return out[i].Authority < out[j].Authority
		}
		return out[i].KeyName < out[j].KeyName
	})
	return out, errors.Join(errs...)
}
