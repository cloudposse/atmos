package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	permissionBits       = 0o777
	defaultConfigMode    = 0o644
	privateDirectoryMode = 0o700
)

// Change is one root-level configuration or lock replacement. Before is nil
// only when the file did not exist. A journal preserves both versions on failure.
type Change struct {
	Name   string `json:"name"`
	Before []byte `json:"before"`
	After  []byte `json:"after"`
	Mode   uint32 `json:"mode,omitempty"`
}

func journalPath(project string) string {
	return filepath.Join(project, ".atmos", "plugins", "transaction.json")
}

// CheckTransaction prevents ordinary commands from observing a partial commit. The error
// names the journal and says exactly what resolves it, since only some journals are
// repaired by `atmos plugin install`.
func CheckTransaction(project string) error {
	defer perf.Track(nil, "config.CheckTransaction")()

	// Windows reports a child of a regular file as missing rather than ENOTDIR.
	// Validate the project itself before treating a missing journal as a clean transaction.
	if info, err := os.Stat(project); err == nil && !info.IsDir() {
		return errUtils.ErrPluginInvalidConfig
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if _, err := os.Stat(journalPath(project)); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	journal := displayPath(journalPath(project))
	builder := errUtils.Build(errUtils.ErrPluginTransactionPending).
		WithHintf("The plugin transaction journal is `%s`.", journal)
	changes, parseErr := parseJournal(project)
	if errors.Is(parseErr, errJournalUnparseable) {
		return builder.
			WithHintf("The journal is not valid JSON. `atmos plugin install` repairs only an unparseable journal: it moves the file aside to `%s%s<timestamp>` and continues.", journal, QuarantineSuffix).
			Err()
	}
	if parseErr != nil {
		return errors.Join(builder.Err(), parseErr)
	}
	if conflicts := conflictingFiles(project, changes); len(conflicts) != 0 {
		return builder.
			WithHintf("The journal conflicts with external edits to %s. `atmos plugin install` repairs only an unparseable journal, so it cannot resolve this.", quoteNames(conflicts)).
			WithHintf("Review `%s`, then remove it or restore the named files to the state it records.", journal).
			Err()
	}
	return builder.WithHintf("The journal records an interrupted commit that matches the current files. Run `atmos plugin install` to finish it.").Err()
}

// Mutate serializes plugin state changes and recovers interrupted commits before
// preparing another. The callback stages complete file bytes without writing them.
func Mutate(ctx context.Context, project string, prepare func() ([]Change, error)) error {
	defer perf.Track(nil, "pluginconfig.Mutate")()

	journal := journalPath(project)
	if err := os.MkdirAll(filepath.Dir(journal), privateDirectoryMode); err != nil {
		return err
	}
	return filelock.New(journal+".lock").WithExclusive(ctx, func() error {
		if err := recoverTransaction(project); err != nil {
			return err
		}
		changes, err := prepare()
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			return nil
		}
		if err := validateChanges(project, changes); err != nil {
			return err
		}
		if err := setChangeModes(project, changes); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := json.Marshal(changes)
		if err != nil {
			return err
		}
		if err := filesystem.NewOSFileSystem().WriteFileAtomic(journal, raw, 0o600); err != nil {
			return err
		}
		// Once journaled, finish the small commit even if the request is canceled.
		return recoverTransaction(project)
	})
}

func recoverTransaction(project string) error {
	changes, err := parseJournal(project)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if errors.Is(err, errJournalUnparseable) {
		return quarantineJournal(project, err)
	}
	if err != nil {
		return err
	}
	if err := validateChanges(project, changes); err != nil {
		return journalConflict(project, changes, err)
	}
	for _, change := range changes {
		if err := filesystem.NewOSFileSystem().WriteFileAtomic(filepath.Join(project, change.Name), change.After, changeMode(change)); err != nil {
			return errors.Join(errUtils.ErrPluginTransactionPending, err)
		}
	}
	return os.Remove(journalPath(project))
}

var errJournalUnparseable = errors.New("plugin transaction journal is not valid JSON")

// parseJournal returns the recorded changes. It reports os.ErrNotExist when no journal
// exists and errJournalUnparseable (with the decode cause) when the bytes are not a journal.
func parseJournal(project string) ([]Change, error) {
	relative, err := filepath.Rel(project, journalPath(project))
	if err != nil {
		return nil, err
	}
	raw, err := manifest.ReadFile(project, relative)
	if err != nil {
		return nil, err
	}
	var changes []Change
	if err := json.Unmarshal(raw, &changes); err != nil {
		return nil, errors.Join(errJournalUnparseable, err)
	}
	return changes, nil
}

// quarantineJournal moves an unparseable journal aside so a mutation can proceed. A
// journal that cannot be decoded records nothing that could be applied or lost, and it
// stays on disk for inspection. Parseable journals are never discarded here.
func quarantineJournal(project string, cause error) error {
	journal := journalPath(project)
	moved, err := Quarantine(journal, time.Now())
	if err != nil {
		return errors.Join(errUtils.ErrPluginTransactionPending, cause, err)
	}
	ui.Warningf("The plugin transaction journal `%s` was not valid JSON; moved it to `%s` and continuing.", displayPath(journal), displayPath(moved))
	return nil
}

// journalConflict explains a parseable journal that disagrees with the files it describes.
func journalConflict(project string, changes []Change, cause error) error {
	if !errors.Is(cause, errUtils.ErrPluginTransactionPending) {
		return cause
	}
	journal := displayPath(journalPath(project))
	builder := errUtils.Build(cause).
		WithHintf("The plugin transaction journal `%s` conflicts with the current files.", journal)
	if conflicts := conflictingFiles(project, changes); len(conflicts) != 0 {
		builder.WithHintf("%s changed outside Atmos after the transaction was journaled.", quoteNames(conflicts))
	}
	return builder.
		WithHintf("Review `%s`, then remove it or restore the named files to the state it records. `atmos plugin install` does not discard a journal it can parse.", journal).
		Err()
}

func conflictingFiles(project string, changes []Change) []string {
	var names []string
	for _, change := range changes {
		current, err := os.ReadFile(filepath.Join(project, change.Name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			names = append(names, change.Name)
			continue
		}
		if !bytes.Equal(current, change.Before) && !bytes.Equal(current, change.After) {
			names = append(names, change.Name)
		}
	}
	return names
}

// displayPath makes paths in messages exact regardless of the working directory.
func displayPath(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		return absolute
	}
	return path
}

func quoteNames(names []string) string {
	quoted := make([]string, len(names))
	for index, name := range names {
		quoted[index] = "`" + name + "`"
	}
	return strings.Join(quoted, ", ")
}

func validateChanges(project string, changes []Change) error {
	seen := make(map[string]bool)
	for _, change := range changes {
		if seen[change.Name] {
			return errUtils.ErrPluginInvalidConfig
		}
		seen[change.Name] = true
		if err := validateChange(&change); err != nil {
			return err
		}
		current, err := os.ReadFile(filepath.Join(project, change.Name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if !bytes.Equal(current, change.Before) && !bytes.Equal(current, change.After) {
			return errUtils.ErrPluginTransactionPending
		}
	}
	return nil
}

func replacementMode(project, name string) (os.FileMode, error) {
	info, err := os.Lstat(filepath.Join(project, name))
	if errors.Is(err, os.ErrNotExist) {
		return defaultConfigMode, nil
	}
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, errUtils.ErrPluginInvalidConfig
	}
	return info.Mode().Perm(), nil
}

func changeMode(change Change) os.FileMode {
	if change.Mode == 0 {
		return defaultConfigMode
	}
	return os.FileMode(change.Mode)
}

func setChangeModes(project string, changes []Change) error {
	for index := range changes {
		mode, err := replacementMode(project, changes[index].Name)
		if err != nil {
			return err
		}
		changes[index].Mode = uint32(mode)
	}
	return nil
}

func validateChange(change *Change) error {
	if change.Mode & ^uint32(permissionBits) != 0 || !filepath.IsLocal(change.Name) || filepath.Base(change.Name) != change.Name || len(change.After) == 0 {
		return errUtils.ErrPluginInvalidConfig
	}
	return nil
}
