package provenance

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/merge"
)

// FormatProvenanceComment formats a provenance entry as an inline comment.
func FormatProvenanceComment(entry merge.ProvenanceEntry) string {
	if entry.Column > 0 {
		return fmt.Sprintf("# from: %s:%d:%d", entry.File, entry.Line, entry.Column)
	}
	return fmt.Sprintf("# from: %s:%d", entry.File, entry.Line)
}
