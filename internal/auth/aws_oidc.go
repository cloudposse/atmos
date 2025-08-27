package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/yaml.v3"
)

// awsOidc implements LoginMethod for assuming an AWS role with an OIDC web identity token
// using STS AssumeRoleWithWebIdentity.
type awsOidc struct {
	Common          schema.ProviderDefaultConfig `yaml:",inline"`
	schema.Identity `yaml:",inline"`

	// Optional, mostly for overrides / local testing.
	Audience       string `yaml:"audience,omitempty" json:"audience,omitempty" mapstructure:"audience,omitempty"`                         // default "sts.amazonaws.com"
	SessionName    string `yaml:"session_name,omitempty" json:"session_name,omitempty" mapstructure:"session_name,omitempty"`             // default derived from GitHub envs
	STSEndpoint    string `yaml:"sts_endpoint,omitempty" json:"sts_endpoint,omitempty" mapstructure:"sts_endpoint,omitempty"`             // optional (e.g., http://localhost:4566 for LocalStack)
	ForceTokenFile string `yaml:"force_token_file,omitempty" json:"force_token_file,omitempty" mapstructure:"force_token_file,omitempty"` // optional path to a pre-fetched JWT (for local dev)
}

func NewAwsOidcFactory(provider string, identity string, config schema.AuthConfig) (LoginMethod, error) {
	data := &awsOidc{
		Identity: NewIdentity(),
	}
	b, err := yaml.Marshal(config.Providers[provider])
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(b, data)
	setDefaults(&data.Common, provider, config)
	data.Identity.Identity = identity
	return data, err
}

// Validate checks minimum required fields
func (i *awsOidc) Validate() error {
	if i.RoleArn == "" {
		return errors.New("RoleARN is required")
	}
	if i.Audience == "" {
		i.Audience = "sts.amazonaws.com"
	}
	if i.Common.Region == "" {
		i.Common.Region = "us-east-1"
	}
	if i.SessionName == "" {
		// Derive a nice session name for auditing
		if rn := os.Getenv("GITHUB_RUN_ID"); rn != "" {
			i.SessionName = fmt.Sprintf("gh-%s", rn)
		} else {
			i.SessionName = "github-actions"
		}
	}
	return nil
}

// Login resolves token source but does not contact AWS.
func (i *awsOidc) Login() error {
	ctx := context.Background()

	// 1) Fetch a GitHub OIDC JWT
	jwt, err := i.loadOIDCJWT()
	if err != nil {
		return err
	}
	baseCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(i.Common.Region),
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}

	// 2) Build STS client using endpoint-resolution v2
	var stsOpts []func(*sts.Options)
	if i.STSEndpoint != "" {
		stsOpts = append(stsOpts, func(o *sts.Options) {
			// Endpoint v2: prefer BaseEndpoint over deprecated global resolver
			o.BaseEndpoint = aws.String(i.STSEndpoint)
		})
	}
	stsClient := sts.NewFromConfig(baseCfg, stsOpts...)

	// 3) Exchange JWT → temp creds
	input := &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(i.RoleArn),
		RoleSessionName:  aws.String(i.SessionName),
		WebIdentityToken: aws.String(jwt),
	}
	if i.RequestedDuration > 0 {
		secs := int32(i.RequestedDuration.Seconds())
		input.DurationSeconds = &secs
	}
	resp, err := stsClient.AssumeRoleWithWebIdentity(ctx, input)
	if err != nil {
		return fmt.Errorf("AssumeRoleWithWebIdentity: %w", err)
	}

	WriteAwsCredentials(
		i.Common.Profile,
		aws.ToString(resp.Credentials.AccessKeyId),
		aws.ToString(resp.Credentials.SecretAccessKey),
		aws.ToString(resp.Credentials.SessionToken),
		"aws/oidc",
	)

	log.Info("✅ Successfully assumed role via OIDC",
		"role", i.RoleArn,
		"session_name", i.SessionName,
		"profile", i.Common.Profile,
		"expires", resp.Credentials.Expiration.Local().Format(time.RFC1123),
	)
	return nil
}

// AssumeRole exchanges the OIDC token for AWS credentials using STS.
func (i *awsOidc) AssumeRole() error {
	return nil
}

func (i *awsOidc) SetEnvVars(info *schema.ConfigAndStacksInfo) error {
	log.Info("Setting AWS environment variables")
	if err := SetAwsEnvVars(info, i.Identity.Identity, i.Provider, i.Common.Region); err != nil {
		return err
	}
	MergeIdentityEnvOverrides(info, i.Env)
	if err := UpdateAwsAtmosConfig(i.Provider, i.Identity.Identity, i.Common.Profile, i.Common.Region, i.RoleArn); err != nil {
		return err
	}
	return nil
}

func (i *awsOidc) Logout() error {
	return RemoveAwsCredentials(i.Common.Profile)
}

func (i *awsOidc) loadOIDCJWT() (string, error) {
	ctx := context.Background()
	if i.ForceTokenFile != "" {
		b, err := os.ReadFile(i.ForceTokenFile)
		if err != nil {
			return "", fmt.Errorf("read ForceTokenFile: %w", err)
		}
		return string(b), nil
	}
	// GitHub OIDC envs require `permissions: id-token: write`
	if u := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"); u != "" {
		reqURL, err := url.Parse(u)
		if err != nil {
			return "", fmt.Errorf("parse ACTIONS_ID_TOKEN_REQUEST_URL: %w", err)
		}
		q := reqURL.Query()
		q.Set("audience", i.Audience)
		reqURL.RawQuery = q.Encode()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
		req.Header.Set("Authorization", "bearer "+os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("request OIDC token: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("request OIDC token: unexpected status %s", resp.Status)
		}
		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return "", fmt.Errorf("decode OIDC response: %w", err)
		}
		if body.Value == "" {
			return "", errors.New("empty OIDC token in response")
		}
		return body.Value, nil
	}
	if p := os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("read AWS_WEB_IDENTITY_TOKEN_FILE: %w", err)
		}
		return string(b), nil
	}
	return "", errors.New("no OIDC token source found (set permissions: id-token: write or provide a token file)")
}

func fetchGitHubOIDCToken(ctx context.Context, audience string) (string, error) {
	reqURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	reqTok := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if reqURL == "" || reqTok == "" {
		return "", errors.New("missing ACTIONS_ID_TOKEN_REQUEST_URL/TOKEN (needs permissions: id-token: write)")
	}
	u, err := url.Parse(reqURL)
	if err != nil {
		return "", fmt.Errorf("parse ACTIONS_ID_TOKEN_REQUEST_URL: %w", err)
	}
	q := u.Query()
	q.Set("audience", audience)
	u.RawQuery = q.Encode()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("Authorization", "bearer "+reqTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request OIDC token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request OIDC token: unexpected status %s", resp.Status)
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode OIDC response: %w", err)
	}
	if body.Value == "" {
		return "", errors.New("empty OIDC token in response")
	}
	return body.Value, nil
}
