package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
)

type sourceJournalEntry struct {
	name               string
	existed, installed bool
}

type sourcePublication struct {
	stage, target, backup string
	rename                sourceRename
	journal               []sourceJournalEntry
}

func (p *sourcePublication) publish(ctx context.Context, changes []string, incoming map[string]bool) (retErr error) {
	defer func() {
		if retErr != nil {
			if err := p.rollback(); err != nil {
				// Preserve the last usable backup if restoration itself fails.
				retErr = errors.Join(retErr, fmt.Errorf("source rollback journal retained at %s: %w", p.backup, err))
				return
			}
		}
		_ = os.RemoveAll(p.backup)
	}()
	for _, name := range changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := p.install(name, incoming[name]); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (p *sourcePublication) install(name string, incoming bool) error {
	dest := filepath.Join(p.target, name)
	if err := os.MkdirAll(filepath.Dir(dest), TargetDirPermissions); err != nil {
		return err
	}
	item, err := p.backupFile(name)
	if err != nil {
		return err
	}
	p.journal = append(p.journal, item)
	if incoming {
		if err := p.rename(filepath.Join(p.stage, name), dest); err != nil {
			return err
		}
		p.journal[len(p.journal)-1].installed = true
	}
	return nil
}

func (p *sourcePublication) rollback() error {
	var result error
	for i := len(p.journal) - 1; i >= 0; i-- {
		item := p.journal[i]
		dest := filepath.Join(p.target, item.name)
		if item.installed {
			if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
				result = errors.Join(result, err)
			}
		}
		if item.existed {
			if err := os.Rename(filepath.Join(p.backup, item.name), dest); err != nil {
				result = errors.Join(result, fmt.Errorf("restore source %s: %w", item.name, err))
			}
		}
	}
	return result
}

func (p *sourcePublication) backupFile(name string) (sourceJournalEntry, error) {
	dest := filepath.Join(p.target, name)
	item := sourceJournalEntry{name: name}
	info, err := os.Lstat(dest)
	if os.IsNotExist(err) {
		return item, nil
	}
	if err != nil {
		return item, err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(dest)
		if err != nil {
			return item, err
		}
		if len(entries) != 0 {
			return item, fmt.Errorf("%w: source file conflicts with nonempty directory %s", errUtils.ErrSourceCopyFailed, name)
		}
	}
	saved := filepath.Join(p.backup, name)
	if err := os.MkdirAll(filepath.Dir(saved), TargetDirPermissions); err != nil {
		return item, err
	}
	if err := p.rename(dest, saved); err != nil {
		return item, err
	}
	item.existed = true
	return item, nil
}
