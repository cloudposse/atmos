package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/xdg"
)

func (e *Engine) backup(candidates []candidate, r *Report) error {
	var base string
	var err error
	if e.BackupDir != nil {
		base, err = e.BackupDir()
	} else {
		base, err = xdg.GetXDGDataDir("doctor/backups", privateDirectoryMode)
	}
	if err != nil {
		return err
	}
	if err = os.MkdirAll(base, privateDirectoryMode); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(base, time.Now().UTC().Format("20060102T150405Z-")+"*")
	if err != nil {
		return err
	}
	r.Backup = dir
	entries := make([]RecoveryEntry, 0, len(candidates))
	for i, c := range candidates {
		name := fmt.Sprintf("%06d.original", i)
		if err = os.WriteFile(filepath.Join(dir, name), c.doc.content, privateFileMode); err != nil {
			return err
		}
		entries = append(entries, RecoveryEntry{Path: c.doc.path, Copy: name, Mode: RecoveryMode(c.doc.mode.Perm()), Before: digest(c.doc.content), After: digest(c.content)})
	}
	manifest, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "manifest.json"), append(manifest, '\n'), privateFileMode); err != nil {
		return err
	}
	r.Recovery = "Use manifest.json to copy each .original backup to its recorded path and restore its recorded permission mode. Compare after_sha256 before overwriting later edits."
	return nil
}

// gitWarnings tells the user which edited files carry Git content the edit could hide. The result is at
// most two notices however many files were edited, because the guidance is the same for all of them and
// it is not a problem left in the project.
func (e *Engine) gitWarnings(ctx context.Context, p *project, candidates []candidate, r *Report) {
	git := func(args ...string) ([]byte, error) {
		//nolint:gosec // Read-only Git subcommands use argument arrays and place paths after --.
		return exec.CommandContext(ctx, "git", append([]string{"-C", p.root}, args...)...).Output()
	}
	notice := func(message, reason string) {
		r.Findings = append(r.Findings, Finding{RuleID: ruleRecovery, Category: "recovery", Severity: validation.SeverityNotice, Message: message, Reason: reason, Status: statusManual})
		e.emit(Event{Phase: "Back up", Label: message, Status: "warning", Done: true})
	}
	if _, err := git("rev-parse", "--show-toplevel"); err != nil {
		if ctx.Err() == nil {
			notice("Git is unavailable or this project is outside a repository; exact backups will be created before editing", "")
		}
		return
	}
	var dirty, uninspectable []string
	for _, c := range candidates {
		if ctx.Err() != nil {
			// A cancelled run learns nothing about Git; reporting every file as uninspectable would be noise.
			return
		}
		path := relative(p.root, c.doc.path)
		out, err := git("status", "--porcelain=v1", "--untracked-files=all", "--ignored", "--", path)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			uninspectable = append(uninspectable, path)
		case len(strings.TrimSpace(string(out))) > 0:
			dirty = append(dirty, path)
		}
	}
	if len(dirty) > 0 {
		notice(fmt.Sprintf("%s had staged, unstaged, untracked, or ignored content; keep the recovery backup", editedFiles(len(dirty))), SummarizePaths(dirty))
	}
	if len(uninspectable) > 0 {
		notice(fmt.Sprintf("Git status unavailable for %s; keep the recovery backup", editedFiles(len(uninspectable))), SummarizePaths(uninspectable))
	}
}

// editedFiles counts edited files in a sentence.
func editedFiles(n int) string {
	if n == 1 {
		return "1 edited file"
	}
	return strconv.Itoa(n) + " edited files"
}

// summarizePaths lists at most maxListedPaths paths and counts the rest.
func SummarizePaths(paths []string) string {
	defer perf.Track(nil, "doctor.SummarizePaths")()

	listed := paths
	if len(listed) > maxListedPaths {
		listed = listed[:maxListedPaths]
	}
	out := strings.Join(listed, ", ")
	if rest := len(paths) - len(listed); rest > 0 {
		out += fmt.Sprintf(", and %d more", rest)
	}
	return out
}
