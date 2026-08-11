package proexec

import (
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

// gateOpen reports whether the current invocation qualifies for
// execution-metadata upload: Atmos must be running in a recognized CI
// environment AND Atmos Pro must be configured with usable credentials.
// This deliberately does not construct an *pro.AtmosProAPIClient (which would
// perform a network-calling OIDC token exchange) — it only checks whether one
// could plausibly be constructed, so this can be called cheaply on every
// command invocation (FR-001/FR-002, research.md Decision 4).
func gateOpen(atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig == nil {
		return false
	}
	return telemetry.IsCI() && proConfigured(atmosConfig)
}

// proConfigured reports whether Atmos Pro has usable credentials configured:
// either a static token, or the full set of GitHub OIDC settings plus a
// workspace ID. Mirrors the branching in pro.NewAtmosProAPIClientFromEnv
// without performing the OIDC exchange.
func proConfigured(atmosConfig *schema.AtmosConfiguration) bool {
	proSettings := atmosConfig.Settings.Pro

	if proSettings.Token != "" {
		return true
	}

	return proSettings.GithubOIDC.RequestURL != "" &&
		proSettings.GithubOIDC.RequestToken != "" &&
		proSettings.WorkspaceID != ""
}
