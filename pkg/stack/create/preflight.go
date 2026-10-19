package create

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	overlayDirPermissions  = 0o700
	overlayFilePermissions = 0o600
	// The overlay follows at most this many nested symlinked directories.
	maxSymlinkDepth = 8
	// The wrap format joins a sentinel with its cause.
	wrapFormat = "%w: %w"
)

// collisionPattern extracts the component, stack and manifests from the duplicate-definition failure
// the stack processor reports when two top-level manifests resolve to the same stack name.
var collisionPattern = regexp.MustCompile(`(?s)component '([^']+)' in the stack '([^']+)' is defined in more than one top-level stack manifest file:\s*(.+?)\.\n`)

// preflightProject runs the production stack processor over the project's existing manifests together
// with every rendered file that lands under stacks.base_path, before any hook runs or file is written.
// The rendered files are placed in a throwaway copy of the stacks directory, so the project is never
// touched and imports resolve exactly as they will after the files are created.
func preflightProject(config *schema.AtmosConfiguration, files []engine.File, root string) error {
	stacksBase := stacksBasePath(config)
	rendered := renderedStackFiles(stacksBase, root, files)
	if len(rendered) == 0 {
		return nil
	}
	overlay, err := os.MkdirTemp("", "atmos-stack-create-")
	if err != nil {
		return fmt.Errorf(wrapFormat, errUtils.ErrCreateTempDir, err)
	}
	defer os.RemoveAll(overlay)
	overlayStacks := filepath.Join(overlay, "stacks")
	if err := copyTree(stacksBase, overlayStacks, 0); err != nil {
		return err
	}
	for _, file := range rendered {
		target := filepath.Join(overlayStacks, file.relative)
		if err := os.MkdirAll(filepath.Dir(target), overlayDirPermissions); err != nil {
			return fmt.Errorf(wrapFormat, errUtils.ErrCreateDirectory, err)
		}
		if err := os.WriteFile(target, []byte(file.content), overlayFilePermissions); err != nil {
			return fmt.Errorf(wrapFormat, errUtils.ErrFileWrite, err)
		}
	}
	processErr := processStacks(config, overlayStacks)
	if processErr == nil {
		return nil
	}
	// A project whose stacks already fail to process is not caused by this scaffold; say so rather than
	// blaming the new files.
	if baselineErr := processStacks(config, stacksBase); baselineErr != nil && !errors.Is(baselineErr, errUtils.ErrNoStackManifestsFound) {
		return existingProjectError(baselineErr, overlayStacks, stacksBase)
	}
	return preflightError(processErr, rendered, overlayStacks, stacksBase)
}

// renderedStack is a rendered output that lives under stacks.base_path.
type renderedStack struct {
	relative string
	content  string
}

func renderedStackFiles(stacksBase, root string, files []engine.File) []renderedStack {
	var rendered []renderedStack
	for _, file := range files {
		relative, err := filepath.Rel(stacksBase, filepath.Join(root, file.Path))
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		rendered = append(rendered, renderedStack{relative: relative, content: file.Content})
	}
	return rendered
}

func stacksBasePath(config *schema.AtmosConfiguration) string {
	if config.StacksBaseAbsolutePath != "" {
		return config.StacksBaseAbsolutePath
	}
	return filepath.Join(config.BasePathAbsolute, config.Stacks.BasePath)
}

// processStacks resolves every stack under stacksBase through the production code paths: the `describe
// stacks` pipeline (import resolution, deep merging, inheritance and stack naming, with templates and YAML
// functions left unevaluated so nothing runs or is fetched) followed by the duplicate-instance check that
// `describe stacks` and `validate stacks` apply to top-level manifests.
func processStacks(config *schema.AtmosConfiguration, stacksBase string) error {
	scoped := *config
	scoped.StacksBaseAbsolutePath = stacksBase
	include, err := u.JoinPaths(stacksBase, config.Stacks.IncludedPaths)
	if err != nil {
		return err
	}
	exclude, err := u.JoinPaths(stacksBase, config.Stacks.ExcludedPaths)
	if err != nil {
		return err
	}
	scoped.IncludeStackAbsolutePaths, scoped.ExcludeStackAbsolutePaths = include, exclude
	absolute, relative, err := cfg.FindAllStackConfigsInPaths(&scoped, include, exclude)
	if err != nil {
		return err
	}
	scoped.StackConfigFilesAbsolutePaths, scoped.StackConfigFilesRelativePaths = absolute, relative
	if _, err := exec.ExecuteDescribeStacks(&scoped, "", nil, nil, nil, false, false, false, true, nil, nil); err != nil {
		return err
	}
	return checkDuplicateInstances(&scoped, relative)
}

// checkDuplicateInstances reports a component declared by two top-level manifests that resolve to the same
// stack. That check loads manifests statically, so a project that needs template evaluation just to load its
// imports cannot be checked this way; such a failure is not a duplicate and the evaluated pass above already
// accepted the stacks.
func checkDuplicateInstances(scoped *schema.AtmosConfiguration, manifests []string) error {
	err := exec.ValidateStaticStacks(context.Background(), scoped, exec.StaticStackInputs{Files: slashPaths(manifests)})
	if errors.Is(err, errUtils.ErrValidationFailed) {
		return err
	}
	return nil
}

func slashPaths(paths []string) []string {
	converted := make([]string, len(paths))
	for i, path := range paths {
		converted[i] = filepath.ToSlash(path)
	}
	return converted
}

// copyTree copies the regular files under src into dst, following symlinks.
func copyTree(src, dst string, depth int) error {
	if depth > maxSymlinkDepth {
		return nil
	}
	info, err := os.Stat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return os.MkdirAll(dst, overlayDirPermissions)
	}
	if err != nil {
		return fmt.Errorf(wrapFormat, errUtils.ErrReadFile, err)
	}
	if !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(dst, overlayDirPermissions); err != nil {
		return fmt.Errorf(wrapFormat, errUtils.ErrCreateDirectory, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf(wrapFormat, errUtils.ErrReadDirectory, err)
	}
	for _, entry := range entries {
		if err := copyEntry(src, dst, entry, depth); err != nil {
			return err
		}
	}
	return nil
}

func copyEntry(src, dst string, entry fs.DirEntry, depth int) error {
	from, to := filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())
	target, err := os.Stat(from)
	if errors.Is(err, fs.ErrNotExist) {
		// A dangling link cannot be imported either, so the overlay behaves the same way.
		return nil
	}
	if err != nil {
		return fmt.Errorf(wrapFormat, errUtils.ErrReadFile, err)
	}
	if !target.IsDir() {
		return copyFile(from, to)
	}
	if entry.Type()&fs.ModeSymlink != 0 {
		depth++
	}
	return copyTree(from, to, depth)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf(wrapFormat, errUtils.ErrReadFile, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, overlayFilePermissions)
	if err != nil {
		return fmt.Errorf(wrapFormat, errUtils.ErrFileWrite, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf(wrapFormat, errUtils.ErrFileWrite, err)
	}
	return out.Close()
}

// existingProjectError reports a project whose stacks fail independently of the new files.
func existingProjectError(cause error, overlay, real string) error {
	return errUtils.Build(fmt.Errorf("%w: the project's existing stacks already fail to process: %s", errUtils.ErrInvalidStackManifest, sanitize(cause, overlay, real))).
		WithHint("Fix the existing stack manifests first; run `atmos validate stacks` to see every problem").
		Err()
}

// preflightError explains why the new files would break the project and how to avoid it.
func preflightError(cause error, rendered []renderedStack, overlay, real string) error {
	names := make([]string, 0, len(rendered))
	for _, file := range rendered {
		names = append(names, filepath.ToSlash(file.relative))
	}
	text := sanitize(cause, overlay, real)
	files := strings.Join(names, ", ")
	if match := collisionPattern.FindStringSubmatch(text); match != nil {
		return collisionError(match, files)
	}
	builder := errUtils.Build(fmt.Errorf("%w: creating %s would break stack processing: %s", errUtils.ErrInvalidStackManifest, files, text))
	switch {
	case errors.Is(cause, errUtils.ErrStackImportNotFound):
		builder.WithHint("Import paths are relative to `stacks.base_path`; create the imported manifest or correct the `--import` value").
			WithHint("List the manifests that exist with `atmos list stacks` or by browsing `stacks.base_path`")
	case strings.Contains(text, "map has no entry for key") || strings.Contains(text, "name_template"):
		builder.WithHint("Pass `--name`, or set the variables your `stacks.name_template` uses (for example with `--set`), so the new stack gets a resolvable name")
	default:
		builder.WithHint("Fix the scaffold output or its answers; nothing was written")
	}
	return builder.Err()
}

func collisionError(match []string, files string) error {
	component, stack := match[1], match[2]
	// Stack validation lists the manifests in map order; sort them so the message is stable.
	listed := strings.Split(match[3], ", ")
	sort.Strings(listed)
	manifests := strings.Join(listed, ", ")
	return errUtils.Build(fmt.Errorf("%w: creating %s would define stack %q again: component %q is declared in more than one top-level manifest (%s)",
		errUtils.ErrInvalidStackManifest, files, stack, component, manifests)).
		WithHint(fmt.Sprintf("Existing manifests already define stack `%s`; pick values (or `--name`) that give the new stack a unique name", stack)).
		WithHint("To change an existing stack, edit its manifest with `atmos stack config set` instead of creating another").
		Err()
}

// sanitize renders an error with the throwaway overlay path replaced by the project's stacks directory.
func sanitize(err error, overlay, real string) string {
	return strings.ReplaceAll(err.Error(), overlay, real)
}
