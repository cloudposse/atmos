package eks

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/realm"
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

// eksDirectRequest is one kubeconfig update on the direct (non-integration)
// Go SDK path.
type eksDirectRequest struct {
	clusterName    string
	region         string
	kubeconfigPath string
	alias          string
	identityName   string
	// dryRun prints the kubeconfig entries instead of writing them, and
	// authenticates without the identity's auto-provisioned integrations.
	dryRun bool
}

// loadEKSAuthManager loads the atmos config and creates the auth manager.
func loadEKSAuthManager() (*schema.AtmosConfiguration, types.AuthManager, error) {
	// Load atmos config.
	atmosConfig, err := initCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}

	// Create auth manager.
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}
	credStore := credentials.NewCredentialStoreWithConfig(&atmosConfig.Auth)
	validator := validation.NewValidator()

	mgr, err := newAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo, atmosConfig.CliConfigPath)
	if err != nil {
		return nil, nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}
	return &atmosConfig, mgr, nil
}

// executeEKSUpdateKubeconfigViaIntegration runs EKS kubeconfig update using a named integration.
// This authenticates via the auth manager and uses the Go SDK instead of shelling out to AWS CLI.
// The kubeconfigPath and alias are optional CLI overrides (empty means "use the integration's
// configured value") forwarded so --kubeconfig/--alias work on the integration path.
func executeEKSUpdateKubeconfigViaIntegration(integrationName, kubeconfigPath, alias string) error {
	defer perf.Track(nil, "eks.executeEKSUpdateKubeconfigViaIntegration")()

	_, mgr, err := loadEKSAuthManager()
	if err != nil {
		return err
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

	return runEKSUpdateKubeconfigDirect(&eksDirectRequest{
		clusterName: clusterName, region: region, kubeconfigPath: kubeconfigPath, alias: alias, identityName: identityName,
	})
}

// runEKSUpdateKubeconfigDirect loads the config and auth manager, then runs req.
func runEKSUpdateKubeconfigDirect(req *eksDirectRequest) error {
	atmosConfig, mgr, err := loadEKSAuthManager()
	if err != nil {
		return err
	}
	return updateEKSKubeconfigDirect(context.Background(), atmosConfig, mgr, req)
}

// updateEKSKubeconfigDirect authenticates req's identity, describes the cluster
// and writes its kubeconfig entry, or prints it for a dry run.
func updateEKSKubeconfigDirect(ctx context.Context, atmosConfig *schema.AtmosConfiguration, mgr types.AuthManager, req *eksDirectRequest) error {
	// A dry run must have no side effects, so the identity's auto-provisioned
	// integrations (an ECR login, the integration's own kubeconfig) are skipped.
	authCtx := ctx
	if req.dryRun {
		authCtx = auth.ContextWithSkipIntegrations(ctx)
	}
	whoami, err := mgr.Authenticate(authCtx, req.identityName)
	if err != nil {
		return errUtils.WrapIdentityAuthFailed(req.identityName, err)
	}

	// mgr is an AuthManager interface; guard against a nil whoami (a (nil, nil)
	// return) before dereferencing Credentials, in addition to the by-design
	// nil-credentials case. Mirrors the AKS direct path.
	if whoami == nil || whoami.Credentials == nil {
		return fmt.Errorf("%w: no credentials available", errUtils.ErrIdentityAuthFailed)
	}

	info, clusterInfo, err := describeEKSKubeCluster(ctx, whoami.Credentials, req)
	if err != nil {
		return err
	}
	// Scope the kube names to an explicitly configured auth realm, as the
	// aws/eks integration does; a derived realm keeps the names unchanged.
	clusterInfo.Realm, clusterInfo.LegacyRealms = kube.NameRealm(mgr.GetRealm().Value, realm.IsExplicit(mgr.GetRealm(), atmosConfig.Auth.RealmSource))

	kubeMgr, err := kube.NewKubeconfigManager(req.kubeconfigPath, "")
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrEKSIntegrationFailed, err)
	}

	// The display name is the context name written: the alias, else [<realm>/]<ARN>.
	displayName := kube.ContextName(info.ARN, req.alias, clusterInfo.Realm)
	if req.dryRun {
		return printEKSKubeconfigDryRun(clusterInfo, req.alias, displayName, kubeMgr.GetPath())
	}

	if _, err := kubeMgr.WriteClusterConfig(clusterInfo, req.alias, "merge"); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrEKSIntegrationFailed, err)
	}
	ui.Success(fmt.Sprintf("EKS kubeconfig: %s → %s", displayName, kubeMgr.GetPath()))

	return nil
}

// describeEKSKubeCluster describes req's cluster and builds its kubeconfig entry.
func describeEKSKubeCluster(ctx context.Context, creds types.ICredentials, req *eksDirectRequest) (*awsCloud.EKSClusterInfo, *kube.ClusterInfo, error) {
	// Create EKS client and describe cluster.
	client, err := awsCloud.NewEKSClient(ctx, creds, req.region)
	if err != nil {
		return nil, nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrEKSIntegrationFailed, err)
	}

	info, err := awsCloud.DescribeCluster(ctx, client, req.clusterName, req.region)
	if err != nil {
		return nil, nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrEKSIntegrationFailed, err)
	}

	clusterInfo, err := awsCloud.BuildKubeClusterInfo(info, req.identityName)
	if err != nil {
		return nil, nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrEKSIntegrationFailed, err)
	}
	return info, clusterInfo, nil
}
