package release

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
)

// Bump this version when rendering or summary instructions change in a way
// that should refresh existing previews even when their source PRs are unchanged.
const previewCachePrefix = "\n<!-- atmos-release-preview-cache:v1 "

type previewCachePR struct {
	releasenotes.PREntry
	State  string
	Merged bool
}

// previewInputFingerprint covers all rendered inputs and summary settings, but
// never includes credentials. Sorted entries make GitHub's response order irrelevant.
func (c *milestoneClient) previewInputFingerprint(header, state string, entries []releasenotes.PREntry, pulls map[int]*releasePull) string {
	input := struct {
		Header, State, Model string
		AI                   bool
		PRs                  []previewCachePR
	}{Header: header, State: state, AI: len(entries) > 0 && c.summaryParams.OpenAIAPIKey != ""}
	if input.AI {
		input.Model = c.summaryParams.Model
	}
	for _, entry := range entries {
		pr := pulls[entry.Number]
		input.PRs = append(input.PRs, previewCachePR{PREntry: entry, State: pr.State, Merged: pr.Merged})
	}
	// These concrete fields contain only JSON-supported values.
	encoded, _ := json.Marshal(input)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

// cachePreviewBody records both the input fingerprint and a checksum of the
// rendered body, so a partial or manually edited preview is regenerated.
func cachePreviewBody(body, fingerprint string) string {
	return body + fmt.Sprintf("%sinputs=%s body=%x -->\n", previewCachePrefix, fingerprint, sha256.Sum256([]byte(body)))
}

// cachedPreviewBody accepts only the already ownership-checked preview and
// requires an intact body with matching inputs; the marker grants no authority.
func cachedPreviewBody(existing *previewIssue, fingerprint string) (string, bool) {
	if existing == nil || len(existing.Body) > previewBodyLimit {
		return "", false
	}
	start := strings.LastIndex(existing.Body, previewCachePrefix)
	if start < 0 {
		return "", false
	}
	if cachePreviewBody(existing.Body[:start], fingerprint) != existing.Body {
		return "", false
	}
	return existing.Body, true
}
