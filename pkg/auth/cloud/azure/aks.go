package azure

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	armcontainerservice "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"k8s.io/client-go/tools/clientcmd"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// AKSClusterInfo contains cluster data needed for kubeconfig generation.
type AKSClusterInfo struct {
	Name          string
	ResourceGroup string
	// SubscriptionID is the subscription the cluster was addressed under, used
	// to build the exec-plugin --subscription-id argument.
	SubscriptionID string
	// ID is the ARM resource ID, used as the kubeconfig cluster map key (the
	// AKS equivalent of an EKS cluster ARN).
	ID       string
	Endpoint string
	// CertificateAuthorityData is base64-encoded, matching EKS's
	// EKSClusterInfo convention — kube.BuildClusterConfig decodes it.
	CertificateAuthorityData string
	// ServerID is the AAD server application ID this cluster's AAD
	// integration expects tokens to be scoped to, parsed from the
	// kubelogin exec args embedded in the exec-format kubeconfig. Empty for
	// clusters where the exec args couldn't be parsed (falls back to the
	// well-known AKSServerAppID).
	ServerID string
	// TenantID is the AAD tenant ID parsed from the same exec args.
	TenantID string
}

type aksServerIDContextKey struct{}

// ContextWithAKSServerID records the cluster's AAD server application ID for
// the Azure provider that authenticates the kubeconfig exec-plugin request.
func ContextWithAKSServerID(ctx context.Context, serverID string) context.Context {
	if serverID == "" {
		return ctx
	}
	return context.WithValue(ctx, aksServerIDContextKey{}, serverID)
}

// AKSServerScopeFromContext returns the requested AKS AAD scope, falling back
// to the well-known AKS server application when no cluster-specific ID exists.
func AKSServerScopeFromContext(ctx context.Context) string {
	if serverID, ok := ctx.Value(aksServerIDContextKey{}).(string); ok && serverID != "" {
		return serverID + "/.default"
	}
	return AKSServerScope
}

// AKSClient defines the interface for AKS API calls (for testability).
//
//go:generate mockgen -destination=mock_aks_client_test.go -package=azure -source=aks.go AKSClient
type AKSClient interface {
	Get(ctx context.Context, resourceGroupName, resourceName string, options *armcontainerservice.ManagedClustersClientGetOptions) (armcontainerservice.ManagedClustersClientGetResponse, error)
	ListClusterUserCredentials(ctx context.Context, resourceGroupName, resourceName string, options *armcontainerservice.ManagedClustersClientListClusterUserCredentialsOptions) (armcontainerservice.ManagedClustersClientListClusterUserCredentialsResponse, error)
}

// NewAKSClient creates a new AKS ManagedClusters client from Atmos credentials.
// SubscriptionID overrides the credential's subscription when non-empty.
func NewAKSClient(ctx context.Context, creds types.ICredentials, subscriptionID string) (AKSClient, error) {
	defer perf.Track(nil, "azure.NewAKSClient")()

	azureCreds, ok := creds.(*types.AzureCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: expected Azure credentials", errUtils.ErrAKSDescribeCluster)
	}

	effectiveSubscriptionID := subscriptionID
	if effectiveSubscriptionID == "" {
		effectiveSubscriptionID = azureCreds.SubscriptionID
	}
	if effectiveSubscriptionID == "" {
		return nil, fmt.Errorf("%w: subscription ID is required", errUtils.ErrAKSDescribeCluster)
	}

	tokenCred, err := BuildAzureCredentialFromCreds(creds)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSDescribeCluster, err)
	}

	cloudEnv := GetCloudEnvironment(azureCreds.CloudEnvironment)
	client, err := armcontainerservice.NewManagedClustersClient(effectiveSubscriptionID, tokenCred, BuildARMClientOptions(cloudEnv))
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSDescribeCluster, err)
	}

	return client, nil
}

// DescribeCluster retrieves cluster information needed for kubeconfig.
//
// It calls Get for the ARM resource ID, then ListClusterUserCredentials with
// Format: exec to obtain a ready-made kubeconfig whose single cluster entry
// carries the server endpoint and CA data, and whose single auth-info entry
// carries the kubelogin exec args this specific cluster expects (--server-id,
// --tenant-id) — parsed out and reused to scope `atmos azure aks token`,
// rather than invoking kubelogin itself, so no external binary is required.
func DescribeCluster(ctx context.Context, client AKSClient, subscriptionID, resourceGroup, name string) (*AKSClusterInfo, error) {
	defer perf.Track(nil, "azure.DescribeCluster")()

	resourceID, err := getClusterResourceID(ctx, client, resourceGroup, name)
	if err != nil {
		return nil, err
	}

	clusterEntry, authInfo, err := getExecKubeconfigEntries(ctx, client, resourceGroup, name)
	if err != nil {
		return nil, err
	}

	serverID, tenantID := parseKubeloginExecArgs(authInfo.Exec.Args)

	return &AKSClusterInfo{
		Name:                     name,
		ResourceGroup:            resourceGroup,
		SubscriptionID:           subscriptionID,
		ID:                       resourceID,
		Endpoint:                 clusterEntry.Server,
		CertificateAuthorityData: base64.StdEncoding.EncodeToString(clusterEntry.CertificateAuthorityData),
		ServerID:                 serverID,
		TenantID:                 tenantID,
	}, nil
}

// getClusterResourceID calls Get to obtain the ARM resource ID for a cluster,
// used by DescribeCluster before fetching the exec-format kubeconfig.
func getClusterResourceID(ctx context.Context, client AKSClient, resourceGroup, name string) (string, error) {
	getResp, err := client.Get(ctx, resourceGroup, name, nil)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSDescribeCluster, err)
	}

	resourceID := ""
	if getResp.ID != nil {
		resourceID = *getResp.ID
	}
	if resourceID == "" {
		return "", fmt.Errorf("%w: %s", errUtils.ErrAKSClusterNotFound, name)
	}

	return resourceID, nil
}

// getExecKubeconfigEntries fetches the exec-format credential kubeconfig for
// a cluster and returns its single cluster and auth-info entries, used by
// DescribeCluster to obtain the server endpoint, CA data, and kubelogin exec
// args (--server-id, --tenant-id) this cluster's AAD integration expects.
func getExecKubeconfigEntries(ctx context.Context, client AKSClient, resourceGroup, name string) (*clientcmdapi.Cluster, *clientcmdapi.AuthInfo, error) {
	execFormat := armcontainerservice.FormatExec
	credsResp, err := client.ListClusterUserCredentials(ctx, resourceGroup, name, &armcontainerservice.ManagedClustersClientListClusterUserCredentialsOptions{
		Format: &execFormat,
	})
	if err != nil {
		return nil, nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSDescribeCluster, err)
	}

	if len(credsResp.Kubeconfigs) == 0 || credsResp.Kubeconfigs[0] == nil {
		return nil, nil, fmt.Errorf("%w: no kubeconfig returned for cluster %s", errUtils.ErrAKSDescribeCluster, name)
	}

	// CredentialResult.Value is typed []byte, so encoding/json already
	// base64-decoded it from the API's JSON string during unmarshaling.
	parsedConfig, err := clientcmd.Load(credsResp.Kubeconfigs[0].Value)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to parse returned kubeconfig: %w", errUtils.ErrAKSDescribeCluster, err)
	}

	clusterEntry := firstValue(parsedConfig.Clusters)
	if clusterEntry == nil {
		return nil, nil, fmt.Errorf("%w: returned kubeconfig has no cluster entries", errUtils.ErrAKSDescribeCluster)
	}

	authInfo := firstValue(parsedConfig.AuthInfos)
	if authInfo == nil || authInfo.Exec == nil {
		// Format: exec silently falls back to a cert/local-account kubeconfig
		// for non-AAD-enabled clusters — no exec block means no AAD integration.
		return nil, nil, fmt.Errorf("%w: %s", errUtils.ErrAKSClusterNotAADEnabled, name)
	}

	return clusterEntry, authInfo, nil
}

// firstValue returns an arbitrary value from a map, or nil for an empty map.
// The exec-format credential kubeconfig always contains exactly one cluster
// and one auth-info entry, so iteration order doesn't matter.
func firstValue[K comparable, V any](m map[K]V) V {
	for _, v := range m {
		return v
	}
	var zero V
	return zero
}

// parseKubeloginExecArgs scans a kubelogin exec-plugin argument list for
// --server-id and --tenant-id values, so `atmos azure aks token` can request
// an AAD token scoped to the cluster's actual AAD server application instead
// of assuming the well-known AKSServerAppID.
func parseKubeloginExecArgs(args []string) (serverID, tenantID string) {
	// Iterate up to len(args)-1 so args[i+1] is always in bounds.
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "--server-id":
			serverID = args[i+1]
		case "--tenant-id":
			tenantID = args[i+1]
		}
	}
	return serverID, tenantID
}

// BuildKubeClusterInfo adapts an AKSClusterInfo + identity name into the
// cloud-agnostic kube.ClusterInfo, building the "azure aks token" exec-plugin
// command line that kube.BuildClusterConfig embeds verbatim.
func BuildKubeClusterInfo(info *AKSClusterInfo, identityName string) *kube.ClusterInfo {
	execArgs := []string{
		"azure",
		"aks",
		"token",
		"--cluster-name",
		info.Name,
		"--resource-group",
		info.ResourceGroup,
	}
	if info.ServerID != "" {
		execArgs = append(execArgs, "--server-id", info.ServerID)
	}
	if info.SubscriptionID != "" {
		execArgs = append(execArgs, "--subscription-id", info.SubscriptionID)
	}

	var execEnv []clientcmdapi.ExecEnvVar
	if identityName != "" {
		execArgs = append(execArgs, "--identity="+identityName)
		execEnv = append(execEnv, clientcmdapi.ExecEnvVar{
			Name:  "ATMOS_IDENTITY",
			Value: identityName,
		})
	}

	return &kube.ClusterInfo{
		Name:                     info.Name,
		Endpoint:                 info.Endpoint,
		CertificateAuthorityData: info.CertificateAuthorityData,
		ID:                       info.ID,
		Region:                   info.ResourceGroup,
		UserPrefix:               "aks",
		ExecArgs:                 execArgs,
		ExecEnv:                  execEnv,
	}
}

// GetToken returns the AKS-scoped bearer token for kubectl. The token itself
// is acquired at identity-authentication time by the credential's provider
// (device-code, OIDC, or Azure CLI — see pkg/auth/providers/azure), since
// Azure AAD tokens are scope-bound at issuance, unlike AWS SigV4 signing.
func GetToken(creds types.ICredentials) (string, time.Time, error) {
	defer perf.Track(nil, "azure.GetToken")()

	azureCreds, ok := creds.(*types.AzureCredentials)
	if !ok {
		return "", time.Time{}, fmt.Errorf("%w: expected Azure credentials", errUtils.ErrAKSTokenGeneration)
	}

	if azureCreds.AKSToken == "" {
		return "", time.Time{}, fmt.Errorf("%w: identity's provider did not yield an AKS-scoped token; ensure the identity resolves through a device-code, OIDC, or Azure CLI provider", errUtils.ErrAKSTokenGeneration)
	}

	if azureCreds.AKSTokenExpiration == "" {
		return azureCreds.AKSToken, time.Time{}, nil
	}

	expiresAt, err := time.Parse(time.RFC3339, azureCreds.AKSTokenExpiration)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: failed to parse AKS token expiration: %w", errUtils.ErrAKSTokenGeneration, err)
	}

	return azureCreds.AKSToken, expiresAt, nil
}
