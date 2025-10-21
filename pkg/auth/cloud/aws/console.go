package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/http"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// AWSFederationEndpoint is the AWS console federation endpoint.
	AWSFederationEndpoint = "https://signin.aws.amazon.com/federation"

	// AWSConsoleDestination is the default AWS console destination.
	AWSConsoleDestination = "https://console.aws.amazon.com/"

	// AWSMaxSessionDuration is the maximum session duration for AWS console (12 hours).
	AWSMaxSessionDuration = 12 * time.Hour

	// AWSDefaultSessionDuration is the default session duration (1 hour).
	AWSDefaultSessionDuration = 1 * time.Hour

	// AWSSigninTokenExpirationMinutes is the number of minutes a signin token remains valid (15 minutes per AWS docs).
	AWSSigninTokenExpirationMinutes = 15
)

// ConsoleURLGenerator generates AWS console federation URLs.
type ConsoleURLGenerator struct {
	httpClient http.Client
}

// NewConsoleURLGenerator creates a new ConsoleURLGenerator with the specified HTTP client.
func NewConsoleURLGenerator(httpClient http.Client) *ConsoleURLGenerator {
	defer perf.Track(nil, "aws.NewConsoleURLGenerator")()

	// Check for nil or typed-nil using reflection.
	if httpClient == nil || (reflect.ValueOf(httpClient).Kind() == reflect.Ptr && reflect.ValueOf(httpClient).IsNil()) {
		httpClient = http.NewDefaultClient(10 * time.Second)
	}

	return &ConsoleURLGenerator{
		httpClient: httpClient,
	}
}

// GetConsoleURL generates an AWS console sign-in URL using temporary credentials.
func (g *ConsoleURLGenerator) GetConsoleURL(ctx context.Context, creds types.ICredentials, options types.ConsoleURLOptions) (string, time.Duration, error) {
	defer perf.Track(nil, "aws.ConsoleURLGenerator.GetConsoleURL")()

	// Validate and extract AWS credentials.
	awsCreds, err := validateAWSCredentials(creds)
	if err != nil {
		return "", 0, err
	}

	// Determine session duration.
	duration := determineSessionDuration(options.SessionDuration)

	// Prepare session data for federation endpoint.
	sessionData, err := prepareSessionData(awsCreds)
	if err != nil {
		return "", 0, err
	}

	// Resolve destination.
	destination, err := resolveDestinationWithDefault(options.Destination)
	if err != nil {
		return "", 0, err
	}

	// Get signin token from federation endpoint.
	signinToken, err := g.getSigninToken(ctx, sessionData, duration)
	if err != nil {
		return "", 0, fmt.Errorf("failed to get signin token: %w", err)
	}

	// Build console URL.
	issuer := options.Issuer
	if issuer == "" {
		issuer = "atmos"
	}

	loginURL := fmt.Sprintf("%s?Action=login&Issuer=%s&Destination=%s&SigninToken=%s",
		AWSFederationEndpoint,
		url.QueryEscape(issuer),
		url.QueryEscape(destination),
		url.QueryEscape(signinToken),
	)

	log.Debug("Generated AWS console URL", "destination", destination, "duration", duration)

	return loginURL, duration, nil
}

// validateAWSCredentials validates and extracts AWS credentials from the interface.
func validateAWSCredentials(creds types.ICredentials) (*types.AWSCredentials, error) {
	awsCreds, ok := creds.(*types.AWSCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: expected AWS credentials, got %T", errUtils.ErrInvalidAuthConfig, creds)
	}

	if awsCreds.AccessKeyID == "" || awsCreds.SecretAccessKey == "" {
		return nil, fmt.Errorf("%w: temporary credentials required (access key and secret key)", errUtils.ErrInvalidAuthConfig)
	}

	if awsCreds.SessionToken == "" {
		return nil, fmt.Errorf("%w: session token required for console access (permanent IAM user credentials cannot be used)", errUtils.ErrInvalidAuthConfig)
	}

	return awsCreds, nil
}

// determineSessionDuration determines and validates the session duration.
func determineSessionDuration(requested time.Duration) time.Duration {
	duration := requested
	if duration == 0 {
		duration = AWSDefaultSessionDuration
	}
	if duration > AWSMaxSessionDuration {
		log.Debug("Session duration exceeds AWS maximum, capping at 12 hours", "requested", duration, "max", AWSMaxSessionDuration)
		duration = AWSMaxSessionDuration
	}
	return duration
}

// prepareSessionData creates and marshals session JSON for the federation endpoint.
func prepareSessionData(awsCreds *types.AWSCredentials) ([]byte, error) {
	sessionJSON := map[string]string{
		"sessionId":    awsCreds.AccessKeyID,
		"sessionKey":   awsCreds.SecretAccessKey,
		"sessionToken": awsCreds.SessionToken,
	}

	sessionData, err := json.Marshal(sessionJSON)
	if err != nil {
		return nil, fmt.Errorf("%v: failed to marshal session data: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	return sessionData, nil
}

// resolveDestinationWithDefault resolves the destination and applies default if empty.
func resolveDestinationWithDefault(dest string) (string, error) {
	destination, err := ResolveDestination(dest)
	if err != nil {
		return "", fmt.Errorf("failed to resolve destination: %w", err)
	}
	if destination == "" {
		destination = AWSConsoleDestination
	}
	return destination, nil
}

// getSigninToken requests a signin token from the AWS federation endpoint.
func (g *ConsoleURLGenerator) getSigninToken(ctx context.Context, sessionData []byte, duration time.Duration) (string, error) {
	defer perf.Track(nil, "aws.ConsoleURLGenerator.getSigninToken")()

	// Build federation endpoint URL for getSigninToken action.
	federationURL := fmt.Sprintf("%s?Action=getSigninToken&SessionDuration=%d&Session=%s",
		AWSFederationEndpoint,
		int(duration.Seconds()),
		url.QueryEscape(string(sessionData)),
	)

	log.Debug("Requesting signin token from AWS federation endpoint", "duration", duration)

	// Make HTTP request to federation endpoint.
	response, err := http.Get(ctx, federationURL, g.httpClient)
	if err != nil {
		return "", fmt.Errorf("%v: failed to call federation endpoint: %w", errUtils.ErrHTTPRequestFailed, err)
	}

	// Parse response to extract SigninToken.
	var result struct {
		SigninToken string `json:"SigninToken"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return "", fmt.Errorf("%v: failed to parse federation response: %w", errUtils.ErrHTTPRequestFailed, err)
	}

	if result.SigninToken == "" {
		return "", fmt.Errorf("%w: empty signin token received from federation endpoint", errUtils.ErrHTTPRequestFailed)
	}

	log.Debug("Successfully obtained signin token from AWS federation endpoint")

	return result.SigninToken, nil
}

// SupportsConsoleAccess returns true for AWS.
func (g *ConsoleURLGenerator) SupportsConsoleAccess() bool {
	defer perf.Track(nil, "aws.ConsoleURLGenerator.SupportsConsoleAccess")()

	return true
}
