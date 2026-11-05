package github

import (
	"strings"
	"unicode/utf8"

	errUtils "github.com/cloudposse/atmos/errors"
)

// GitHub limits issue comments to 65,536 characters. A byte limit is conservative
// for Unicode text and also avoids submitting an oversized encoded body.
const issueCommentMaxBytes = 65536

// limitIssueCommentBody keeps the upsert marker and a visible truncation notice
// ahead of the retained content. The full summary remains untouched at the caller.
func limitIssueCommentBody(body, marker string) (string, error) {
	if len(body) <= issueCommentMaxBytes {
		return body, nil
	}
	const notice = "> [!WARNING]\n> Comment truncated to fit GitHub's size limit. Consult the workflow summary, logs, or artifacts for the full output.\n\n"
	prefix := notice
	if marker != "" {
		prefix = marker + "\n" + prefix
		body = strings.ReplaceAll(body, marker, "")
	}
	budget := issueCommentMaxBytes - len(prefix)
	if budget <= 0 {
		return "", errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithExplanation("The comment marker exceeds GitHub's comment size limit").Err()
	}
	if len(body) > budget {
		body = body[:budget]
		for !utf8.ValidString(body) && len(body) > 0 {
			body = body[:len(body)-1]
		}
	}
	return prefix + body, nil
}
