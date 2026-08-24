package dtos

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitRequest(t *testing.T) {
	t.Run("full request serializes correctly", func(t *testing.T) {
		req := CommitRequest{
			Branch: "feature/my-branch",
			Changes: CommitChanges{
				Additions: []CommitFileAddition{
					{Path: "main.tf", Contents: "dGVycmFmb3Jt"},
				},
				Deletions: []CommitFileDeletion{
					{Path: "deprecated.tf"},
				},
			},
			CommitMessage: "terraform fmt",
			Comment:       "Applied formatting",
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "feature/my-branch", parsed["branch"])
		assert.Equal(t, "terraform fmt", parsed["commitMessage"])
		assert.Equal(t, "Applied formatting", parsed["comment"])

		changes := parsed["changes"].(map[string]interface{})
		additions := changes["additions"].([]interface{})
		assert.Len(t, additions, 1)

		firstAddition := additions[0].(map[string]interface{})
		assert.Equal(t, "main.tf", firstAddition["path"])
		assert.Equal(t, "dGVycmFmb3Jt", firstAddition["contents"])

		deletions := changes["deletions"].([]interface{})
		assert.Len(t, deletions, 1)
		assert.Equal(t, "deprecated.tf", deletions[0].(map[string]interface{})["path"])
	})

	t.Run("comment is omitted when empty", func(t *testing.T) {
		req := CommitRequest{
			Branch: "main",
			Changes: CommitChanges{
				Additions: []CommitFileAddition{},
				Deletions: []CommitFileDeletion{},
			},
			CommitMessage: "fix",
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "comment")
	})
}

func TestCommitResponse(t *testing.T) {
	t.Run("successful response deserializes correctly", func(t *testing.T) {
		body := `{
			"success": true,
			"status": 200,
			"data": { "sha": "abc123def456" }
		}`

		var resp CommitResponse
		err := json.Unmarshal([]byte(body), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, 200, resp.Status)
		assert.Equal(t, "abc123def456", resp.Data.SHA)
	})

	t.Run("error response deserializes correctly", func(t *testing.T) {
		body := `{
			"success": false,
			"status": 400,
			"errorMessage": "validation failed",
			"traceId": "trace-123"
		}`

		var resp CommitResponse
		err := json.Unmarshal([]byte(body), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.ErrorMessage)
		assert.Equal(t, "trace-123", resp.TraceID)
	})
}
