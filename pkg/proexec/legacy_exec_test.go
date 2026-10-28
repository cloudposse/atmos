package proexec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	proexec "github.com/cloudposse/atmos/pkg/pro/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLegacyCommandClassification(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{path: "atmos terraform plan", want: true},
		{path: "atmos terraform apply", want: true},
		{path: "atmos terraform deploy", want: true},
		{path: "atmos describe affected", want: true},
		{path: "atmos version"},
		{path: "terraform plan"},
		{path: ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			assert.Equal(t, tc.want, IsSyncCommand(tc.path))
			assert.Equal(t, proexec.IsSyncCommand(tc.path), IsSyncCommand(tc.path))
		})
	}
}

func TestLegacyGateAndVersionedData(t *testing.T) {
	t.Setenv("CI", "true")
	configured := &schema.AtmosConfiguration{}
	configured.Settings.Pro.Token = "test-token"
	for _, tc := range []struct {
		name   string
		config *schema.AtmosConfiguration
		want   bool
	}{
		{name: "nil configuration"},
		{name: "missing credentials", config: &schema.AtmosConfiguration{}},
		{name: "configured CI", config: configured, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, GateOpen(tc.config))
			assert.Equal(t, proexec.GateOpen(tc.config), GateOpen(tc.config))
		})
	}
	for _, tc := range []struct {
		name    string
		version int
		payload any
	}{
		{name: "nil payload"},
		{name: "structured payload", version: 2, payload: map[string]any{"changed": 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, proexec.VersionedData(tc.version, "plan", tc.payload), VersionedData(tc.version, "plan", tc.payload))
		})
	}
}

func TestLegacyFlagsFromCommand(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{name: "unchanged defaults"},
		{name: "bool flag", args: []string{"--upload-status"}, want: []string{"--upload-status"}},
		{name: "string and false boolean", args: []string{"--stack=prod", "--upload-status=false"}, want: []string{"--stack", "prod", "--upload-status"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "plan"}
			cmd.Flags().String("stack", "dev", "stack")
			cmd.Flags().Bool("upload-status", false, "upload status")
			require.NoError(t, cmd.Flags().Parse(tc.args))
			assert.Equal(t, tc.want, FlagsFromCommand(cmd))
			assert.Equal(t, proexec.FlagsFromCommand(cmd), FlagsFromCommand(cmd))
		})
	}
	assert.Nil(t, FlagsFromCommand(nil))
}

func TestLegacyInvocationStateAndReporting(t *testing.T) {
	for _, tc := range []struct {
		name  string
		begin func() (string, func())
	}{
		{name: "legacy begins", begin: BeginInvocation},
		{name: "owner begins", begin: proexec.BeginInvocation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := proexec.ExecutionID()
			id, restore := tc.begin()
			t.Cleanup(restore)
			require.NotEmpty(t, id)
			assert.Equal(t, id, ExecutionID())
			assert.Equal(t, id, proexec.ExecutionID())
			nested, restoreNested := proexec.BeginInvocation()
			assert.NotEqual(t, id, nested)
			assert.Equal(t, nested, ExecutionID())
			restoreNested()
			assert.Equal(t, id, ExecutionID())

			info := schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "prod"}
			cause := errUtils.ExitCodeError{Code: 42}
			var legacyErr, ownerErr error = cause, cause
			AttachComponentReporting(&legacyErr, &info, "terraform", "apply")
			proexec.AttachComponentReporting(&ownerErr, &info, "terraform", "apply")
			require.ErrorIs(t, legacyErr, cause)
			assert.Equal(t, errUtils.GetExitCode(ownerErr), errUtils.GetExitCode(legacyErr))
			var reported *errUtils.ReportingError
			require.ErrorAs(t, legacyErr, &reported)
			snapshot, correlationID := reported.ReportingContext()
			assert.Equal(t, id, correlationID)
			assert.Equal(t, "terraform", snapshot.ComponentType)
			assert.Equal(t, "apply", snapshot.SubCommand)
			assert.Equal(t, "prod", snapshot.Stack)
			assert.Empty(t, info.ComponentType, "reporting must not mutate the caller's configuration")
			var success error
			AttachComponentReporting(&success, &info, "terraform", "apply")
			require.NoError(t, success)
			restore()
			assert.Equal(t, previous, ExecutionID())
		})
	}
}

func TestLegacyCaptureSync(t *testing.T) {
	t.Setenv("CI", "true")
	for _, tc := range []struct {
		name    string
		capture func(*schema.AtmosConfiguration, *ExecRecordInput) error
	}{
		{name: "legacy", capture: CaptureSync},
		{name: "owner", capture: proexec.CaptureSync},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, received := legacyUploadServer(t)
			id, restore := BeginInvocation()
			defer restore()
			input := &ExecRecordInput{Command: "terraform plan", Args: []string{"vpc"}, Flags: []string{"--stack", "prod"}, ExitCode: 2, Data: VersionedData(1, "plan", map[string]any{"changed": 3})}
			require.NoError(t, tc.capture(config, input))
			record := receiveLegacyUpload(t, received)
			assert.Equal(t, id, record.ExecutionID)
			assert.Equal(t, input.Command, record.Command)
			assert.Equal(t, input.Args, record.Args)
			assert.Equal(t, input.Flags, record.Flags)
			assert.Equal(t, 2, record.ExitCode)
			assert.JSONEq(t, `{"version":1,"plan":{"changed":3}}`, string(record.Data))
			assert.Empty(t, input.ExecutionID, "capture defaults must not mutate the caller's input")
		})
	}
	require.NoError(t, CaptureSync(nil, nil), "closed gate must not dereference input")
}

func TestLegacyAsyncStateCrossesPackageBoundary(t *testing.T) {
	t.Setenv("CI", "true")
	for _, tc := range []struct {
		name      string
		setConfig func(*schema.AtmosConfiguration)
		setData   func(any)
		capture   func(*cobra.Command, error)
	}{
		{name: "legacy sets owner reads", setConfig: SetAtmosConfig, setData: SetPendingAsyncData, capture: proexec.CaptureAsync},
		{name: "owner sets legacy reads", setConfig: proexec.SetAtmosConfig, setData: proexec.SetPendingAsyncData, capture: CaptureAsync},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, received := legacyUploadServer(t)
			tc.setConfig(config)
			t.Cleanup(func() { proexec.SetAtmosConfig(nil); proexec.SetPendingAsyncData(nil) })
			id, restore := proexec.BeginInvocation()
			defer restore()
			root := &cobra.Command{Use: "atmos"}
			cmd := &cobra.Command{Use: "list"}
			root.AddCommand(cmd)
			tc.setData(map[string]any{"version": 1, "instances": []string{"vpc"}})
			cause := errUtils.ExitCodeError{Code: 7}
			tc.capture(cmd, cause)
			record := receiveLegacyUpload(t, received)
			assert.Equal(t, id, record.ExecutionID)
			assert.Equal(t, "list", record.Command)
			assert.Equal(t, 7, record.ExitCode)
			assert.JSONEq(t, `{"version":1,"instances":["vpc"]}`, string(record.Data))
			tc.capture(cmd, nil)
			record = receiveLegacyUpload(t, received)
			assert.Nil(t, record.Data, "the payload must be consumed exactly once across both import paths")
			assert.Equal(t, 0, record.ExitCode)
		})
	}
}

func legacyUploadServer(t *testing.T) (*schema.AtmosConfiguration, <-chan dtos.ExecUploadRequest) {
	t.Helper()
	received := make(chan dtos.ExecUploadRequest, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/atmos/exec") {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		var record dtos.ExecUploadRequest
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			t.Errorf("decode execution record: %v", err)
			http.Error(w, "invalid record", http.StatusBadRequest)
			return
		}
		received <- record
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(server.Close)
	config := &schema.AtmosConfiguration{}
	config.Settings.Pro.BaseURL = server.URL
	config.Settings.Pro.Token = "test-token"
	return config, received
}

func receiveLegacyUpload(t *testing.T, received <-chan dtos.ExecUploadRequest) dtos.ExecUploadRequest {
	t.Helper()
	select {
	case record := <-received:
		return record
	case <-time.After(5 * time.Second):
		t.Fatal("execution metadata was not uploaded to the local server")
		return dtos.ExecUploadRequest{}
	}
}
