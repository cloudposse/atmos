package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	smithymiddleware "github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// eksTokenPrefix is the standard prefix for EKS bearer tokens.
	eksTokenPrefix = "k8s-aws-v1."

	// eksClusterIDHeader is the header used to identify the EKS cluster for token generation.
	eksClusterIDHeader = "x-k8s-aws-id"

	// eksTokenExpiry is the default token lifetime for EKS tokens.
	eksTokenExpiry = 14 * time.Minute

	// eksPresignLifetimeSeconds is the presigned URL lifetime (60 seconds).
	eksPresignLifetimeSeconds = 60
)

// EKSClusterInfo contains cluster data needed for kubeconfig generation.
type EKSClusterInfo struct {
	Name                     string
	Endpoint                 string
	CertificateAuthorityData string
	ARN                      string
	Region                   string
}

// EKSClient defines the interface for EKS API calls (for testability).
type EKSClient interface {
	DescribeCluster(ctx context.Context, input *eks.DescribeClusterInput, opts ...func(*eks.Options)) (*eks.DescribeClusterOutput, error)
}

// NewEKSClient creates a new EKS client from Atmos credentials.
func NewEKSClient(ctx context.Context, creds types.ICredentials, region string) (EKSClient, error) {
	defer perf.Track(nil, "aws.NewEKSClient")()

	cfg, err := BuildAWSConfigFromCreds(ctx, creds, region)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrEKSDescribeCluster, err)
	}

	return eks.NewFromConfig(cfg), nil
}

// DescribeCluster retrieves cluster information needed for kubeconfig.
func DescribeCluster(ctx context.Context, client EKSClient, clusterName, region string) (*EKSClusterInfo, error) {
	defer perf.Track(nil, "aws.DescribeCluster")()

	output, err := client.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: aws.String(clusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrEKSDescribeCluster, err)
	}

	if output.Cluster == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrEKSClusterNotFound, clusterName)
	}

	var caData string
	if output.Cluster.CertificateAuthority != nil {
		caData = aws.ToString(output.Cluster.CertificateAuthority.Data)
	}

	return &EKSClusterInfo{
		Name:                     clusterName,
		Endpoint:                 aws.ToString(output.Cluster.Endpoint),
		CertificateAuthorityData: caData,
		ARN:                      aws.ToString(output.Cluster.Arn),
		Region:                   region,
	}, nil
}

// GetToken generates an EKS bearer token via STS pre-signed GetCallerIdentity URL.
// This follows the same approach as `aws eks get-token` / aws-iam-authenticator.
func GetToken(ctx context.Context, creds types.ICredentials, clusterName, region string) (string, time.Time, error) {
	defer perf.Track(nil, "aws.GetToken")()

	cfg, err := BuildAWSConfigFromCreds(ctx, creds, region)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: %w", errUtils.ErrEKSTokenGeneration, err)
	}

	stsClient := sts.NewFromConfig(cfg)

	// Create STS presign client with cluster ID header and URL expiry injected before signing.
	// The x-k8s-aws-id header binds the token to the cluster.
	// X-Amz-Expires=60 matches aws-iam-authenticator / `aws eks get-token` behavior.
	presignClient := sts.NewPresignClient(stsClient, func(po *sts.PresignOptions) {
		po.ClientOptions = append(po.ClientOptions, func(o *sts.Options) {
			o.APIOptions = append(o.APIOptions,
				smithyhttp.SetHeaderValue(eksClusterIDHeader, clusterName),
				addPresignExpiry(eksPresignLifetimeSeconds),
			)
		})
	})

	// Presign GetCallerIdentity request.
	presignedReq, err := presignClient.PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: failed to presign GetCallerIdentity: %w", errUtils.ErrEKSTokenGeneration, err)
	}

	log.Debug("Presigned URL generated", "url_length", len(presignedReq.URL))

	// Encode as base64url (no padding) with k8s-aws-v1 prefix.
	token := eksTokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(presignedReq.URL))

	// Token expires based on presigned URL lifetime.
	expiresAt := time.Now().Add(eksTokenExpiry)

	return token, expiresAt, nil
}

// addPresignExpiry returns a middleware that adds X-Amz-Expires to the presigned URL query string.
// This is required for EKS token validation - matching aws-iam-authenticator behavior.
// The AWS SDK v2 does not set X-Amz-Expires automatically for presigned requests.
func addPresignExpiry(expirySeconds int) func(*smithymiddleware.Stack) error {
	return func(stack *smithymiddleware.Stack) error {
		return stack.Build.Add(smithymiddleware.BuildMiddlewareFunc(
			"AddPresignExpiry",
			func(ctx context.Context, input smithymiddleware.BuildInput, next smithymiddleware.BuildHandler) (smithymiddleware.BuildOutput, smithymiddleware.Metadata, error) {
				req, ok := input.Request.(*smithyhttp.Request)
				if ok {
					query := req.URL.Query()
					query.Set("X-Amz-Expires", strconv.Itoa(expirySeconds))
					req.URL.RawQuery = query.Encode()
				}
				return next.HandleBuild(ctx, input)
			},
		), smithymiddleware.Before)
	}
}
