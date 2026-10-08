package imagesummary

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// MaxCommentChars is the length at which a comment body is truncated. GitHub rejects bodies over
	// 65,536 characters, and Atmos adds a hidden marker to the body, so the cut leaves headroom.
	MaxCommentChars = 65000

	// The truncationNote constant is appended to a truncated comment.
	truncationNote = "\n\n_Comment truncated; see the job summary for the full report._\n"

	// The codeFence constant is the Markdown fence that must be balanced in a truncated body.
	codeFence = "```"

	// The detailsOpen and detailsClose constants are the HTML tags of a collapsible section.
	detailsOpen  = "<details>"
	detailsClose = "</details>"

	// The closeAllowance constant is the number of unclosed collapsible sections TruncateComment closes.
	closeAllowance = 4
)

// TruncateComment shortens a comment body to at most MaxCommentChars characters and appends a note
// that points to the job summary. A body within the limit is returned unchanged. A code fence or
// collapsible section left open by the cut is closed so the note does not render inside it.
func TruncateComment(body string) string {
	defer perf.Track(nil, "imagesummary.TruncateComment")()

	runes := []rune(body)
	if len(runes) <= MaxCommentChars {
		return body
	}
	// Reserve room for the note and for the closing fence and sections so the result stays within the limit.
	reserved := len([]rune(truncationNote)) + len(codeFence) + 1 + closeAllowance*(len(detailsClose)+1)
	kept := string(runes[:MaxCommentChars-reserved])
	if strings.Count(kept, codeFence)%2 == 1 {
		kept += "\n" + codeFence
	}
	unclosed := strings.Count(kept, detailsOpen) - strings.Count(kept, detailsClose)
	for i := 0; i < unclosed && i < closeAllowance; i++ {
		kept += "\n" + detailsClose
	}
	return kept + truncationNote
}
