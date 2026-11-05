package source

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestMetadataCacheIdentityPreservesSourceQuery distinguishes source versions using the original URI
// while keeping credentials and query values out of diagnostics.
func TestMetadataCacheIdentityPreservesSourceQuery(t *testing.T) {
	const redacted = "https://example.test/template.yaml"
	const original = "https://cache-user:original-secret@example.test/template.yaml?version=1&token=original-token"
	const changed = "https://cache-user:changed-secret@example.test/template.yaml?version=2&token=changed-token"
	for _, tt := range []struct {
		name        string
		metadata    workdir.WorkdirMetadata
		uri         string
		wantChanged bool
	}{
		{name: "same original URI", metadata: workdir.WorkdirMetadata{Source: original, SourceURI: redacted}, uri: original},
		{name: "different query", metadata: workdir.WorkdirMetadata{Source: original, SourceURI: redacted}, uri: changed, wantChanged: true},
		{name: "legacy same URI", metadata: workdir.WorkdirMetadata{SourceURI: redacted}, uri: redacted},
		{name: "legacy changed URI", metadata: workdir.WorkdirMetadata{SourceURI: redacted}, uri: changed, wantChanged: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := checkMetadataChanges(&tt.metadata, &schema.VendorComponentSource{Uri: tt.uri})
			assert.Equal(t, tt.wantChanged, got)
			if tt.wantChanged {
				assert.Contains(t, reason, "Source URI changed")
			} else {
				assert.Empty(t, reason)
			}
			for _, secret := range []string{"cache-user", "original-secret", "changed-secret", "original-token", "changed-token", "version=", "token="} {
				assert.NotContains(t, reason, secret)
			}
		})
	}
}
