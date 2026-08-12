package gcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GKEClusterInfo contains the cluster data needed for kubeconfig generation.
type GKEClusterInfo struct {
	Name                     string
	ProjectID                string
	Location                 string
	ID                       string
	Endpoint                 string
	CertificateAuthorityData string
}

// GKEClient is the narrow GKE API surface used by Atmos.
//
//go:generate mockgen -destination=mock_gke_client_test.go -package=gcp -source=gke.go GKEClient
type GKEClient interface {
	GetCluster(ctx context.Context, resourceName string) (*container.Cluster, error)
}

type gkeClient struct {
	clusters *container.ProjectsLocationsClustersService
}

// GetCluster retrieves a GKE cluster by fully qualified resource name.
func (c *gkeClient) GetCluster(ctx context.Context, resourceName string) (*container.Cluster, error) {
	return c.clusters.Get(resourceName).Context(ctx).Do()
}

// NewGKEClient creates a GKE API client from an Atmos GCP access token.
func NewGKEClient(ctx context.Context, creds types.ICredentials) (GKEClient, error) {
	defer perf.Track(nil, "gcp.NewGKEClient")()

	gcpCreds, ok := creds.(*types.GCPCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: expected GCP credentials", errUtils.ErrGKEDescribeCluster)
	}
	if strings.TrimSpace(gcpCreds.AccessToken) == "" {
		return nil, fmt.Errorf("%w: GCP access token is empty", errUtils.ErrGKEDescribeCluster)
	}
	if gcpCreds.IsExpired() {
		return nil, fmt.Errorf("%w: GCP access token is expired", errUtils.ErrGKEDescribeCluster)
	}

	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: gcpCreds.AccessToken,
		Expiry:      gcpCreds.TokenExpiry,
	})
	service, err := container.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create GKE API client: %w", errUtils.ErrGKEDescribeCluster, err)
	}

	return &gkeClient{clusters: service.Projects.Locations.Clusters}, nil
}

// DescribeCluster retrieves the public endpoint and CA data for a GKE cluster.
func DescribeCluster(ctx context.Context, client GKEClient, projectID, location, name string) (*GKEClusterInfo, error) {
	defer perf.Track(nil, "gcp.DescribeCluster")()

	resourceName := ClusterResourceName(projectID, location, name)
	cluster, err := client.GetCluster(ctx, resourceName)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrGKEDescribeCluster, resourceName, err)
	}
	if cluster == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrGKEClusterNotFound, resourceName)
	}

	endpoint := strings.TrimSpace(cluster.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("%w: cluster %s returned an empty API endpoint", errUtils.ErrGKEDescribeCluster, resourceName)
	}
	if !strings.HasPrefix(endpoint, "https://") && !strings.HasPrefix(endpoint, "http://") {
		endpoint = "https://" + endpoint
	}

	if cluster.MasterAuth == nil || strings.TrimSpace(cluster.MasterAuth.ClusterCaCertificate) == "" {
		return nil, fmt.Errorf("%w: cluster %s returned no CA certificate", errUtils.ErrGKEDescribeCluster, resourceName)
	}
	caData := strings.TrimSpace(cluster.MasterAuth.ClusterCaCertificate)
	decodedCA, err := base64.StdEncoding.DecodeString(caData)
	if err != nil || len(decodedCA) == 0 {
		return nil, fmt.Errorf("%w: cluster %s returned malformed base64 CA certificate", errUtils.ErrGKEDescribeCluster, resourceName)
	}

	return &GKEClusterInfo{
		Name:                     name,
		ProjectID:                projectID,
		Location:                 location,
		ID:                       resourceName,
		Endpoint:                 endpoint,
		CertificateAuthorityData: caData,
	}, nil
}

// ClusterResourceName returns the canonical fully-qualified GKE cluster name.
func ClusterResourceName(projectID, location, name string) string {
	return fmt.Sprintf("projects/%s/locations/%s/clusters/%s", projectID, location, name)
}

// BuildKubeClusterInfo adapts GKE cluster data to the shared kubeconfig writer.
func BuildKubeClusterInfo(info *GKEClusterInfo, identityName string) *kube.ClusterInfo {
	execArgs := []string{"gcp", "gke", "token"}

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
		Region:                   info.ProjectID + "-" + info.Location,
		UserPrefix:               "gke",
		ExecArgs:                 execArgs,
		ExecEnv:                  execEnv,
	}
}

// GetToken returns a GCP OAuth2 access token for Kubernetes ExecCredential output.
func GetToken(creds types.ICredentials) (string, time.Time, error) {
	defer perf.Track(nil, "gcp.GetGKEToken")()

	gcpCreds, ok := creds.(*types.GCPCredentials)
	if !ok {
		return "", time.Time{}, fmt.Errorf("%w: expected GCP credentials", errUtils.ErrGKETokenGeneration)
	}
	if strings.TrimSpace(gcpCreds.AccessToken) == "" {
		return "", time.Time{}, fmt.Errorf("%w: GCP access token is empty", errUtils.ErrGKETokenGeneration)
	}
	if gcpCreds.IsExpired() {
		return "", time.Time{}, fmt.Errorf("%w: GCP access token is expired", errUtils.ErrGKETokenGeneration)
	}

	return gcpCreds.AccessToken, gcpCreds.TokenExpiry, nil
}
