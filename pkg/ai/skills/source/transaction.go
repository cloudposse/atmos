package source

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type journalEntry struct {
	Target, Stage, Backup string
	HadOriginal, Remove   bool
}
type journal struct {
	Project     string
	ManualRoots []string
	Committed   bool
	Entries     []journalEntry
}

func (e *Engine) journals() []string {
	return []string{filepath.Join(e.stateDir(scopeProject), "transaction.json"), filepath.Join(e.stateDir(scopeUser), "transaction.json")}
}

func (e *Engine) pending() error {
	for _, path := range e.journals() {
		if err := safePath(path); err != nil {
			return err
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%w: %s; run skill sync --recover", ErrRecovery, path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	return writeAtomicMode(path, data, privateFileMode)
}

func writeAtomicMode(path string, data []byte, mode os.FileMode) error {
	if err := safePath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".skills-write-")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func (e *Engine) saveJournal(j journal) error {
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	for _, path := range e.journals() {
		if err = writeAtomic(path, raw); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) apply(ops []operation) error {
	j := journal{Project: e.Project, ManualRoots: e.manualRoots}
	for _, op := range ops {
		entry, err := stageOperation(&op)
		if err != nil {
			cleanupStages(j)
			return err
		}
		j.Entries = append(j.Entries, entry)
	}
	if err := e.saveJournal(j); err != nil {
		cleanupStages(j)
		return err
	}
	for _, entry := range j.Entries {
		if err := e.applyEntry(entry); err != nil {
			return errors.Join(err, e.rollback(j))
		}
	}
	j.Committed = true
	if err := e.saveJournal(j); err != nil {
		return fmt.Errorf("%w: cannot mark transaction committed: %w", ErrRecovery, err)
	}
	return e.cleanupJournal(j)
}

func stageOperation(op *operation) (journalEntry, error) {
	entry := journalEntry{Target: op.Target, Remove: op.Remove}
	if err := safePath(op.Target); err != nil {
		return entry, err
	}
	if err := os.MkdirAll(filepath.Dir(op.Target), directoryMode); err != nil {
		return entry, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(op.Target), ".skills-stage-")
	if err != nil {
		return entry, err
	}
	entry.Stage = stage
	entry.Backup = stage + ".backup"
	_, err = os.Stat(op.Target)
	entry.HadOriginal = err == nil
	if err != nil && !os.IsNotExist(err) {
		_ = os.RemoveAll(stage)
		return entry, err
	}
	if err = populateStage(op, stage); err != nil {
		_ = os.RemoveAll(stage)
		return entry, err
	}
	return entry, nil
}

func populateStage(op *operation, stage string) error {
	if op.Remove {
		return nil
	}
	if op.Source != "" {
		return copyTree(op.Source, stage)
	}
	if err := os.Remove(stage); err != nil {
		return err
	}
	return os.WriteFile(stage, op.Data, privateFileMode)
}

func (e *Engine) applyEntry(entry journalEntry) error {
	if err := safePath(entry.Target); err != nil {
		return err
	}
	if entry.HadOriginal {
		if err := e.Rename(entry.Target, entry.Backup); err != nil {
			return err
		}
	}
	if !entry.Remove {
		return e.Rename(entry.Stage, entry.Target)
	}
	return nil
}

func cleanupStages(j journal) {
	for _, entry := range j.Entries {
		_ = os.RemoveAll(entry.Stage)
	}
}

func (e *Engine) rollback(j journal) error {
	var failures []error
	for i := len(j.Entries) - 1; i >= 0; i-- {
		if err := e.restoreEntry(j.Entries[i]); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return errors.Join(append([]error{ErrRecovery}, failures...)...)
	}
	return e.cleanupJournal(j)
}

func (e *Engine) restoreEntry(entry journalEntry) error {
	if err := safePath(entry.Target); err != nil {
		return err
	}
	_, backupErr := os.Stat(entry.Backup)
	if backupErr == nil {
		if err := os.RemoveAll(entry.Target); err != nil {
			return err
		}
		return e.Rename(entry.Backup, entry.Target)
	}
	if !os.IsNotExist(backupErr) {
		return backupErr
	}
	_, stageErr := os.Stat(entry.Stage)
	if !entry.HadOriginal && os.IsNotExist(stageErr) && !entry.Remove {
		return os.RemoveAll(entry.Target)
	}
	return nil
}

func (e *Engine) cleanupJournal(j journal) error {
	for _, entry := range j.Entries {
		for _, p := range []string{entry.Stage, entry.Backup} {
			if err := os.RemoveAll(p); err != nil {
				return fmt.Errorf("%w: %w", ErrRecovery, err)
			}
		}
	}
	for _, path := range e.journals() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Recover rolls back an interrupted apply or cleans a committed transaction.
func (e *Engine) Recover() error {
	unlock, err := e.acquire(false)
	if err != nil {
		return err
	}
	defer unlock()
	j, err := e.readJournal()
	if err != nil {
		return err
	}
	if j.Project == "" {
		return nil
	}
	if j.Project != e.Project {
		return fmt.Errorf("%w: recover from project %s", ErrRecovery, j.Project)
	}
	e.manualRoots = j.ManualRoots
	for _, entry := range j.Entries {
		if !e.validJournalEntry(entry) {
			return ErrInvalid
		}
	}
	if j.Committed {
		return e.cleanupJournal(j)
	}
	return e.rollback(j)
}

func (e *Engine) readJournal() (journal, error) {
	for _, path := range e.journals() {
		if err := safePath(path); err != nil {
			return journal{}, err
		}
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return journal{}, err
		}
		var j journal
		err = json.Unmarshal(raw, &j)
		return j, err
	}
	return journal{}, nil
}

//nolint:revive // Recovery validates every path before interpreting the journal as filesystem operations.
func (e *Engine) validJournalEntry(entry journalEntry) bool {
	if !strings.HasPrefix(filepath.Base(entry.Stage), ".skills-stage-") {
		return false
	}
	if filepath.Dir(entry.Stage) != filepath.Dir(entry.Target) || entry.Backup != entry.Stage+".backup" {
		return false
	}
	if e.manualJournalTarget(entry.Target) {
		return true
	}
	if entry.Target == e.lockPath() {
		return true
	}
	for _, root := range e.ownedRoots() {
		rel, err := filepath.Rel(root, entry.Target)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func (e *Engine) ownedRoots() []string {
	roots := []string{e.stateDir(scopeProject), e.stateDir(scopeUser)}
	for _, scope := range []string{scopeProject, scopeUser} {
		for _, client := range []string{"claude-code", "vscode", "gemini"} {
			target, _ := e.destination(scope, client, "placeholder")
			roots = append(roots, filepath.Dir(target))
		}
	}
	return roots
}

func (e *Engine) manualJournalTarget(target string) bool {
	for _, root := range e.manualRoots {
		if filepath.IsAbs(root) && filepath.Dir(root) != root && filepath.Dir(target) == root && skillNamePattern.MatchString(filepath.Base(target)) {
			return true
		}
	}

	return false
}
