package doctor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
	"github.com/cloudposse/atmos/pkg/xdg"
)

type candidate struct {
	doc     *document
	content []byte
	edits   []edit
}

// RecoveryEntry records enough information to restore the exact original bytes.
type RecoveryEntry struct {
	Path   string       `json:"path"`
	Copy   string       `json:"copy"`
	Mode   RecoveryMode `json:"mode"`
	Before string       `json:"before_sha256"`
	After  string       `json:"after_sha256"`
}

// RecoveryMode is a file's permission bits. It is written as the octal string chmod accepts, for
// example "0644", and reads both that form and the decimal number older manifests recorded.
type RecoveryMode uint32

// MarshalJSON writes the mode as an octal string.
func (m RecoveryMode) MarshalJSON() ([]byte, error) {
	defer perf.Track(nil, "doctor.RecoveryMode.MarshalJSON")()

	return json.Marshal(fmt.Sprintf("%04o", uint32(m)))
}

// UnmarshalJSON reads an octal string, or the decimal number written by older manifests.
func (m *RecoveryMode) UnmarshalJSON(data []byte) error {
	defer perf.Track(nil, "doctor.RecoveryMode.UnmarshalJSON")()

	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		var legacy uint32
		if legacyErr := json.Unmarshal(data, &legacy); legacyErr != nil {
			return fmt.Errorf("%w: file mode must be an octal string such as \"0644\"", ErrRecoveryManifest)
		}
		*m = RecoveryMode(legacy)
		return nil
	}
	value, err := strconv.ParseUint(text, octalBase, modeBits)
	if err != nil {
		return fmt.Errorf("%w: file mode %q is not octal", ErrRecoveryManifest, text)
	}
	*m = RecoveryMode(value)
	return nil
}

func digest(content []byte) string { sum := sha256.Sum256(content); return hex.EncodeToString(sum[:]) }

func (e *Engine) fix(ctx context.Context, p *project, o *Options, r *Report, edits []edit) error {
	v := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})
	if err := e.verifySchemaClosure(ctx, p, o); err != nil {
		return err
	}
	if l := p.projectLimit(); l != nil {
		markIncompleteRepairs(p, r, edits, l)
		return nil
	}
	candidates, err := e.stageCandidates(ctx, p, r, edits)
	if err != nil {
		return err
	}
	candidates = e.verifyCandidates(p, r, candidates, v)
	if len(candidates) == 0 {
		return nil
	}
	verified, err := e.verifyCandidateProject(ctx, o, r, candidates)
	if err != nil || !verified {
		return err
	}
	if err = e.unchanged(ctx, p, o); err != nil {
		return err
	}
	if o.DryRun {
		markDryRun(r, candidates)
		return nil
	}
	return e.applyVerified(ctx, p, o, r, candidates)
}

func (e *Engine) stageCandidates(ctx context.Context, p *project, r *Report, edits []edit) ([]candidate, error) {
	groups := map[*document][]edit{}
	for _, change := range edits {
		groups[change.doc] = append(groups[change.doc], change)
	}
	var candidates []candidate
	for _, d := range p.docs {
		changes := groups[d]
		if len(changes) == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		e.emit(Event{Phase: "Stage fixes", Label: relative(p.root, d.path), Total: len(groups)})
		if isSymlink(d.path) {
			// A symlinked document may be read, but a write would replace the link or edit a shared target.
			rejectSymlinked(r, changes)
			e.emit(Event{Phase: "Stage fixes", Label: relative(p.root, d.path), Done: true, Status: statusVerificationIncomplete, Total: len(groups)})
			continue
		}
		content, err := transformDocument(d, changes)
		if err != nil {
			rejectCandidate(r, changes, err)
		} else {
			candidates = append(candidates, candidate{d, content, changes})
		}
		e.emit(Event{Phase: "Stage fixes", Label: relative(p.root, d.path), Done: true, Status: statusFor(err), Total: len(groups)})
	}
	return candidates, nil
}

func transformDocument(d *document, edits []edit) ([]byte, error) {
	content := bytes.Clone(d.content)
	for _, change := range edits {
		var err error
		content, err = applyEdit(content, &change)
		if err != nil {
			return nil, err
		}
	}
	return content, nil
}

func (e *Engine) verifyCandidates(p *project, r *Report, candidates []candidate, v validator.Validator) []candidate {
	var verified []candidate
	for _, c := range candidates {
		e.emit(Event{Phase: "Verify", Label: relative(p.root, c.doc.path), Total: len(candidates) + 1})
		err := verifyDocument(c.doc.content, c.content, c.edits)
		if err == nil {
			err = verifySchema(v, c.doc.schema, c.doc.content, c.content)
		}
		if err != nil {
			rejectCandidate(r, c.edits, err)
		} else {
			verified = append(verified, c)
		}
		e.emit(Event{Phase: "Verify", Label: relative(p.root, c.doc.path), Done: true, Status: statusFor(err), Total: len(candidates) + 1})
	}
	return verified
}

func verifySchema(v validator.Validator, source string, before, after []byte) error {
	if source == "" {
		return nil
	}
	baseline, err := schemaErrors(v, source, before)
	if err != nil {
		return err
	}
	changed, err := schemaErrors(v, source, after)
	if err != nil {
		return err
	}
	for message := range changed {
		if !baseline[message] {
			return fmt.Errorf("%w: new schema error: %s", ErrVerification, message)
		}
	}
	return nil
}

func rejectCandidate(r *Report, edits []edit, err error) {
	for _, change := range edits {
		f := &r.Findings[change.finding]
		f.Status = statusVerificationFailed
		f.Fixable = false
		f.Reason = err.Error()
		f.Remediation = change.manualRemediation()
	}
}

func (e *Engine) prepareWrites(ctx context.Context, p *project, o *Options, r *Report, candidates []candidate) error {
	e.gitWarnings(ctx, p, candidates, r)
	if err := ctx.Err(); err != nil {
		return err
	}
	e.emit(Event{Phase: "Back up", Label: "Exact original files"})
	if err := e.backup(candidates, r); err != nil {
		return err
	}
	e.emit(Event{Phase: "Back up", Label: "Exact original files", Done: true, Status: statusPassed})
	return e.unchanged(ctx, p, o)
}

func (e *Engine) applyCandidates(ctx context.Context, p *project, o *Options, r *Report, candidates []candidate) error {
	for i, c := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		e.emit(Event{Phase: "Apply", Label: relative(p.root, c.doc.path), Index: i, Total: len(candidates)})
		if err := e.unchanged(ctx, p, o); err != nil {
			return err
		}
		// A file can become a link after staging; the write must not follow it.
		if isSymlink(c.doc.path) {
			return fmt.Errorf("%w: %s became a symlink; doctor does not write through symlinks", ErrChanged, relative(p.root, c.doc.path))
		}
		if err := e.FS.WriteFileAtomic(c.doc.path, c.content, c.doc.mode); err != nil {
			return err
		}
		// Record each successful write before observing cancellation or another failure.
		r.ChangedFiles = append(r.ChangedFiles, relative(p.root, c.doc.path))
		c.doc.content = c.content
		for _, change := range c.edits {
			f := &r.Findings[change.finding]
			f.Status = statusFixed
			f.Reason = change.fixedReason()
		}
		e.emit(Event{Phase: "Apply", Label: relative(p.root, c.doc.path), Index: i + 1, Total: len(candidates), Done: true, Status: statusFixed})
	}
	return nil
}

func (e *Engine) recheck(ctx context.Context, p *project, o *Options, r *Report) error {
	e.emit(Event{Phase: "Recheck", Label: "Written configuration"})
	if err := e.unchanged(ctx, p, o); err != nil {
		return err
	}
	fresh := *e
	fresh.Observe = nil
	recheckOptions := *o.rediscoveryOptions()
	recheckOptions.Fix = false
	checked, err := fresh.Run(ctx, recheckOptions)
	if err != nil {
		return err
	}
	retainDecisions(p, r, &checked)
	for _, check := range r.Checks {
		if check.ID == "semantic-verification" {
			checked.Checks = append(checked.Checks, check)
		}
	}
	r.Checks = checked.Checks
	e.emit(Event{Phase: "Recheck", Label: "Written configuration", Done: true, Status: "completed"})
	return nil
}

type sourceOverlay struct {
	filesystem.FileSystem
	files map[string][]byte
}

func (f sourceOverlay) ReadFile(path string) ([]byte, error) {
	defer perf.Track(nil, "doctor.sourceOverlay.ReadFile")()
	if content, ok := f.files[filepath.Clean(path)]; ok {
		return bytes.Clone(content), nil
	}
	return f.FileSystem.ReadFile(path)
}

func (e *Engine) verifyProject(ctx context.Context, o *Options, candidates []candidate) error {
	options := *o.rediscoveryOptions()
	options.Fix = false
	options.DryRun = false
	options.Only = []string{ruleSchema, ruleImports, ruleStacks}
	baselineEngine := *e
	baselineEngine.Observe = nil
	before, err := baselineEngine.Run(ctx, options)
	if err != nil {
		return err
	}
	overlay := sourceOverlay{FileSystem: e.FS, files: map[string][]byte{}}
	for _, candidate := range candidates {
		overlay.files[candidate.doc.path] = candidate.content
	}
	candidateEngine := baselineEngine
	candidateEngine.FS = overlay
	after, err := candidateEngine.Run(ctx, options)
	if err != nil {
		return err
	}
	errorsBefore := map[string]int{}
	key := func(f Finding) string { return f.RuleID + "\x00" + f.File + "\x00" + f.Path + "\x00" + f.Message }
	for i := range before.Findings {
		f := &before.Findings[i]
		if f.Severity == validation.SeverityError {
			errorsBefore[key(*f)]++
		}
	}
	for i := range after.Findings {
		f := &after.Findings[i]
		if f.Severity != validation.SeverityError {
			continue
		}
		k := key(*f)
		if errorsBefore[k] == 0 {
			return fmt.Errorf("%w: candidate introduces %s in %s", ErrVerification, f.RuleID, f.File)
		}
		errorsBefore[k]--
	}
	return nil
}

func statusFor(err error) string {
	if err != nil {
		return statusVerificationFailed
	}
	return statusPassed
}

func schemaErrors(v validator.Validator, source string, content []byte) (map[string]bool, error) {
	results, err := v.ValidateYAMLContent(source, content)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, result := range results {
		out[result.Field()+": "+result.Description()] = true
	}
	return out, nil
}

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

func (e *Engine) gitWarnings(ctx context.Context, p *project, candidates []candidate, r *Report) {
	git := func(args ...string) ([]byte, error) {
		//nolint:gosec // Read-only Git subcommands use argument arrays and place paths after --.
		return exec.CommandContext(ctx, "git", append([]string{"-C", p.root}, args...)...).Output()
	}
	warn := func(file, message string) {
		r.Findings = append(r.Findings, Finding{RuleID: "recovery", Category: "recovery", Severity: validation.SeverityWarning, File: file, Message: message, Status: statusManual})
		e.emit(Event{Phase: "Back up", Label: message, Status: "warning", Done: true})
	}
	if _, err := git("rev-parse", "--show-toplevel"); err != nil {
		warn("", "Git is unavailable or this project is outside a repository; exact backups will be created before editing")
		return
	}
	for _, c := range candidates {
		path := relative(p.root, c.doc.path)
		out, err := git("status", "--porcelain=v1", "--untracked-files=all", "--ignored", "--", path)
		if err != nil {
			warn(path, "Unable to inspect Git status for "+path)
			continue
		}
		if len(strings.TrimSpace(string(out))) > 0 {
			warn(path, "Editing "+path+": staged, unstaged, untracked, or ignored content exists; preserve the recovery backup")
		}
	}
}

func (e *Engine) verifySchemaClosure(ctx context.Context, p *project, o *Options) error {
	options := *o
	options.Only = []string{ruleSchema}
	options.Affected = false
	return e.customSchemas(ctx, p, &options, &Report{}, validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{}))
}

func markIncompleteRepairs(p *project, r *Report, edits []edit, l *limit) {
	r.Checks = append(r.Checks, Check{ID: "semantic-verification", Status: "incomplete", Reason: "The complete schema and source closure could not be captured"})
	for _, change := range edits {
		finding := &r.Findings[change.finding]
		finding.Status = statusVerificationIncomplete
		finding.Fixable = false
		finding.Reason = "Not fixed automatically because " + l.describe(p.root)
		finding.Remediation = change.manualRemediation()
	}
}

func retainDecisions(p *project, r, checked *Report) {
	var retained []Finding
	decisions := map[string]Finding{}
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.Status == statusFixed || f.RuleID == "recovery" {
			retained = append(retained, *f)
		}
		// A rejected repair keeps its decision; the recheck cannot know why staging refused it.
		if f.Status == statusVerificationFailed || f.Reason == reasonSymlinkedSource {
			decisions[f.RuleID+"/"+f.File+"/"+f.Path] = *f
		}
	}
	for i := range checked.Findings {
		f := &checked.Findings[i]
		if original, ok := decisions[f.RuleID+"/"+filepath.Join(p.root, f.File)+"/"+f.Path]; ok {
			*f = original
		}
	}
	retained = append(retained, checked.Findings...)
	r.Findings = retained
}

func (e *Engine) verifyCandidateProject(ctx context.Context, o *Options, r *Report, candidates []candidate) (bool, error) {
	e.emit(Event{Phase: "Verify", Label: "Complete candidate project", Total: len(candidates) + 1})
	if err := e.verifyProject(ctx, o, candidates); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		for _, c := range candidates {
			rejectCandidate(r, c.edits, err)
		}
		e.emit(Event{Phase: "Verify", Label: "Complete candidate project", Total: len(candidates) + 1, Done: true, Status: statusVerificationFailed})
		return false, nil
	}
	e.emit(Event{Phase: "Verify", Label: "Complete candidate project", Total: len(candidates) + 1, Done: true, Status: statusPassed})
	r.Checks = append(r.Checks, Check{ID: "semantic-verification", Status: statusPassed, Files: len(candidates), Reason: "Production field consumers, unrelated YAML nodes, and candidate schemas compared through a source overlay"})
	return true, nil
}

func markDryRun(r *Report, candidates []candidate) {
	for _, c := range candidates {
		for _, change := range c.edits {
			r.Findings[change.finding].Reason = "Candidate verified; dry run left the source unchanged"
		}
	}
}

func (e *Engine) applyVerified(ctx context.Context, p *project, o *Options, r *Report, candidates []candidate) error {
	if err := e.prepareWrites(ctx, p, o, r, candidates); err != nil {
		return err
	}
	if err := e.applyCandidates(ctx, p, o, r, candidates); err != nil {
		return err
	}
	return e.recheck(ctx, p, o, r)
}
