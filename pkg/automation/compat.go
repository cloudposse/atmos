// Package automation preserves the original Go automation contracts.
//
// Deprecated: use github.com/cloudposse/atmos/pkg/api for new integrations.
package automation

import "github.com/cloudposse/atmos/pkg/api"

// StepLibrary dispatches registered Atmos steps.
//
// Deprecated: use api.StepLibrary.
type StepLibrary = api.StepLibrary

// StepCall contains native Go step configuration.
//
// Deprecated: use api.StepCall.
type StepCall = api.StepCall

// StepResult exposes the value and outputs returned by a step.
//
// Deprecated: use api.StepResult.
type StepResult = api.StepResult

// CastLibrary supplies the optional recording service.
//
// Deprecated: use api.CastLibrary.
type CastLibrary = api.CastLibrary

// ExecutionPolicy bounds an operation and its retry waits.
//
// Deprecated: use api.ExecutionPolicy.
type ExecutionPolicy = api.ExecutionPolicy

// FileSystem supplies read-only filesystem operations.
//
// Deprecated: use api.FileSystem.
type FileSystem = api.FileSystem

// FileInfo describes a file without reading its contents.
//
// Deprecated: use api.FileInfo.
type FileInfo = api.FileInfo

// LocalFileSystem implements filesystem operations using the operating system.
//
// Deprecated: use api.LocalFileSystem.
type LocalFileSystem = api.LocalFileSystem
