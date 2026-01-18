package exec

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	log "github.com/charmbracelet/log"
	"github.com/google/go-containerregistry/pkg/authn"
)

var (
	// Static errors for ECR authentication
	errInvalidECRRegistryFormat   = errors.New("invalid ECR registry format")
	errCouldNotParseECRAccount    = errors.New("could not parse ECR account/region")
	errFailedToGetECRAuthToken    = errors.New("failed to get ECR authorization token")
	errNoECRAuthorizationData     = errors.New("no authorization data returned from ECR")
	errEmptyECRAuthorizationToken = errors.New("empty ECR authorization token")
	errFailedToDecodeECRAuthToken = errors.New("failed to decode ECR authorization token")
	errInvalidECRAuthTokenFormat  = errors.New("invalid ECR authorization token format")
	errFailedToLoadAWSConfig      = errors.New("failed to load AWS config")
)

// Precompiled: supports ecr and ecr-fips across partitions (incl. .cn).
var ecrRegistryRe = regexp.MustCompile(`^(?P<acct>\d{12})\.dkr\.(?P<svc>ecr(?:-fips)?)\.(?P<region>[a-z0-9-]+)\.amazonaws\.com(?:\.cn)?$`)

// parseECRRegistry parses ECR registry string and extracts account ID and region.
func parseECRRegistry(registry string) (accountID, region string, err error) {
	m := ecrRegistryRe.FindStringSubmatch(registry)
	if m == nil {
		return "", "", fmt.Errorf("%w: %s", errInvalidECRRegistryFormat, registry)
	}

	accountID = m[ecrRegistryRe.SubexpIndex("acct")]
	region = m[ecrRegistryRe.SubexpIndex("region")]

	if accountID == "" || region == "" {
		return "", "", fmt.Errorf("%w from %s", errCouldNotParseECRAccount, registry)
	}

	return accountID, region, nil
}

// getECRAuthToken retrieves the authorization token from ECR.
func getECRAuthToken(ctx context.Context, ecrClient *ecr.Client, accountID string) (*types.AuthorizationData, error) {
	authTokenInput := &ecr.GetAuthorizationTokenInput{
		RegistryIds: []string{accountID},
	}
	authTokenOutput, err := ecrClient.GetAuthorizationToken(ctx, authTokenInput)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errFailedToGetECRAuthToken, err)
	}
	if len(authTokenOutput.AuthorizationData) == 0 {
		return nil, fmt.Errorf("%w for account %s", errNoECRAuthorizationData, accountID)
	}
	return &authTokenOutput.AuthorizationData[0], nil
}

// parseECRCredentials decodes and parses the ECR authorization token.
func parseECRCredentials(authData *types.AuthorizationData, registry string) (username, password string, err error) {
	if authData.AuthorizationToken == nil {
		return "", "", fmt.Errorf("%w for %s", errEmptyECRAuthorizationToken, registry)
	}

	token, err := base64.StdEncoding.DecodeString(*authData.AuthorizationToken)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", errFailedToDecodeECRAuthToken, err)
	}

	parts := strings.SplitN(string(token), ":", 2)
	if len(parts) != 2 {
		return "", "", errInvalidECRAuthTokenFormat
	}

	return parts[0], parts[1], nil
}

// getECRAuth attempts to get AWS ECR authentication using AWS credentials
// Supports SSO/role providers by not gating on environment variables
// Supports both standard ECR and FIPS endpoints.
func getECRAuth(registry string) (authn.Authenticator, error) {
	accountID, region, err := parseECRRegistry(registry)
	if err != nil {
		return nil, err
	}

	log.Debug("Extracted ECR registry info", "registry", registry, "accountID", accountID, "region", region)

	// Create context with timeout to prevent hanging AWS API calls
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Load AWS config for the target region
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errFailedToLoadAWSConfig, err)
	}
	ecrClient := ecr.NewFromConfig(cfg)

	// Get ECR authorization token
	authData, err := getECRAuthToken(ctx, ecrClient, accountID)
	if err != nil {
		return nil, err
	}

	// Parse credentials from token
	username, password, err := parseECRCredentials(authData, registry)
	if err != nil {
		return nil, err
	}

	log.Debug("Successfully obtained ECR credentials", "registry", registry, "accountID", accountID, "region", region)

	return &authn.Basic{
		Username: username,
		Password: password,
	}, nil
}
