package security

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/securityhub"

	"github.com/cloudposse/atmos/pkg/aws/identity"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// SecurityHubAPI defines the subset of AWS Security Hub API used by this package.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_aws_clients.go -package=security
type SecurityHubAPI interface {
	GetFindings(ctx context.Context, params *securityhub.GetFindingsInput, optFns ...func(*securityhub.Options)) (*securityhub.GetFindingsOutput, error)
	GetEnabledStandards(ctx context.Context, params *securityhub.GetEnabledStandardsInput, optFns ...func(*securityhub.Options)) (*securityhub.GetEnabledStandardsOutput, error)
	DescribeStandardsControls(ctx context.Context, params *securityhub.DescribeStandardsControlsInput, optFns ...func(*securityhub.Options)) (*securityhub.DescribeStandardsControlsOutput, error)
}

// TaggingAPI defines the subset of AWS Resource Groups Tagging API used by this package.
type TaggingAPI interface {
	GetResources(ctx context.Context, params *resourcegroupstaggingapi.GetResourcesInput, optFns ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error)
}

// awsClientCache holds cached AWS service clients keyed by region.
type awsClientCache struct {
	mu            sync.Mutex
	securityHub   map[string]SecurityHubAPI
	tagging       map[string]TaggingAPI
	securityHubFn func(cfg aws.Config) SecurityHubAPI
	taggingFn     func(cfg aws.Config) TaggingAPI
}

// newAWSClientCache creates a new client cache with default factory functions.
func newAWSClientCache() *awsClientCache {
	return &awsClientCache{
		securityHub: make(map[string]SecurityHubAPI),
		tagging:     make(map[string]TaggingAPI),
		securityHubFn: func(cfg aws.Config) SecurityHubAPI {
			return securityhub.NewFromConfig(cfg)
		},
		taggingFn: func(cfg aws.Config) TaggingAPI {
			return resourcegroupstaggingapi.NewFromConfig(cfg)
		},
	}
}

// getSecurityHubClient returns a cached or new Security Hub client for the given region.
func (c *awsClientCache) getSecurityHubClient(ctx context.Context, region string) (SecurityHubAPI, error) {
	defer perf.Track(nil, "security.awsClientCache.getSecurityHubClient")()

	c.mu.Lock()
	defer c.mu.Unlock()

	if client, ok := c.securityHub[region]; ok {
		return client, nil
	}

	cfg, err := identity.LoadConfig(ctx, region, "", 0)
	if err != nil {
		return nil, err
	}

	log.Debug("Created Security Hub client", "region", region)

	client := c.securityHubFn(cfg)
	c.securityHub[region] = client
	return client, nil
}

// getTaggingClient returns a cached or new Resource Groups Tagging API client for the given region.
func (c *awsClientCache) getTaggingClient(ctx context.Context, region string) (TaggingAPI, error) {
	defer perf.Track(nil, "security.awsClientCache.getTaggingClient")()

	c.mu.Lock()
	defer c.mu.Unlock()

	if client, ok := c.tagging[region]; ok {
		return client, nil
	}

	cfg, err := identity.LoadConfig(ctx, region, "", 0)
	if err != nil {
		return nil, err
	}

	log.Debug("Created Resource Groups Tagging API client", "region", region)

	client := c.taggingFn(cfg)
	c.tagging[region] = client
	return client, nil
}
