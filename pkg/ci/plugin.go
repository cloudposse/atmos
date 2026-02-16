// Package ci provides CI/CD provider abstractions and integrations.
package ci

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -typed -destination=mock_plugin_test.go -package=ci github.com/cloudposse/atmos/pkg/ci/internal/plugin Plugin,ComponentConfigurationResolver

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
)

// HookAction represents what CI action to perform.
type HookAction = plugin.HookAction

const (
	// ActionSummary writes to job summary ($GITHUB_STEP_SUMMARY).
	ActionSummary = plugin.ActionSummary

	// ActionOutput writes to CI outputs ($GITHUB_OUTPUT).
	ActionOutput = plugin.ActionOutput

	// ActionUpload uploads an artifact (e.g., planfile).
	ActionUpload = plugin.ActionUpload

	// ActionDownload downloads an artifact.
	ActionDownload = plugin.ActionDownload

	// ActionCheck validates or checks (e.g., drift detection).
	ActionCheck = plugin.ActionCheck
)

// HookBinding declares what happens at a specific hook event.
type HookBinding = plugin.HookBinding

// Plugin is implemented by component types that support CI integration.
// Covers templates, outputs, and artifacts for pipeline automation.
// Unlike Provider (which represents CI platforms like GitHub/GitLab), this interface
// represents component types (terraform, helmfile) and their CI behavior.
type Plugin = plugin.Plugin

// ComponentConfigurationResolver is an optional interface that Plugins can implement
// to resolve artifact paths (e.g., planfile paths) when not explicitly provided.
// The executor checks for this interface before upload/download actions and uses it
// to derive the path from component and stack information.
type ComponentConfigurationResolver = plugin.ComponentConfigurationResolver

// OutputResult contains parsed command output.
type OutputResult = plugin.OutputResult

// TerraformOutputData contains terraform-specific output data.
type TerraformOutputData = plugin.TerraformOutputData

// TerraformOutput represents a single terraform output value.
type TerraformOutput = plugin.TerraformOutput

// MovedResource represents a resource that has been moved.
type MovedResource = plugin.MovedResource

// ResourceCounts contains resource change counts.
type ResourceCounts = plugin.ResourceCounts

// HelmfileOutputData contains helmfile-specific output data.
type HelmfileOutputData = plugin.HelmfileOutputData

// ReleaseInfo contains helmfile release information.
type ReleaseInfo = plugin.ReleaseInfo

// TemplateContext contains all data available to CI summary templates.
type TemplateContext = plugin.TemplateContext

// HookBindings is a slice of HookBinding with helper methods.
type HookBindings = plugin.HookBindings
