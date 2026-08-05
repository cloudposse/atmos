package step

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/archive"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ArchiveHandler packs (and, for zip/uncompressed tar, updates) zip/tar
// archives using pkg/archive — the Go standard library only, no external
// zip/tar binaries required. See docs/prd/archive-step.md. Available as a
// component lifecycle hook via the existing `kind: step` bridge
// (`kind: step` / `type: archive` / `with: {...}`); no dedicated hook kind
// is registered for it.
type ArchiveHandler struct {
	BaseHandler
}

func init() {
	Register(&ArchiveHandler{
		BaseHandler: NewBaseHandler("archive", CategoryCommand, false),
	})
}

var archiveValidActions = map[string]bool{
	string(archive.ActionCreate):  true,
	string(archive.ActionExtract): true,
	string(archive.ActionUpdate):  true,
	string(archive.ActionReplace): true,
}

var archiveValidMtimeModes = map[string]bool{
	"":                      true,
	archive.MtimeFilesystem: true,
	archive.MtimeEpoch:      true,
	archive.MtimeGit:        true,
}

// Validate checks the archive step configuration before execution.
func (h *ArchiveHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ArchiveHandler.Validate")()

	action := step.Action
	if action == "" {
		action = string(archive.ActionReplace)
	}
	if !archiveValidActions[action] {
		return errUtils.Build(errUtils.ErrArchiveStepInvalidAction).
			WithContext("step", step.Name).
			WithContext("action", action).
			WithHint("Use one of: create, extract, update, replace").
			Err()
	}
	// mtime is not validated here: it supports Go templates and
	// resolveArchiveOptions resolves it before archive.Run sees it, so
	// checking the raw (possibly templated) value here would reject a
	// template that resolves to a valid mode. See resolveArchiveMtime.

	if _, err := archiveSourceString(step); err != nil {
		return err
	}
	return h.ValidateRequired(step, "destination", step.Destination)
}

// Execute runs the configured archive action.
func (h *ArchiveHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ArchiveHandler.Execute")()

	opts, action, err := resolveArchiveOptions(step, vars)
	if err != nil {
		return nil, err
	}

	if err := archive.Run(action, &opts); err != nil {
		return nil, fmt.Errorf("step '%s': %w", step.Name, err)
	}

	return NewStepResult(opts.Destination).
		WithMetadata("action", string(action)).
		WithMetadata("destination", opts.Destination).
		WithMetadata("source", opts.Source), nil
}

func resolveArchiveOptions(step *schema.WorkflowStep, vars *Variables) (archive.PackOptions, archive.Action, error) {
	source, err := archiveSourceString(step)
	if err != nil {
		return archive.PackOptions{}, "", err
	}
	source, err = vars.Resolve(source)
	if err != nil {
		return archive.PackOptions{}, "", fmt.Errorf("step '%s': failed to resolve source: %w", step.Name, err)
	}

	destination, err := vars.Resolve(step.Destination)
	if err != nil {
		return archive.PackOptions{}, "", fmt.Errorf("step '%s': failed to resolve destination: %w", step.Name, err)
	}
	format, err := vars.Resolve(step.Format)
	if err != nil {
		return archive.PackOptions{}, "", fmt.Errorf("step '%s': failed to resolve format: %w", step.Name, err)
	}
	subpath, err := vars.Resolve(step.Subpath)
	if err != nil {
		return archive.PackOptions{}, "", fmt.Errorf("step '%s': failed to resolve subpath: %w", step.Name, err)
	}
	include, err := resolveArchiveGlobs(step.Include, vars)
	if err != nil {
		return archive.PackOptions{}, "", fmt.Errorf("step '%s': failed to resolve include: %w", step.Name, err)
	}
	exclude, err := resolveArchiveGlobs(step.Exclude, vars)
	if err != nil {
		return archive.PackOptions{}, "", fmt.Errorf("step '%s': failed to resolve exclude: %w", step.Name, err)
	}
	mtime, err := resolveArchiveMtime(step, vars)
	if err != nil {
		return archive.PackOptions{}, "", err
	}

	action := archive.Action(step.Action)
	if action == "" {
		action = archive.ActionReplace
	}

	return archive.PackOptions{
		Source:      source,
		Destination: destination,
		Format:      format,
		Subpath:     subpath,
		Include:     include,
		Exclude:     exclude,
		Mtime:       mtime,
	}, action, nil
}

// resolveArchiveMtime resolves mtime and validates the result, not the raw
// field — validating before resolution would reject a template that
// resolves to a valid mode.
func resolveArchiveMtime(step *schema.WorkflowStep, vars *Variables) (string, error) {
	mtime, err := vars.Resolve(step.Mtime)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve mtime: %w", step.Name, err)
	}
	if !archiveValidMtimeModes[mtime] {
		return "", errUtils.Build(errUtils.ErrArchiveInvalidMtimeMode).
			WithContext("step", step.Name).
			WithContext("mtime", mtime).
			WithHint("Use one of: filesystem, epoch, git").
			Err()
	}
	return mtime, nil
}

// archiveSourceString reads step.Source as a plain string. The field is
// shared with the workdir step type (which also accepts a source map);
// archive requires a plain string path.
func archiveSourceString(step *schema.WorkflowStep) (string, error) {
	if step.Source == nil {
		return "", errUtils.Build(errUtils.ErrArchiveSourceRequired).
			WithContext("step", step.Name).
			Err()
	}
	src, ok := step.Source.(string)
	if !ok {
		return "", errUtils.Build(errUtils.ErrArchiveStepInvalidSource).
			WithContext("step", step.Name).
			WithHint("Set 'source' to a plain string path").
			Err()
	}
	if src == "" {
		return "", errUtils.Build(errUtils.ErrArchiveSourceRequired).
			WithContext("step", step.Name).
			Err()
	}
	return src, nil
}

func resolveArchiveGlobs(patterns []string, vars *Variables) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	resolved := make([]string, len(patterns))
	for i, p := range patterns {
		r, err := vars.Resolve(p)
		if err != nil {
			return nil, err
		}
		resolved[i] = r
	}
	return resolved, nil
}
