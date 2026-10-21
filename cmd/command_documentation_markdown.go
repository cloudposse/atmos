package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	atmoshttp "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

const maxCommandDocumentationBytes = 4 * 1024 * 1024

// commandMarkdownURL mirrors the website's per-page Markdown route convention.
func commandMarkdownURL(documentationURL string) (string, error) {
	parsed, err := url.Parse(documentationURL)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/index"
	}
	if !strings.HasSuffix(path, ".md") {
		path += ".md"
	}
	parsed.Path = path
	return parsed.String(), nil
}

// renderCommandDocumentation prints the source URL and uses the shared Markdown
// renderer, without opening a browser or running the command's execution hooks.
func renderCommandDocumentation(cmd *cobra.Command, documentationURL string) error {
	markdownURL, err := commandMarkdownURL(documentationURL)
	if err != nil {
		return fmt.Errorf("resolve Markdown documentation: %w", err)
	}
	out := cmd.OutOrStdout()
	printDocumentationURL(out, markdownURL)
	progress := spinner.New("Fetching documentation")
	progress.Start()
	content, err := fetchCommandDocumentation(discoveryContext(cmd), markdownURL)
	progress.Stop()
	if err != nil {
		return fmt.Errorf("fetch documentation %s: %w", markdownURL, err)
	}
	ui.New(out).Markdown(content)
	return nil
}

// fetchCommandDocumentation bounds downloads and honors cancellation and the
// shared HTTP client's timeout. HTML error/fallback pages are not Markdown.
func fetchCommandDocumentation(ctx context.Context, markdownURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, markdownURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "text/markdown, text/plain;q=0.9")
	response, err := atmoshttp.NewDefaultClient().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: %s", errUtils.ErrHTTPRequestFailed, response.Status)
	}
	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
		return "", fmt.Errorf("%w: expected Markdown, received HTML", errUtils.ErrDownloadFile)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxCommandDocumentationBytes+1))
	if err != nil {
		return "", err
	}
	if len(content) > maxCommandDocumentationBytes {
		return "", fmt.Errorf("%w: documentation exceeds %d bytes", errUtils.ErrDownloadFile, maxCommandDocumentationBytes)
	}
	return string(content), nil
}
