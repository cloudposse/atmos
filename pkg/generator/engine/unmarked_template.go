package engine

import (
	"regexp"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// templateDataRoots lists the top-level names scaffold templates read their data from (see
// ProcessTemplateWithDelimiters). An action that starts with one of them, such as
// `{{ .Config.environment | quote }}`, can only be meant for Atmos. Matching on these roots,
// rather than on any `{{ .`, keeps files that carry another tool's templates (for example a Helm
// chart's `{{ .Values.image }}`) from being flagged.
const templateDataRoots = `(?:Config|TemplateName|TemplateDescription|ScaffoldPath|matrix|file)`

// unmarkedTemplateHint is the remedy shared by the warning and the error.
const unmarkedTemplateHint = "add a `# atmos:template` first line or a .tmpl suffix"

// templateActionRegexp returns the pattern matching a template action that reads scaffold data
// under the given delimiters. Falls back to the default delimiters unless exactly two are given.
func templateActionRegexp(delimiters []string) *regexp.Regexp {
	if len(delimiters) != 2 {
		delimiters = []string{defaultLeftDelimiter, defaultRightDelimiter}
	}
	return regexp.MustCompile(regexp.QuoteMeta(delimiters[0]) + `-?\s*\.` + templateDataRoots + `\b`)
}

// ContainsTemplateActions reports whether content holds an action that reads scaffold data
// (such as `{{ .Config.name }}`) under the given delimiters. Content that was rendered as a
// template never does, so a hit on a file's final bytes means it was copied without being
// rendered: it contains template syntax but was not marked as a template.
func ContainsTemplateActions(content string, delimiters []string) bool {
	defer perf.Track(nil, "engine.ContainsTemplateActions")()

	return templateActionRegexp(delimiters).MatchString(content)
}

// UnmarkedTemplateWarning returns the user-facing warning for a file that is copied as-is
// although it holds template actions, or "" when it holds none. The sourcePath argument is the
// template file to fix, and destPath is where it was written (omitted from the message when empty
// or equal to sourcePath).
func UnmarkedTemplateWarning(sourcePath, destPath, content string, delimiters []string) string {
	defer perf.Track(nil, "engine.UnmarkedTemplateWarning")()

	if !ContainsTemplateActions(content, delimiters) {
		return ""
	}
	subject := "`" + sourcePath + "`"
	if destPath != "" && destPath != sourcePath {
		subject += " (copied to `" + destPath + "`)"
	}
	return subject + " contains template syntax but is not marked as a template; " + unmarkedTemplateHint
}

// ExplainUnmarkedTemplate decorates a failure to parse or validate a generated file (cause) when
// the likely reason is a template the author forgot to mark: the file's final content still holds
// template actions. The returned error names the file and the missing marker and wraps cause. When
// the content holds no such actions, cause is returned unchanged. Callers that parse generated
// files, such as `atmos stack create` validating its manifest, call it on their parse error.
func ExplainUnmarkedTemplate(path, content string, delimiters []string, cause error) error {
	defer perf.Track(nil, "engine.ExplainUnmarkedTemplate")()

	if cause == nil || !ContainsTemplateActions(content, delimiters) {
		return cause
	}
	return errUtils.Build(errUtils.ErrUnprocessedTemplate).
		WithCause(cause).
		WithExplanationf("Generated file `%s` contains template syntax but is not marked as a template", path).
		WithHint("In the scaffold, "+unmarkedTemplateHint).
		WithHint("Templated files are the ones ending in `.tmpl` or starting with an `atmos:template` comment").
		WithContext("file_path", path).
		WithExitCode(2).
		Err()
}
