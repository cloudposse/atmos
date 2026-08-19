package exec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCaptureExecMetadataSync_FlagsReflectRealInvocation is a regression test
// for a production bug: an `atmos terraform plan cdn -s plat-use2-dev
// --upload-status` invocation uploaded an execution record whose `flags`
// field was empty, even though real flags were passed on the command line.
//
// Root cause: captureExecMetadataSync (internal/exec/terraform.go) used to
// report info.AdditionalArgsAndFlags as the record's Flags. That field is NOT
// "the CLI flags the user passed to atmos" — per cli_utils.go, it holds only
// the pass-through args left over after Cobra has already parsed and
// consumed recognized atmos flags like `-s`/`--stack`. So `-s plat-use2-dev`
// was never in that slice to begin with, and the one atmos-recognized flag
// that IS threaded through it, `--upload-status`, is explicitly stripped out
// in buildPlanSubcommandArgs (terraform_execute_helpers_args.go) before
// capture runs. The fix: Flags is now sourced from the invoking
// *cobra.Command's own record of explicitly-set flags
// (proexec.FlagsFromCommand), matching the async path.
func TestCaptureExecMetadataSync_FlagsReflectRealInvocation(t *testing.T) {
	t.Setenv("CI", "true")

	var mu sync.Mutex
	var received []dtos.ExecUploadRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/atmos/exec") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var req dtos.ExecUploadRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		mu.Lock()
		received = append(received, req)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.BaseURL = server.URL
	atmosConfig.Settings.Pro.Token = "test-token"
	// A short-lived Pro exec sync timeout keeps this test fast if the upload
	// path ever regresses to hanging instead of completing.
	atmosConfig.Settings.Pro.Exec.SyncTimeoutSeconds = 2

	// Mirrors the real invocation `atmos terraform plan cdn -s plat-use2-dev
	// --upload-status`: the planCmd's own flags are set exactly as the user
	// would type them, and AdditionalArgsAndFlags is empty (as it would be
	// after buildPlanSubcommandArgs strips --upload-status), proving Flags no
	// longer depends on that field at all.
	plan := &cobra.Command{Use: "plan"}
	plan.Flags().StringP("stack", "s", "", "stack")
	plan.Flags().Bool("upload-status", false, "upload status")
	require.NoError(t, plan.Flags().Parse([]string{"-s", "plat-use2-dev", "--upload-status"}))

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:       "cdn",
		Stack:                  "plat-use2-dev",
		AdditionalArgsAndFlags: []string{},
	}

	captureExecMetadataSync(atmosConfig, "plan", info, plan, nil)

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, received, 1, "expected exactly one exec-metadata upload for a single invocation")
	assert.ElementsMatch(t, []string{"--stack", "plat-use2-dev", "--upload-status"}, received[0].Flags,
		"execution record's Flags must reflect the real CLI flags used, as bare tokens with no synthesized value for the bool flag")
}
