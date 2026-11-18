package aws

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/tools/clientcmd"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestDescribeCluster_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	client := NewMockEKSClient(ctrl)
	client.EXPECT().
		DescribeCluster(gomock.Any(), gomock.Any()).
		Return(&eks.DescribeClusterOutput{
			Cluster: &ekstypes.Cluster{
				Name:     aws.String("dev-cluster"),
				Endpoint: aws.String("https://XXXX.gr7.us-east-2.eks.amazonaws.com"),
				CertificateAuthority: &ekstypes.Certificate{
					Data: aws.String("LS0tLS1CRUdJTi..."),
				},
				Arn: aws.String("arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"),
			},
		}, nil)

	info, err := DescribeCluster(context.Background(), client, "dev-cluster", "us-east-2")
	require.NoError(t, err)
	assert.Equal(t, "dev-cluster", info.Name)
	assert.Equal(t, "https://XXXX.gr7.us-east-2.eks.amazonaws.com", info.Endpoint)
	assert.Equal(t, "LS0tLS1CRUdJTi...", info.CertificateAuthorityData)
	assert.Equal(t, "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster", info.ARN)
	assert.Equal(t, "us-east-2", info.Region)
}

func TestDescribeCluster_NilCluster(t *testing.T) {
	ctrl := gomock.NewController(t)

	client := NewMockEKSClient(ctrl)
	client.EXPECT().
		DescribeCluster(gomock.Any(), gomock.Any()).
		Return(&eks.DescribeClusterOutput{Cluster: nil}, nil)

	_, err := DescribeCluster(context.Background(), client, "missing-cluster", "us-east-2")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSClusterNotFound)
}

func TestDescribeCluster_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)

	client := NewMockEKSClient(ctrl)
	client.EXPECT().
		DescribeCluster(gomock.Any(), gomock.Any()).
		Return(nil, assert.AnError)

	_, err := DescribeCluster(context.Background(), client, "dev-cluster", "us-east-2")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSDescribeCluster)
}

func TestDescribeCluster_NilCertificateAuthority(t *testing.T) {
	ctrl := gomock.NewController(t)

	client := NewMockEKSClient(ctrl)
	client.EXPECT().
		DescribeCluster(gomock.Any(), gomock.Any()).
		Return(&eks.DescribeClusterOutput{
			Cluster: &ekstypes.Cluster{
				Name:                 aws.String("dev-cluster"),
				Endpoint:             aws.String("https://example.eks.amazonaws.com"),
				CertificateAuthority: nil,
				Arn:                  aws.String("arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"),
			},
		}, nil)

	info, err := DescribeCluster(context.Background(), client, "dev-cluster", "us-east-2")
	require.NoError(t, err)
	assert.Equal(t, "", info.CertificateAuthorityData)
}

func TestGetToken_InvalidCredentials(t *testing.T) {
	// Non-AWS credentials should fail.
	_, _, err := GetToken(context.Background(), nil, "dev-cluster", "us-east-2")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSTokenGeneration)
}

func TestGetToken_Success(t *testing.T) {
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBYaDH...",
		Region:          "us-east-2",
	}

	token, expiresAt, err := GetToken(context.Background(), creds, "dev-cluster", "us-east-2")
	require.NoError(t, err)

	// Token should start with k8s-aws-v1. prefix.
	assert.True(t, strings.HasPrefix(token, eksTokenPrefix), "token should start with %s", eksTokenPrefix)

	// Token should not be empty after prefix.
	assert.Greater(t, len(token), len(eksTokenPrefix))

	// Expiration should be in the future.
	assert.True(t, expiresAt.After(time.Now()))
}

func TestNewEKSClient_InvalidCredentials(t *testing.T) {
	// Non-AWS credentials should fail.
	_, err := NewEKSClient(context.Background(), nil, "us-east-2")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSDescribeCluster)
}

func TestBuildKubeClusterInfo_WithIdentity(t *testing.T) {
	info := &EKSClusterInfo{
		Name:                     "dev-cluster",
		Endpoint:                 "https://XXXX.gr7.us-east-2.eks.amazonaws.com",
		CertificateAuthorityData: "LS0tLS1CRUdJTi...",
		ARN:                      "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster",
		Region:                   "us-east-2",
	}

	clusterInfo, err := BuildKubeClusterInfo(info, "dev-admin")

	require.NoError(t, err)
	assert.Equal(t, "dev-cluster", clusterInfo.Name)
	assert.Equal(t, "https://XXXX.gr7.us-east-2.eks.amazonaws.com", clusterInfo.Endpoint)
	assert.Equal(t, "LS0tLS1CRUdJTi...", clusterInfo.CertificateAuthorityData)
	assert.Equal(t, "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster", clusterInfo.ID)
	assert.Equal(t, "us-east-2", clusterInfo.Region)
	assert.Equal(t, "eks", clusterInfo.UserPrefix)
	assert.Equal(t, []string{"aws", "eks", "token", "--cluster-name", "dev-cluster", "--region", "us-east-2", "--identity=dev-admin"}, clusterInfo.ExecArgs)
	require.Len(t, clusterInfo.ExecEnv, 2)
	assert.Equal(t, "ATMOS_IDENTITY", clusterInfo.ExecEnv[0].Name)
	assert.Equal(t, "dev-admin", clusterInfo.ExecEnv[0].Value)
}

func TestBuildKubeClusterInfo_WithoutIdentity(t *testing.T) {
	info := &EKSClusterInfo{
		Name:     "dev-cluster",
		Endpoint: "https://XXXX.gr7.us-east-2.eks.amazonaws.com",
		ARN:      "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster",
		Region:   "us-east-2",
	}

	clusterInfo, err := BuildKubeClusterInfo(info, "")

	require.NoError(t, err)
	assert.Equal(t, []string{"aws", "eks", "token", "--cluster-name", "dev-cluster", "--region", "us-east-2"}, clusterInfo.ExecArgs)
	require.Len(t, clusterInfo.ExecEnv, 1)
	workingDir, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, "ATMOS_CHDIR", clusterInfo.ExecEnv[0].Name)
	assert.Equal(t, workingDir, clusterInfo.ExecEnv[0].Value)
}

// Regression for #3674: SDK region resolution must survive in the written exec plugin.
func TestDescribeCluster_ResolvedRegion(t *testing.T) {
	for _, tt := range []struct {
		name, arn, region, want string
		wantErr                 bool
	}{
		{name: "default region", arn: "arn:aws:eks:eu-central-1:123456789012:cluster/bar", want: "eu-central-1"},
		{name: "GovCloud", arn: "arn:aws-us-gov:eks:us-gov-west-1:123456789012:cluster/bar", want: "us-gov-west-1"},
		{name: "explicit region", arn: "arn:aws:eks:eu-central-1:123456789012:cluster/bar", region: "eu-central-1", want: "eu-central-1"},
		{name: "explicit region without ARN", region: "eu-central-1", want: "eu-central-1"},
		{name: "missing ARN", wantErr: true},
		{name: "malformed ARN", arn: "invalid", wantErr: true},
		{name: "missing region", arn: "arn:aws:eks::123456789012:cluster/bar", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockEKSClient(gomock.NewController(t))
			client.EXPECT().DescribeCluster(gomock.Any(), gomock.Any()).Return(&eks.DescribeClusterOutput{
				Cluster: &ekstypes.Cluster{Arn: aws.String(tt.arn)},
			}, nil)
			info, err := DescribeCluster(context.Background(), client, "bar", tt.region)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrEKSDescribeCluster)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, info.Region)
			clusterInfo, err := BuildKubeClusterInfo(info, "foo")
			require.NoError(t, err)
			config := kube.BuildClusterConfig(clusterInfo, "")
			user := config.AuthInfos["atmos-eks-bar-"+tt.want]
			require.NotNil(t, user)
			assert.Equal(t, []string{"aws", "eks", "token", "--cluster-name", "bar", "--region", tt.want, "--identity=foo"}, user.Exec.Args)
		})
	}
}

// Regression for #3673: persist the effective directory, not a relative ATMOS_CHDIR.
func TestBuildKubeClusterInfo_WorkingDirectory(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project with spaces")
	require.NoError(t, os.Mkdir(projectDir, 0o700))
	t.Chdir(projectDir)
	t.Setenv("ATMOS_CHDIR", "relative-original-value")
	workingDir, err := os.Getwd()
	require.NoError(t, err)

	info, err := BuildKubeClusterInfo(&EKSClusterInfo{
		Name: "bar", Region: "eu-central-1", ARN: "arn:aws:eks:eu-central-1:123456789012:cluster/bar",
	}, "foo")
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "config")
	mgr, err := kube.NewKubeconfigManager(path, "")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(info, "", "merge")
	require.NoError(t, err)

	// Reading from an unrelated directory must retain the original auth context.
	t.Chdir(t.TempDir())
	config, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	user := config.AuthInfos["atmos-eks-bar-eu-central-1"]
	require.NotNil(t, user)
	env := map[string]string{}
	for _, entry := range user.Exec.Env {
		env[entry.Name] = entry.Value
	}
	assert.Equal(t, workingDir, env["ATMOS_CHDIR"])
	assert.True(t, filepath.IsAbs(env["ATMOS_CHDIR"]))
	assert.Equal(t, "foo", env["ATMOS_IDENTITY"])
}

func TestBuildKubeClusterInfo_WorkingDirectoryUnavailable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Only Linux reliably fails Getwd after removing the current directory")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.Remove(dir))

	info, err := BuildKubeClusterInfo(&EKSClusterInfo{Name: "bar", Region: "eu-central-1"}, "foo")
	require.ErrorIs(t, err, errUtils.ErrWorkingDirAccessFailed)
	assert.Nil(t, info)
}
