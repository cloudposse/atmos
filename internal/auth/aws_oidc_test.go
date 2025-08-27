package auth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeSTS implements stsAPI for unit testing.
type fakeSTS struct {
	Resp *sts.AssumeRoleWithWebIdentityOutput
	Err  error
}

func (f *fakeSTS) AssumeRoleWithWebIdentity(ctx context.Context, in *sts.AssumeRoleWithWebIdentityInput, _ ...func(*sts.Options)) (*sts.AssumeRoleWithWebIdentityOutput, error) {
	return f.Resp, f.Err
}

func TestAwsOidc_Validate_Login_AssumeRole_SetEnv(t *testing.T) {
	// Isolate HOME and AWS files
	tmp := t.TempDir()
	credFile := filepath.Join(tmp, ".aws", "credentials")
	cfgFile := filepath.Join(tmp, ".aws", "config")
	t.Setenv("HOME", tmp)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)
	t.Setenv("AWS_CONFIG_FILE", cfgFile)

	// Prepare token file
	tokenPath := filepath.Join(tmp, "token.jwt")
	if err := os.WriteFile(tokenPath, []byte("header.payload.signature"), 0o644); err != nil {
		t.Fatalf("write token: %v", err)
	}

	i := &awsOidc{}
	i.Common.Region = "us-east-1"
	i.Common.Profile = "src-prof"       // source profile name used for credentials file section
	i.Identity.Identity = "ci-identity" // identity name -> becomes AWS_PROFILE in env
	i.Provider = "aws/oidc"
	i.RoleArn = "arn:aws:iam::111111111111:role/Test"
	i.SessionDuration = 900
	i.WebIdentityTokenFile = tokenPath

	if err := i.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := i.Login(); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if i.WebIdentityToken == "" {
		t.Fatalf("expected token loaded from file")
	}

	// Fake STS returns static creds
	expires := time.Now().Add(1 * time.Hour).UTC()
	i.stsClient = &fakeSTS{Resp: &sts.AssumeRoleWithWebIdentityOutput{
		Credentials: &sts.Credentials{
			AccessKeyId:     aws.String("ASIAUNITTEST"),
			SecretAccessKey: aws.String("secret"),
			SessionToken:    aws.String("token"),
			Expiration:      aws.Time(expires),
		},
	}}
	if err := i.AssumeRole(); err != nil {
		// include wrapped error for easier debugging
		t.Fatalf("AssumeRole: %v", err)
	}

	// Verify credentials written
	b, err := os.ReadFile(credFile)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "["+i.Common.Profile+"]") || !strings.Contains(content, "aws_session_token") {
		t.Fatalf("expected credentials for profile %q written, got:\n%s", i.Common.Profile, content)
	}

	// Verify env setup writes AWS_PROFILE = identity
	info := &schema.ConfigAndStacksInfo{ComponentEnvSection: schema.AtmosSectionMapType{}}
	if err := i.SetEnvVars(info); err != nil {
		t.Fatalf("SetEnvVars: %v", err)
	}
	if got := info.ComponentEnvSection["AWS_PROFILE"]; got != i.Identity.Identity {
		t.Fatalf("expected AWS_PROFILE %q, got %v", i.Identity.Identity, got)
	}
	if got := info.ComponentEnvSection["AWS_REGION"]; got != i.Common.Region {
		t.Fatalf("expected AWS_REGION %q, got %v", i.Common.Region, got)
	}
}
