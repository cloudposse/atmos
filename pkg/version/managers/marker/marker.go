// Package marker implements the marker file manager: the Renovate
// regex-manager equivalent for arbitrary text files. A comment annotation
//
//	<comment> atmos:version <entry-name> [match=<regex-with-one-capture-group>]
//
// marks a line (trailing comment) or the next non-blank, non-comment line
// (standalone comment) as carrying a managed version. The manager rewrites the
// version token in place from the lock. Comment delimiters are detected per
// language (#, //, ;, --, <!--, /*), so the annotation works across YAML,
// shell, Dockerfiles, Go, SQL, HTML, and more. Formats without comments (JSON)
// should use the template manager instead.
package marker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
	"github.com/cloudposse/atmos/pkg/version/managers"
)

// Name is the manager's registry name.
const Name = "marker"

var (
	// ErrBadMatchExpression is returned for a match= regex that does not
	// compile or lacks a capture group.
	ErrBadMatchExpression = errUtils.ErrVersionMarkerBadMatch

	// Extracts the entry name and optional match override from an annotation.
	markerPattern = regexp.MustCompile(`atmos:version\s+([A-Za-z0-9_-]+)(?:\s+match=(\S+))?`)

	// Default token replaced in the target line.
	versionTokenPattern = regexp.MustCompile(`\bv?\d+(\.\d+){1,3}(-[0-9A-Za-z.+-]+)?\b`)

	// Immutable identifiers (OCI digests and git SHAs) replaced for pinned
	// entries.
	digestTokenPattern = regexp.MustCompile(`sha256:[0-9a-f]{64}|\b[0-9a-f]{40}\b`)

	// Comment openers searched for annotations, ordered longest first so
	// "<!--" wins over "--".
	commentDelimiters = []string{"<!--", "//", "/*", "--", "#", ";"}
)

// Manager rewrites marker-annotated version tokens from the lock.
type Manager struct{}

// Name returns the manager's registry name.
func (Manager) Name() string {
	defer perf.Track(nil, "marker.Manager.Name")()

	return Name
}

// DefaultPaths is empty: the marker manager only runs over configured paths.
func (Manager) DefaultPaths() []string {
	defer perf.Track(nil, "marker.Manager.DefaultPaths")()

	return nil
}

// Plan scans the configured files for annotations and returns the rewrites
// needed to match the locked versions.
func (Manager) Plan(ctx context.Context, in *managers.Input) ([]managers.FileChange, error) {
	defer perf.Track(in.Config, "marker.Manager.Plan")()

	if len(in.Paths) == 0 {
		return nil, nil
	}
	files, err := managers.ExpandPaths(in.Dir, in.Paths)
	if err != nil {
		return nil, err
	}
	var changes []managers.FileChange
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		updated, err := rewriteMarkers(content, in.Refs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if !bytes.Equal(content, updated) {
			changes = append(changes, managers.FileChange{Path: file, Old: content, New: updated})
		}
	}
	return changes, nil
}

// annotation is one parsed atmos:version marker.
type annotation struct {
	name  string
	match string
	// commentStart is the index of the comment delimiter on the marker line
	// (-1 when the whole line is the comment).
	commentStart int
}

// rewriteMarkers rewrites every annotated version token in a document.
func rewriteMarkers(content []byte, refs map[string]manager.VersionRef) ([]byte, error) {
	lines := strings.Split(string(content), "\n")
	for i := 0; i < len(lines); i++ {
		note := parseAnnotation(lines[i])
		if note == nil {
			continue
		}
		ref, ok := refs[note.name]
		if !ok || ref.Version == "" {
			continue
		}
		if note.commentStart > 0 {
			// Trailing comment: rewrite the code portion of this line.
			code := lines[i][:note.commentStart]
			updated, err := replaceToken(code, note.match, &ref)
			if err != nil {
				return nil, err
			}
			lines[i] = updated + lines[i][note.commentStart:]
			continue
		}
		// Standalone comment: rewrite the next non-blank, non-comment line.
		target := nextTargetLine(lines, i+1)
		if target < 0 {
			continue
		}
		updated, err := replaceToken(lines[target], note.match, &ref)
		if err != nil {
			return nil, err
		}
		lines[target] = updated
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// parseAnnotation extracts an atmos:version annotation from a line, deciding
// whether it is a trailing or standalone comment.
func parseAnnotation(line string) *annotation {
	markerIndex := strings.Index(line, "atmos:version")
	if markerIndex < 0 {
		return nil
	}
	match := markerPattern.FindStringSubmatch(line[markerIndex:])
	if match == nil {
		return nil
	}
	note := &annotation{name: match[1], match: match[2], commentStart: -1}
	// Locate the comment delimiter introducing the annotation: the rightmost
	// delimiter before the marker text.
	for _, delimiter := range commentDelimiters {
		idx := strings.LastIndex(line[:markerIndex], delimiter)
		if idx < 0 || idx <= note.commentStart {
			continue
		}
		if nestedInChosenDelimiter(line, note.commentStart, idx) {
			continue
		}
		note.commentStart = idx
	}
	if note.commentStart >= 0 && strings.TrimSpace(line[:note.commentStart]) == "" {
		note.commentStart = -1 // Comment-only line: standalone annotation.
	}
	return note
}

func nestedInChosenDelimiter(line string, chosenStart, candidateStart int) bool {
	if chosenStart < 0 {
		return false
	}
	for _, delimiter := range commentDelimiters {
		if strings.HasPrefix(line[chosenStart:], delimiter) {
			return candidateStart < chosenStart+len(delimiter)
		}
	}
	return false
}

// nextTargetLine returns the index of the next non-blank, non-comment line.
func nextTargetLine(lines []string, from int) int {
	for i := from; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if isCommentLine(trimmed) {
			continue
		}
		return i
	}
	return -1
}

// isCommentLine reports whether a trimmed line starts with a comment delimiter.
func isCommentLine(trimmed string) bool {
	for _, delimiter := range commentDelimiters {
		if strings.HasPrefix(trimmed, delimiter) {
			return true
		}
	}
	return false
}

// replaceToken rewrites the managed token in a line. A match= override
// replaces its first capture group; otherwise pinned entries replace digest
// tokens (sha256 or 40-hex identifiers) when present, and the first
// version-shaped token is replaced with the version.
func replaceToken(line, matchExpr string, ref *manager.VersionRef) (string, error) {
	if matchExpr != "" {
		return replaceWithExpression(line, matchExpr, ref)
	}
	pinned := ref.Pin == manager.PinDigest && ref.Digest != ""
	if pinned {
		if location := digestTokenPattern.FindStringIndex(line); location != nil {
			return line[:location[0]] + ref.Digest + line[location[1]:], nil
		}
	}
	if location := versionTokenPattern.FindStringIndex(line); location != nil {
		return line[:location[0]] + ref.Version + line[location[1]:], nil
	}
	return line, nil
}

// replaceWithExpression replaces the first capture group of a custom match
// expression with the managed reference (digest when pinned, else version).
func replaceWithExpression(line, matchExpr string, ref *manager.VersionRef) (string, error) {
	expression, err := regexp.Compile(matchExpr)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", ErrBadMatchExpression, matchExpr, err)
	}
	groups := expression.FindStringSubmatchIndex(line)
	const groupIndexStart = 2 // Submatch index pairs start after the full match.
	if groups == nil || len(groups) < groupIndexStart*2 || groups[groupIndexStart] < 0 {
		return line, nil
	}
	replacement := ref.Version
	if ref.Pin == manager.PinDigest && ref.Digest != "" {
		replacement = ref.Digest
	}
	return line[:groups[groupIndexStart]] + replacement + line[groups[groupIndexStart+1]:], nil
}

func init() {
	managers.Register(Manager{})
}
