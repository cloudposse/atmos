// Package includescope resolves local !include and !include.raw paths against a fixed scope
// instead of the process working directory.
//
// The generic stack-manifest include lookup tries a bare path against the current directory
// first. That is the wrong anchor for configuration that must behave identically wherever Atmos
// runs from (workflow manifests and custom command definitions), so those loaders rewrite each
// local include path to an absolute one before the generic include processor sees it:
//
//   - "./x" and "../x" resolve against the directory of the file that contains the tag.
//   - "x/y" (bare) resolves against the Atmos project base path.
//   - Absolute paths and remote sources (URLs, go-getter shorthands) are left as written.
package includescope

import (
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	includeTag    = utils.AtmosYamlFuncInclude
	includeRawTag = utils.AtmosYamlFuncIncludeRaw

	doubleQuote = `"`
	singleQuote = "'"
)

// Scope anchors local include paths.
type Scope struct {
	// File is the file that contains the tag. Dot-prefixed paths resolve against its directory.
	File string
	// BasePath is the absolute Atmos project base path. Bare paths resolve against it.
	BasePath string
}

// Resolved is the outcome of resolving one include argument.
type Resolved struct {
	// Value is the include argument to hand to the generic include processor. For a local file
	// its path is absolute; remote sources and unparsable arguments are returned unchanged.
	Value string
	// LocalPath is the absolute local file the include reads, or empty for remote sources and
	// unparsable arguments.
	LocalPath string
	// HasQuery reports that a YQ expression post-processes the file content.
	HasQuery bool
}

// IsIncludeTag reports whether tag is !include or !include.raw.
func IsIncludeTag(tag string) bool {
	defer perf.Track(nil, "includescope.IsIncludeTag")()

	return tag == includeTag || tag == includeRawTag
}

// Resolve resolves the argument of an !include or !include.raw tag against the scope.
func (s Scope) Resolve(value string) Resolved {
	defer perf.Track(nil, "includescope.Scope.Resolve")()

	parsed, err := parser.ParseInclude(value)
	if err != nil {
		return Resolved{Value: value}
	}
	local, ok := s.localPath(parsed.Path)
	if !ok {
		return Resolved{Value: value}
	}
	resolved := Resolved{Value: quote(local), LocalPath: local, HasQuery: parsed.Query != ""}
	if resolved.HasQuery {
		resolved.Value += " " + queryArgument(parsed.Query)
	}
	return resolved
}

// localPath returns the absolute local path for an include path, or false for a remote source.
func (s Scope) localPath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), true
	}
	if isDotPrefixed(path) {
		return filepath.Clean(filepath.Join(filepath.Dir(s.File), path)), true
	}
	candidate := filepath.Clean(filepath.Join(s.BasePath, path))
	if fileExists(candidate) || !utils.IsRemoteIncludePath(path) {
		return candidate, true
	}
	return "", false
}

// Rewrite makes every local !include and !include.raw argument under root absolute, resolved
// against the scope. Remote sources are left untouched.
func (s Scope) Rewrite(root *yaml.Node) {
	defer perf.Track(nil, "includescope.Scope.Rewrite")()

	if root == nil {
		return
	}
	if root.Kind == yaml.ScalarNode && IsIncludeTag(root.Tag) {
		if resolved := s.Resolve(root.Value); resolved.LocalPath != "" {
			root.Value = resolved.Value
		}
		return
	}
	for _, child := range root.Content {
		s.Rewrite(child)
	}
}

// LocalFile returns the absolute file a script value was read from when node is an
// !include or !include.raw scalar that Rewrite already made absolute and that has no YQ
// expression (a query changes the content, so the file is no longer the script's source).
func LocalFile(node *yaml.Node) (string, bool) {
	defer perf.Track(nil, "includescope.LocalFile")()

	if node == nil || node.Kind != yaml.ScalarNode || !IsIncludeTag(node.Tag) {
		return "", false
	}
	parsed, err := parser.ParseInclude(node.Value)
	if err != nil || parsed.Query != "" || !filepath.IsAbs(parsed.Path) {
		return "", false
	}
	return parsed.Path, true
}

func isDotPrefixed(path string) bool {
	first := path
	if i := strings.IndexAny(path, `/\`); i >= 0 {
		first = path[:i]
	}
	return first == "." || first == ".."
}

// quote wraps value so ParseInclude returns it unchanged, whatever characters it contains.
func quote(value string) string {
	return doubleQuote + strings.ReplaceAll(value, doubleQuote, doubleQuote+doubleQuote) + doubleQuote
}

// queryArgument renders a YQ expression so ParseInclude reads it back unchanged. ParseInclude
// strips one level of quotes from the expression, so one that itself starts with a quote
// character needs quoting again.
func queryArgument(query string) string {
	if strings.HasPrefix(query, doubleQuote) || strings.HasPrefix(query, singleQuote) {
		return singleQuote + strings.ReplaceAll(query, singleQuote, singleQuote+singleQuote) + singleQuote
	}
	return query
}

func fileExists(path string) bool {
	return utils.FileExists(path)
}
