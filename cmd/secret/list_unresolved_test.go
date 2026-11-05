package secret

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/secrets"
)

// TestStatusRow_UnresolvedSelector proves an unresolved selector is shown as its own state with a
// reason, rather than as a bare "error".
func TestStatusRow_UnresolvedSelector(t *testing.T) {
	const (
		selector      = "!aws.cloudformation.output producer dev StoreName"
		unresolvedWhy = "backend selector resolved with --verify (requires credentials)"
	)
	tests := []struct {
		name       string
		status     secrets.Status
		wantStatus string
		wantReason string
	}{
		{
			name: "unresolved selector",
			status: secrets.Status{
				Declaration: secrets.Declaration{Name: "API_KEY", BackendType: secrets.BackendStore, BackendName: selector},
				Unknown:     true, Unresolved: true,
				Reason: unresolvedWhy,
			},
			wantStatus: "unresolved",
			wantReason: unresolvedWhy,
		},
		{
			name: "error carries its reason",
			status: secrets.Status{
				Declaration: secrets.Declaration{Name: "API_KEY"},
				Err:         errors.New("access denied"), Reason: "access denied",
			},
			wantStatus: "error",
			wantReason: "access denied",
		},
		{
			name:       "plain unknown has no reason",
			status:     secrets.Status{Declaration: secrets.Declaration{Name: "API_KEY"}, Unknown: true},
			wantStatus: "unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := statusRow("dev", "api", &tt.status)
			assert.Equal(t, tt.wantStatus, row["status"])
			assert.Equal(t, tt.wantReason, row["reason"])
			assert.Equal(t, tt.wantReason != "", rowsHaveReason([]map[string]any{row}))
		})
	}

	// The provider column keeps the raw selector so the user sees what is unresolved.
	row := statusRow("dev", "api", &secrets.Status{Declaration: secrets.Declaration{Name: "K", BackendType: secrets.BackendStore, BackendName: selector}, Unresolved: true})
	assert.Equal(t, "store:"+selector, row["provider"])
}

func TestRenderSecretRows_ReasonColumnOnlyWhenNeeded(t *testing.T) {
	t.Run("reason present", func(t *testing.T) {
		stdout, _ := setupIOCapture(t)
		rows := statusesToData("dev", "api", []secrets.Status{
			{Declaration: secrets.Declaration{Name: "A"}, Unknown: true, Unresolved: true, Reason: "needs --verify"},
		})
		require.NoError(t, renderSecretRows(rows, false, "json", "none"))
		assert.Contains(t, stdout.String(), "needs --verify")
		assert.Contains(t, stdout.String(), "unresolved")
	})

	t.Run("no reason", func(t *testing.T) {
		stdout, _ := setupIOCapture(t)
		rows := statusesToData("dev", "api", []secrets.Status{{Declaration: secrets.Declaration{Name: "A"}, Initialized: true}})
		require.NoError(t, renderSecretRows(rows, false, "json", "none"))
		assert.NotContains(t, stdout.String(), "Reason")
	})
}
