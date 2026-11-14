package artifacts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/artifact"
)

func incompleteListing() *artifact.ListResult {
	result := listing()
	result.Incomplete = true
	result.Repositories = append(result.Repositories, artifact.RepositoryListing{
		Repository:  "shared",
		Diagnostics: []artifact.Diagnostic{{Severity: artifact.SeverityError, Code: "list-failed", Message: "401 unauthorized"}},
	})
	result.Repositories[0].Source, result.Repositories[0].FetchedAt = "cache", "2026-03-04T05:06:07Z"
	return result
}

func TestIncompleteListingIsNeverPresentedAsComplete(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().List(gomock.Any(), gomock.Any()).Return(incompleteListing(), nil).Times(2)

	table := h.run("list")
	require.NoError(t, table.err, "the readable repositories are still listed")
	assert.Contains(t, table.stdout, "team/app")
	assert.Contains(t, table.stderr, "incomplete")
	assert.Contains(t, table.stderr, "shared")
	assert.Contains(t, table.stderr, "401 unauthorized")

	structured := h.run("list", "--format=json")
	require.NoError(t, structured.err)
	envelope := decodeObject(t, structured.stdout)
	assert.Equal(t, true, envelope["incomplete"])
	repositories := envelope["repositories"].([]any)
	first := repositories[0].(map[string]any)
	assert.Equal(t, "cache", first["source"], "the provenance of a listing is part of the result")
	assert.Equal(t, "2026-03-04T05:06:07Z", first["fetched_at"])
}

func TestCompleteListingOmitsTheIncompleteMarker(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().List(gomock.Any(), gomock.Any()).Return(listing(), nil)
	got := h.run("list", "--format=json")
	require.NoError(t, got.err)
	assert.NotContains(t, decodeObject(t, got.stdout), "incomplete")
}
