package eks

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// executeEKSUpdateKubeconfigViaIntegration runs EKS kubeconfig update using a named integration.
// This authenticates via the auth manager and uses the Go SDK instead of shelling out to AWS CLI.
func executeEKSUpdateKubeconfigViaIntegration(integrationName string) error {
	defer perf.Track(nil, "eks.executeEKSUpdateKubeconfigViaIntegration")()

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}

	// Create auth manager.
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	mgr, err := auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Execute the integration (handles authentication, EKS describe, kubeconfig write).
	ctx := context.Background()
	return mgr.ExecuteIntegration(ctx, integrationName)
}

// executeEKSUpdateKubeconfigDirect runs EKS kubeconfig update using Go SDK with explicit parameters.
// This authenticates the identity and uses the Go SDK to describe the cluster and write kubeconfig.
func executeEKSUpdateKubeconfigDirect(clusterName, region, kubeconfigPath, alias, identityName string) error {
	defer perf.Track(nil, "eks.executeEKSUpdateKubeconfigDirect")()

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}

	// Create auth manager and authenticate.
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	mgr, err := auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo, atmosConfig.CliConfigPath)
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

	if err := kubeMgr.WriteClusterConfig(info, alias, identityName, "merge"); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	displayName := alias
	if displayName == "" {
		displayName = info.ARN
	}
	ui.Success(fmt.Sprintf("EKS kubeconfig: %s → %s", displayName, kubeMgr.GetPath()))

	return nil
}
