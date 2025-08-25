package exec

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	log "github.com/charmbracelet/log"
	"github.com/google/go-containerregistry/pkg/authn"
)

// getECRAuth attempts to get AWS ECR authentication using AWS credentials
// Supports SSO/role providers by not gating on environment variables
// Supports both standard ECR and FIPS endpoints
func getECRAuth(registry string) (authn.Authenticator, error) {
	// Use regex to match both standard and FIPS ECR endpoints
	// Supports: <account>.dkr.ecr.<region>.amazonaws.com[.cn]
	// Supports: <account>.dkr.ecr-fips.<region>.amazonaws.com[.cn]
	re := regexp.MustCompile(`^(?P<acct>\d{12})\.dkr\.(?P<svc>ecr(?:-fips)?)\.(?P<region>[a-z0-9-]+)\.amazonaws\.com(?:\.cn)?$`)
	m := re.FindStringSubmatch(registry)
	if m == nil {
		return nil, fmt.Errorf("invalid ECR registry format: %s", registry)
	}

	accountID := m[re.SubexpIndex("acct")]
	region := m[re.SubexpIndex("region")]

	if accountID == "" || region == "" {
		return nil, fmt.Errorf("could not parse ECR account/region from %s", registry)
	}

	log.Debug("Extracted ECR registry info", "registry", registry, "accountID", accountID, "region", region)

	// Create context with timeout to prevent hanging AWS API calls
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Load AWS config for the target region
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	ecrClient := ecr.NewFromConfig(cfg)

	// Get ECR authorization token for the target account
	authTokenInput := &ecr.GetAuthorizationTokenInput{
		RegistryIds: []string{accountID},
	}
	authTokenOutput, err := ecrClient.GetAuthorizationToken(ctx, authTokenInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECR authorization token: %w", err)
	}
	if len(authTokenOutput.AuthorizationData) == 0 {
		return nil, fmt.Errorf("no authorization data returned from ECR")
	}

	// Prefer the entry whose ProxyEndpoint matches the target registry
	authData := authTokenOutput.AuthorizationData[0]
	for i := range authTokenOutput.AuthorizationData {
		ad := authTokenOutput.AuthorizationData[i]
		if ad.ProxyEndpoint != nil && strings.Contains(*ad.ProxyEndpoint, registry) {
			authData = ad
			break
		}
	}

	// Nil-safe token decoding
	if authData.AuthorizationToken == nil {
		return nil, fmt.Errorf("empty ECR authorization token for %s", registry)
	}

	token, err := base64.StdEncoding.DecodeString(*authData.AuthorizationToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ECR authorization token: %w", err)
	}

	// Parse username:password from token
	parts := strings.SplitN(string(token), ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid ECR authorization token format")
	}

	username := parts[0]
	password := parts[1]

	log.Debug("Successfully obtained ECR credentials", "registry", registry, "accountID", accountID, "region", region)

	return &authn.Basic{
		Username: username,
		Password: password,
	}, nil
}
