package function

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	atmoshttp "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Include function tags are defined in tags.go.
// Use YAMLTag(TagInclude) and YAMLTag(TagIncludeRaw) to get the YAML tag format.

// Default HTTP timeout for URL fetches.
const defaultHTTPTimeout = 30 * time.Second

// IncludeFunction implements the !include YAML function.
// It includes content from local files or remote URLs.
type IncludeFunction struct {
	BaseFunction
}

// NewIncludeFunction creates a new IncludeFunction.
func NewIncludeFunction() *IncludeFunction {
	defer perf.Track(nil, "function.NewIncludeFunction")()

	return &IncludeFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "include",
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the !include function.
// Syntax: !include path/to/file.yaml
// Syntax: !include https://example.com/file.yaml
func (f *IncludeFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.IncludeFunction.Execute")()

	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("%w: !include requires a file path or URL", ErrInvalidArguments)
	}

	return loadIncludeContent(ctx, args, execCtx, false)
}

// IncludeRawFunction implements the !include.raw YAML function.
// It includes content from local files or remote URLs as raw text.
type IncludeRawFunction struct {
	BaseFunction
}

// NewIncludeRawFunction creates a new IncludeRawFunction.
func NewIncludeRawFunction() *IncludeRawFunction {
	defer perf.Track(nil, "function.NewIncludeRawFunction")()

	return &IncludeRawFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "include.raw",
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the !include.raw function.
// Syntax: !include.raw path/to/file.txt
// Syntax: !include.raw https://example.com/file.txt
func (f *IncludeRawFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.IncludeRawFunction.Execute")()

	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("%w: !include.raw requires a file path or URL", ErrInvalidArguments)
	}

	return loadIncludeContent(ctx, args, execCtx, true)
}

// loadIncludeContent loads content from a file path or URL.
func loadIncludeContent(ctx context.Context, path string, execCtx *ExecutionContext, raw bool) (any, error) {
	defer perf.Track(nil, "function.loadIncludeContent")()

	var content []byte
	var err error

	if isURL(path) {
		content, err = fetchURL(ctx, path)
	} else {
		content, err = readLocalFile(path, execCtx)
	}

	if err != nil {
		return nil, err
	}

	// For raw mode, return as string.
	if raw {
		return string(content), nil
	}

	// For regular include, return the content as string.
	// The actual YAML parsing will be done by the caller if needed.
	return string(content), nil
}

// isURL checks if a path is a URL.
func isURL(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

// fetchURL fetches content from a URL using the atmos HTTP client.
func fetchURL(ctx context.Context, url string) ([]byte, error) {
	defer perf.Track(nil, "function.fetchURL")()

	client := atmoshttp.NewDefaultClient(atmoshttp.WithTimeout(defaultHTTPTimeout))
	content, err := atmoshttp.Get(ctx, url, client)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch URL %s: %w", ErrInvalidArguments, url, err)
	}

	return content, nil
}

// readLocalFile reads content from a local file.
func readLocalFile(path string, execCtx *ExecutionContext) ([]byte, error) {
	defer perf.Track(nil, "function.readLocalFile")()

	// If path is relative, resolve it relative to the source file.
	if !filepath.IsAbs(path) && execCtx != nil && execCtx.SourceFile != "" {
		dir := filepath.Dir(execCtx.SourceFile)
		path = filepath.Join(dir, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read file %s: %w", ErrInvalidArguments, path, err)
	}

	return content, nil
}
