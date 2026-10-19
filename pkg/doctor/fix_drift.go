package doctor

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validator"
)

// isSymlink reports whether path itself, not a directory above it, is a symbolic link.
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// rejectSymlinked leaves the findings of a symlinked document for manual review. Doctor never writes
// through a symlink, but the other documents' repairs still proceed.
func rejectSymlinked(r *Report, edits []edit) {
	for _, change := range edits {
		f := &r.Findings[change.finding]
		f.Status = statusVerificationIncomplete
		f.Fixable = false
		f.Reason = reasonSymlinkedSource
		f.Remediation = change.manualRemediation()
	}
}

// isReadOnly reports whether the file at path has no write permission bit set for anyone.
func isReadOnly(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().Perm()&writeBits == 0
}

// rejectReadOnly leaves the findings of a read-only document for manual review. A read-only file marks
// content its owner does not want edited, and an atomic rename would silently replace it.
func rejectReadOnly(r *Report, edits []edit) {
	for _, change := range edits {
		f := &r.Findings[change.finding]
		f.Status = statusVerificationIncomplete
		f.Fixable = false
		f.Reason = reasonReadOnlySource
		f.Remediation = change.manualRemediation()
	}
}

func (e *Engine) unchanged(ctx context.Context, p *project, o *Options) error {
	current, err := e.discover(ctx, o.rediscoveryOptions(), &Report{})
	if err != nil {
		return err
	}
	if len(current.docs) != len(p.docs) {
		return fmt.Errorf("%w: source closure changed", ErrChanged)
	}
	for _, d := range current.docs {
		if p.byPath[d.path] == nil {
			return fmt.Errorf("%w: new source %s", ErrChanged, relative(p.root, d.path))
		}
	}
	if err := e.unchangedSchemas(ctx, p, current, o); err != nil {
		return err
	}
	for _, d := range p.docs {
		if err := e.unchangedDocument(p, d); err != nil {
			return err
		}
	}
	return nil
}

// unchangedDocument reports whether the document still has the bytes and permissions captured at discovery.
func (e *Engine) unchangedDocument(p *project, d *document) error {
	// Drift is content and mode, read through any symlink; only writes refuse links.
	content, err := e.FS.ReadFile(d.path)
	if err != nil {
		return err
	}
	info, err := e.FS.Stat(d.path)
	if err != nil {
		return err
	}
	if !bytes.Equal(content, d.content) || info.Mode().Perm() != d.mode {
		return fmt.Errorf("%w: %s", ErrChanged, relative(p.root, d.path))
	}
	return nil
}

func (e *Engine) unchangedSchemas(ctx context.Context, p, current *project, o *Options) error {
	schemaOptions := *o
	schemaOptions.Only = []string{ruleSchema}
	schemaOptions.Affected = false
	if err := e.customSchemas(ctx, current, &schemaOptions, &Report{}, validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})); err != nil {
		return err
	}
	if len(current.extra) != len(p.extra) {
		return fmt.Errorf("%w: custom schema source closure changed", ErrChanged)
	}
	for path, before := range p.extra {
		after, err := e.FS.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(before, after) {
			return fmt.Errorf("%w: %s", ErrChanged, relative(p.root, path))
		}
	}
	return nil
}
