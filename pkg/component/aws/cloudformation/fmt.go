package cloudformation

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// yamlIndent is the indentation width formatTemplate re-emits with, matching
// this codebase's general YAML formatting convention.
const yamlIndent = 2

// templateFilePermissions matches the mode template files are typically
// authored with.
const templateFilePermissions = 0o644

// formatTemplate re-serializes a CloudFormation template with consistent
// indentation, preserving comments and key order via yaml.v3's Node API.
// Plain gopkg.in/yaml.v3 (not the Atmos custom-tag-aware u.UnmarshalYAML) is
// used deliberately, matching parameters.go's registerNoEchoValues. CFN's
// short-form intrinsic function tags (such as Ref, Sub, and GetAtt) would
// otherwise be rejected as unknown Atmos custom tags. There is no cfn-format
// binary to shell out to (and Rain's own formatter is archived along with the
// rest of Rain — see the PRD's migration notes), so this is a native,
// dependency-free round-trip rather than a wrapped external tool.
func formatTemplate(body string) (string, error) {
	defer perf.Track(nil, "cloudformation.formatTemplate")()

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}

	return buf.String(), nil
}

// forceBlockStyle recursively resets every node's Style to the encoder's
// default heuristics. A plain yaml.Node round-trip preserves each node's
// original style, so content that started as JSON (CloudFormation's common
// stored representation for a deployed template, regardless of how it was
// originally authored) survives formatTemplate's re-indentation still
// wrapped in JSON's flow notation (`{...}`/`[...]`) and double-quoted keys
// instead of rendering as normal, idiomatic YAML. Only Style controls
// flow/quoting presentation; Tag (which carries intrinsic function short
// forms like !Ref/!Sub/!GetAtt) is untouched.
func forceBlockStyle(node *yaml.Node) {
	if node == nil {
		return
	}
	node.Style = 0
	for _, child := range node.Content {
		forceBlockStyle(child)
	}
}

// formatTemplateAsBlockYAML re-serializes a template body as pretty-printed,
// block-style YAML regardless of whether CloudFormation returned it as JSON
// or YAML — for read-only display (e.g. `get template`), where there's no
// locally-authored file/style to preserve. This differs from formatTemplate,
// which deliberately preserves a local file's existing style/comments for
// `aws cfn fmt`.
func formatTemplateAsBlockYAML(body string) (string, error) {
	defer perf.Track(nil, "cloudformation.formatTemplateAsBlockYAML")()

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return "", fmt.Errorf(wrapFmt, errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}
	forceBlockStyle(&doc)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf(wrapFmt, errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf(wrapFmt, errUtils.ErrInvalidAwsCloudFormationSettings, err)
	}

	return buf.String(), nil
}

// fmtSkipInlineKey is private execution context, never a user-facing flag.
const fmtSkipInlineKey = "cloudformation-fmt-skip-inline"

// fmtBulkStateKey carries the shared *fmtBulkState through a bulk fmt run's flags. It is private
// execution context, never a user-facing flag.
const fmtBulkStateKey = "cloudformation-fmt-bulk-state"

// fmtBulkState is the state one bulk `fmt` run (--all, --affected, --tags, --labels) shares
// across its components: the templates already handled (components commonly share one template
// file, which must be checked once, not once per component) and the templates `--check` found
// unformatted. Collecting them lets a --check run report every unformatted file and fail once at
// the end, instead of stopping at the first.
type fmtBulkState struct {
	mu          sync.Mutex
	seen        map[string]bool
	unformatted []string
}

// newFmtBulkState returns an empty bulk fmt state.
func newFmtBulkState() *fmtBulkState {
	return &fmtBulkState{seen: make(map[string]bool)}
}

// attachFmtBulkState registers a fresh bulk state in flags and returns it.
func attachFmtBulkState(flags map[string]any) *fmtBulkState {
	state := newFmtBulkState()
	flags[fmtBulkStateKey] = state
	return state
}

// fmtBulkStateFrom returns the bulk state carried by flags, or nil outside a bulk run.
func fmtBulkStateFrom(flags map[string]any) *fmtBulkState {
	state, _ := flags[fmtBulkStateKey].(*fmtBulkState)
	return state
}

// claim reports whether path is seen for the first time, marking it handled. A template is
// identified by its resolved file path, so two components pointing at the same file share one
// check; symlinked spellings of one file resolve to the same entry.
func (s *fmtBulkState) claim(path string) bool {
	key := resolvedTemplateKey(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[key] {
		return false
	}
	s.seen[key] = true
	return true
}

// recordUnformatted notes a template that --check found unformatted.
func (s *fmtBulkState) recordUnformatted(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unformatted = append(s.unformatted, path)
}

// resolvedTemplateKey normalizes a template path for de-duplication: cleaned, with symlinks
// resolved when the file exists (falling back to the cleaned path otherwise).
func resolvedTemplateKey(path string) string {
	cleaned := filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return resolved
	}
	return cleaned
}

// finishFmtBulk turns the outcome of a bulk fmt run into its final error. A failure from the
// graph itself (for example a malformed template) is returned unchanged; otherwise, when --check
// found unformatted templates, it returns one ErrAwsCloudFormationFmtNotClean error that lists
// every one of them, so CI fails after reporting them all. Outside a bulk run (no state in
// flags) it returns graphErr unchanged.
func finishFmtBulk(flags map[string]any, graphErr error) error {
	defer perf.Track(nil, "cloudformation.finishFmtBulk")()

	state := fmtBulkStateFrom(flags)
	if graphErr != nil || state == nil {
		return graphErr
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.unformatted) == 0 {
		return nil
	}
	paths := append([]string(nil), state.unformatted...)
	sort.Strings(paths)
	return fmt.Errorf("%w: %d template(s): %s", errUtils.ErrAwsCloudFormationFmtNotClean, len(paths), strings.Join(paths, ", "))
}

// runFmt formats the component's local template. With --check, reports
// whether the file is already formatted (via ErrAwsCloudFormationFmtNotClean,
// for CI) without writing; otherwise formats in place. In a bulk run it also
// skips templates a sibling component already handled and, under --check,
// records unformatted templates instead of failing, so every one is reported.
func runFmt(spec *stackSpec, flags map[string]any, summary map[string]any) (map[string]any, error) {
	if spec.TemplateAbsPath == "" {
		if skip, _ := flags[fmtSkipInlineKey].(bool); skip {
			ui.Warning(fmt.Sprintf("%s: skipped (%v)", spec.StackName, errUtils.ErrAwsCloudFormationFmtRequiresPath))
			summary["skipped"] = true
			return summary, nil
		}
		return summary, errUtils.ErrAwsCloudFormationFmtRequiresPath
	}

	bulk := fmtBulkStateFrom(flags)
	if bulk != nil && !bulk.claim(spec.TemplateAbsPath) {
		// Another component already handled this template file in this run.
		summary["skipped"] = true
		return summary, nil
	}

	formatted, err := formatTemplate(spec.TemplateBody)
	if err != nil {
		return summary, err
	}

	clean := formatted == spec.TemplateBody
	summary["formatted"] = !clean

	check, _ := flags["check"].(bool)
	if check {
		return summary, reportFmtCheck(spec.TemplateAbsPath, clean, bulk)
	}

	if clean {
		return summary, nil
	}
	if err := writeTemplateAtomic(spec.TemplateAbsPath, formatted); err != nil {
		return summary, fmt.Errorf("%w: %s: %w", errUtils.ErrAwsCloudFormationFmtWriteFailed, spec.TemplateAbsPath, err)
	}
	_ = data.Writeln(fmt.Sprintf("%s: formatted", spec.TemplateAbsPath))
	return summary, nil
}

// reportFmtCheck reports one template's --check result. A clean template prints
// "already formatted" (never plain "formatted", which reads like a write happened). An
// unformatted one prints "not formatted" and, outside a bulk run, fails with
// ErrAwsCloudFormationFmtNotClean; inside a bulk run it is recorded for finishFmtBulk so the
// remaining templates are still checked.
func reportFmtCheck(path string, clean bool, bulk *fmtBulkState) error {
	if clean {
		_ = data.Writeln(fmt.Sprintf("%s: already formatted", path))
		return nil
	}
	_ = data.Writeln(fmt.Sprintf("%s: not formatted", path))
	if bulk != nil {
		bulk.recordUnformatted(path)
		return nil
	}
	return fmt.Errorf("%w: %s", errUtils.ErrAwsCloudFormationFmtNotClean, path)
}

// writeTemplateAtomic writes the formatted template to a temp file in the same directory and
// renames it over TemplateAbsPath (via pkg/filesystem's WriteFileAtomic), instead of truncating
// the existing file in place with os.WriteFile. A disk-full or I/O error mid-write then leaves
// the original template untouched rather than empty or partially overwritten. The original
// file's mode is preserved when it can be read; templateFilePermissions is used as a fallback
// (e.g. the file was somehow removed between load and format).
func writeTemplateAtomic(path, formatted string) error {
	defer perf.Track(nil, "cloudformation.writeTemplateAtomic")()

	mode := os.FileMode(templateFilePermissions)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return filesystem.NewOSFileSystem().WriteFileAtomic(path, []byte(formatted), mode)
}
