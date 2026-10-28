// Package proexec preserves the original import path for Pro execution metadata.
//
// Deprecated: Use github.com/cloudposse/atmos/pkg/pro/exec.
package proexec

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	proexec "github.com/cloudposse/atmos/pkg/pro/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecRecordInput describes the record uploaded to Atmos Pro.
//
// Deprecated: Use exec.ExecRecordInput from pkg/pro/exec.
type ExecRecordInput = proexec.ExecRecordInput

// CaptureSync delegates to its owning package.
//
// Deprecated: Use proexec.CaptureSync.
func CaptureSync(atmosConfig *schema.AtmosConfiguration, in *ExecRecordInput) error {
	defer perf.Track(nil, "proexec.CaptureSync")()

	return proexec.CaptureSync(atmosConfig, in)
}

// IsSyncCommand delegates to its owning package.
//
// Deprecated: Use proexec.IsSyncCommand.
func IsSyncCommand(commandPath string) bool {
	defer perf.Track(nil, "proexec.IsSyncCommand")()

	return proexec.IsSyncCommand(commandPath)
}

// GateOpen delegates to its owning package.
//
// Deprecated: Use proexec.GateOpen.
func GateOpen(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(nil, "proexec.GateOpen")()

	return proexec.GateOpen(atmosConfig)
}

// AttachComponentReporting delegates to its owning package.
//
// Deprecated: Use proexec.AttachComponentReporting.
func AttachComponentReporting(result *error, info *schema.ConfigAndStacksInfo, componentType, command string) {
	defer perf.Track(nil, "proexec.AttachComponentReporting")()

	proexec.AttachComponentReporting(result, info, componentType, command)
}

// VersionedData delegates to its owning package.
//
// Deprecated: Use proexec.VersionedData.
func VersionedData(version int, key string, payload any) map[string]any {
	defer perf.Track(nil, "proexec.VersionedData")()

	return proexec.VersionedData(version, key, payload)
}

// SetAtmosConfig delegates to its owning package.
//
// Deprecated: Use proexec.SetAtmosConfig.
func SetAtmosConfig(atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(nil, "proexec.SetAtmosConfig")()

	proexec.SetAtmosConfig(atmosConfig)
}

// SetPendingAsyncData delegates to its owning package.
//
// Deprecated: Use proexec.SetPendingAsyncData.
func SetPendingAsyncData(data any) {
	defer perf.Track(nil, "proexec.SetPendingAsyncData")()

	proexec.SetPendingAsyncData(data)
}

// CaptureAsync delegates to its owning package.
//
// Deprecated: Use proexec.CaptureAsync.
func CaptureAsync(cmd *cobra.Command, err error) {
	defer perf.Track(nil, "proexec.CaptureAsync")()

	proexec.CaptureAsync(cmd, err)
}

// FlagsFromCommand delegates to its owning package.
//
// Deprecated: Use proexec.FlagsFromCommand.
func FlagsFromCommand(cmd *cobra.Command) []string {
	defer perf.Track(nil, "proexec.FlagsFromCommand")()

	return proexec.FlagsFromCommand(cmd)
}

// BeginInvocation delegates to its owning package.
//
// Deprecated: Use proexec.BeginInvocation.
func BeginInvocation() (string, func()) {
	defer perf.Track(nil, "proexec.BeginInvocation")()

	return proexec.BeginInvocation()
}

// ExecutionID delegates to its owning package.
//
// Deprecated: Use proexec.ExecutionID.
func ExecutionID() string {
	defer perf.Track(nil, "proexec.ExecutionID")()

	return proexec.ExecutionID()
}
