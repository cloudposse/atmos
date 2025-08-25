package exec

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	log "github.com/charmbracelet/log"
	"github.com/google/go-containerregistry/pkg/authn"
)

// getECRAuth attempts to get AWS ECR authentication using AWS credentials
// Supports SSO/role providers by not gating on environment variables
func getECRAuth(registry string) (authn.Authenticator, error) {
	// Parse <account>.dkr.ecr.<region>.amazonaws.com[.cn]
	parts := strings.Split(registry, ".")
	if len(parts) < 6 {
		return nil, fmt.Errorf("invalid ECR registry format: %s", registry)
	}
	accountID := parts[0]
	// Region follows the "ecr" label: <acct>.dkr.ecr.<region>.amazonaws.com[.cn]
	region := ""
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "ecr" {
			region = parts[i+1]
			break
		}
	}
	if accountID == "" || region == "" {
		return nil, fmt.Errorf("could not parse ECR account/region from %s", registry)
	}

	log.Debug("Extracted ECR registry info", "registry", registry, "accountID", accountID, "region", region)

	// Load AWS config for the target region
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	ecrClient := ecr.NewFromConfig(cfg)

	// Get ECR authorization token for the target account
	authTokenInput := &ecr.GetAuthorizationTokenInput{
		RegistryIds: []string{accountID},
	}
	authTokenOutput, err := ecrClient.GetAuthorizationToken(context.Background(), authTokenInput)
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
	token, err := base64.StdEncoding.DecodeString(*authData.AuthorizationToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ECR authorization token: %w", err)
	}

	// Parse username:password from token
	parts = strings.SplitN(string(token), ":", 2)
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
