package local

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

// A root has at most one transaction because managed operations hold its lock.
// The marker is hidden by the existing temporary-file listing rule.
const transactionName = tempPrefix + "transaction"

type transaction struct {
	Target    string `json:"target"`
	DataTemp  string `json:"data_temp"`
	AttrsTemp string `json:"attrs_temp,omitempty"`
	HadAttrs  bool   `json:"had_attrs"`
	OldAttrs  []byte `json:"old_attrs,omitempty"`
}

// commitManaged records the old attributes before publishing either replacement. The presence of
// DataTemp distinguishes a failed/incomplete data rename from a completed one during recovery.
func (s *Store) commitManaged(root *os.Root, key string, pending *staged, noAttrs bool, rename func(*os.Root, string, string) error) error {
	target := filepath.FromSlash(key)
	if err := s.writeTransaction(root, target, pending); err != nil {
		return wrapIO(blob.OpPut, key, err)
	}
	pending.preserve = true
	err := publishTransaction(root, target, pending, noAttrs, rename)
	if err != nil {
		if recoveryErr := s.recoverCommit(root); recoveryErr != nil {
			return wrapIO(blob.OpPut, key, errors.Join(err, recoveryErr))
		}
		pending.preserve = false
		return wrapIO(blob.OpPut, key, err)
	}
	if err := s.recoverCommit(root); err != nil {
		return wrapIO(blob.OpPut, key, err)
	}
	pending.preserve = false
	pending.tempPath, pending.attrsTemp = "", ""
	return nil
}

func (s *Store) writeTransaction(root *os.Root, target string, pending *staged) error {
	if _, err := root.Stat(pending.tempPath); err != nil {
		return err
	}
	old, err := rootReadFile(root, attrsPath(target))
	hadAttrs := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	txn := transaction{Target: target, DataTemp: pending.tempPath, AttrsTemp: pending.attrsTemp, HadAttrs: hadAttrs, OldAttrs: old}
	data, err := json.Marshal(txn)
	if err != nil {
		return err
	}
	file, name, err := rootCreateTemp(root, ".", s.dirPerm, s.filePerm)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(name) }()
	_, writeErr := file.Write(data)
	if err := firstError(writeErr, file.Close()); err != nil {
		return err
	}
	return rootRenameReplace(root, name, transactionName)
}

func publishTransaction(root *os.Root, target string, pending *staged, noAttrs bool, rename func(*os.Root, string, string) error) error {
	var err error
	if pending.attrsTemp != "" {
		err = rename(root, pending.attrsTemp, attrsPath(target))
	} else if noAttrs {
		err = root.Remove(attrsPath(target))
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
	}
	if err == nil {
		err = rename(root, pending.tempPath, target)
	}
	return err
}

// recoverCommit is idempotent: it restores the old attributes while the data temp still exists,
// and otherwise finishes cleanup after the data rename. Callers hold the root lock.
func (s *Store) recoverCommit(root *os.Root) error {
	data, err := rootReadFile(root, transactionName)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var txn transaction
	if err := json.Unmarshal(data, &txn); err != nil {
		return fmt.Errorf("decode local transaction: %w", err)
	}
	if err := s.validateTransaction(&txn); err != nil {
		return err
	}
	_, statErr := root.Stat(txn.DataTemp)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if statErr == nil {
		if err := s.restoreAttrs(root, &txn); err != nil {
			return err
		}
	}
	if err := root.Remove(transactionName); err != nil {
		return err
	}
	_ = root.Remove(txn.DataTemp)
	s.removeAttrsTemp(root, &txn)
	return nil
}

func (s *Store) validateTransaction(txn *transaction) error {
	if err := s.validate(blob.OpPut, filepath.ToSlash(txn.Target)); err != nil ||
		filepath.Dir(txn.DataTemp) != filepath.Dir(txn.Target) || !strings.HasPrefix(filepath.Base(txn.DataTemp), tempPrefix) {
		return fmt.Errorf("%w: invalid local transaction", errUtils.ErrBlobInvalid)
	}
	return nil
}

func (s *Store) removeAttrsTemp(root *os.Root, txn *transaction) {
	if txn.AttrsTemp != "" && filepath.Dir(txn.AttrsTemp) == filepath.Dir(txn.Target) && strings.HasPrefix(filepath.Base(txn.AttrsTemp), tempPrefix) {
		_ = root.Remove(txn.AttrsTemp)
	}
}

func (s *Store) restoreAttrs(root *os.Root, txn *transaction) error {
	if !txn.HadAttrs {
		err := root.Remove(attrsPath(txn.Target))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	file, name, err := rootCreateTemp(root, filepath.Dir(txn.Target), s.dirPerm, s.filePerm)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(name) }()
	_, writeErr := file.Write(txn.OldAttrs)
	if err := firstError(writeErr, file.Close()); err != nil {
		return err
	}
	return rootRenameReplace(root, name, attrsPath(txn.Target))
}
