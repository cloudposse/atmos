package auth

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/stretchr/testify/assert"
)

// TestIntegrationKindsRegisteredViaProductionImports guards against a regression
// where an integration package under pkg/auth/integrations/* registers its kind(s)
// in init() but is never blank-imported by pkg/auth/manager.go — leaving the kind
// unresolvable at runtime (integrations.Create → "unknown integration kind") even
// though the package's own unit tests, which import the sub-package directly, still
// pass and mask the gap.
//
// This test lives in package auth (not auth_test) and imports NO integrations/*
// sub-package directly, so the only thing that can register these kinds here is
// manager.go's blank-import block — i.e. the exact production import path. Remove
// any of those blank imports and this test fails.
//
// Regression: azure/aks and azure/acr shipped unregistered because
// `_ ".../pkg/auth/integrations/azure"` was missing from manager.go (the aws and
// github integration packages were blank-imported there; azure was not).
func TestIntegrationKindsRegisteredViaProductionImports(t *testing.T) {
	wantKinds := []string{
		integrations.KindAWSECR,
		integrations.KindAWSECRPublic,
		integrations.KindAWSEKS,
		integrations.KindAzureACR,
		integrations.KindAzureAKS,
		integrations.KindGitHubSTS,
	}

	for _, kind := range wantKinds {
		assert.Truef(t, integrations.IsRegistered(kind),
			"integration kind %q must be registered via pkg/auth/manager.go's blank imports; "+
				"add `_ \"github.com/cloudposse/atmos/pkg/auth/integrations/<pkg>\"` to manager.go if missing",
			kind)
	}
}
