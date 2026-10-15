package eks

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Package-level seams so the Go SDK orchestration here (config load, auth-manager
// creation) can be unit-tested without a live AWS account. Overridden in tests;
// the same function-var pattern used in cmd/azure/aks/update_kubeconfig_sdk.go.
var (
	initCliConfig  = cfg.InitCliConfig
	newAuthManager = auth.NewAuthManager
)

// executeEKSUpdateKubeconfigViaIntegration runs EKS kubeconfig update using a named integration.
// This authenticates via the auth manager and uses the Go SDK instead of shelling out to AWS CLI.
// kubeconfigPath and alias are optional CLI overrides (empty means "use the integration's
// configured value") forwarded so --kubeconfig/--alias work on the integration path.
func executeEKSUpdateKubeconfigViaIntegration(integrationName, kubeconfigPath, alias string) error {
	defer perf.Track(nil, "eks.executeEKSUpdateKubeconfigViaIntegration")()

	// Load atmos config.
	atmosConfig, err := initCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}

	// Create auth manager.
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}
	credStore := credentials.NewCredentialStoreWithConfig(&atmosConfig.Auth)
	validator := validation.NewValidator()

	mgr, err := newAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Execute the integration (handles authentication, EKS describe, kubeconfig write).
	// The CLI overrides are no-ops when empty, so the integration's configured values win.
	ctx := context.Background()
	return mgr.ExecuteIntegration(ctx, integrationName,
		types.WithKubeconfigPath(kubeconfigPath),
		types.WithAlias(alias))
}

// executeEKSUpdateKubeconfigDirect runs EKS kubeconfig update using Go SDK with explicit parameters.
// This authenticates the identity and uses the Go SDK to describe the cluster and write kubeconfig.
func executeEKSUpdateKubeconfigDirect(clusterName, region, kubeconfigPath, alias, identityName string) error {
	defer perf.Track(nil, "eks.executeEKSUpdateKubeconfigDirect")()

	// Load atmos config.
	atmosConfig, err := initCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}

	// Create auth manager and authenticate.
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}
	credStore := credentials.NewCredentialStoreWithConfig(&atmosConfig.Auth)
	validator := validation.NewValidator()

	mgr, err := newAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	ctx := context.Background()
	whoami, err := mgr.Authenticate(ctx, identityName)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrIdentityAuthFailed, err)
	}

	if whoami.Credentials == nil {
		return fmt.Errorf("%w: no credentials available", errUtils.ErrIdentityAuthFailed)
	}

	// Create EKS client and describe cluster.
	client, err := awsCloud.NewEKSClient(ctx, whoami.Credentials, region)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	info, err := awsCloud.DescribeCluster(ctx, client, clusterName, region)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	// Write kubeconfig.
	kubeMgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	if _, err := kubeMgr.WriteClusterConfig(awsCloud.BuildKubeClusterInfo(info, identityName), alias, "merge"); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	displayName := alias
	if displayName == "" {
		displayName = info.ARN
	}
	ui.Success(fmt.Sprintf("EKS kubeconfig: %s → %s", displayName, kubeMgr.GetPath()))

	return nil
}
