package aks

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// executeAKSUpdateKubeconfigViaIntegration runs AKS kubeconfig update using a named integration.
func executeAKSUpdateKubeconfigViaIntegration(integrationName string) error {
	defer perf.Track(nil, "aks.executeAKSUpdateKubeconfigViaIntegration")()

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}

	// Create auth manager.
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}
	credStore := credentials.NewCredentialStoreWithConfig(&atmosConfig.Auth)
	validator := validation.NewValidator()

	mgr, err := auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Execute the integration (handles authentication, AKS describe, kubeconfig write).
	ctx := context.Background()
	return mgr.ExecuteIntegration(ctx, integrationName)
}

// aksKubeconfigDirectParams bundles the parameters needed to authenticate,
// describe an AKS cluster, and write its kubeconfig via the direct
// (non-integration) path, used by executeAKSUpdateKubeconfigDirect.
type aksKubeconfigDirectParams struct {
	clusterName    string
	resourceGroup  string
	subscriptionID string
	kubeconfigPath string
	alias          string
	identityName   string
}

// executeAKSUpdateKubeconfigDirect runs AKS kubeconfig update using the Go SDK with explicit parameters.
func executeAKSUpdateKubeconfigDirect(p *aksKubeconfigDirectParams) error {
	defer perf.Track(nil, "aks.executeAKSUpdateKubeconfigDirect")()

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}

	// Create auth manager and authenticate.
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}
	credStore := credentials.NewCredentialStoreWithConfig(&atmosConfig.Auth)
	validator := validation.NewValidator()

	mgr, err := auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	ctx := context.Background()
	whoami, err := mgr.Authenticate(ctx, p.identityName)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrIdentityAuthFailed, err)
	}

	if whoami.Credentials == nil {
		return fmt.Errorf("%w: no credentials available", errUtils.ErrIdentityAuthFailed)
	}

	if p.subscriptionID == "" {
		if azureCreds, ok := whoami.Credentials.(*types.AzureCredentials); ok {
			p.subscriptionID = azureCreds.SubscriptionID
		}
	}

	return writeAKSKubeconfigDirect(ctx, whoami.Credentials, p)
}

// writeAKSKubeconfigDirect describes the AKS cluster and writes its kubeconfig
// entry, extracted from executeAKSUpdateKubeconfigDirect to keep it within
// the function-length limit.
func writeAKSKubeconfigDirect(ctx context.Context, creds types.ICredentials, p *aksKubeconfigDirectParams) error {
	// Create AKS client and describe cluster.
	client, err := azureCloud.NewAKSClient(ctx, creds, p.subscriptionID)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSIntegrationFailed, err)
	}

	info, err := azureCloud.DescribeCluster(ctx, client, p.subscriptionID, p.resourceGroup, p.clusterName)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSIntegrationFailed, err)
	}

	// Write kubeconfig.
	kubeMgr, err := kube.NewKubeconfigManager(p.kubeconfigPath, "")
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSIntegrationFailed, err)
	}

	if _, err := kubeMgr.WriteClusterConfig(azureCloud.BuildKubeClusterInfo(info, p.identityName), p.alias, "merge"); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSIntegrationFailed, err)
	}

	displayName := p.alias
	if displayName == "" {
		displayName = info.ID
	}
	ui.Success(fmt.Sprintf("AKS kubeconfig: %s → %s", displayName, kubeMgr.GetPath()))

	return nil
}
