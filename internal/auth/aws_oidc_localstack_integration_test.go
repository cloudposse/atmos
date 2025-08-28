//go:build integration_auth_localstack
// +build integration_auth_localstack

package auth

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/cloudposse/atmos/pkg/schema"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startLocalstack(t *testing.T) (tc.Container, string) {
	t.Helper()
	ctx := context.Background()
	req := tc.ContainerRequest{
		Image:        "localstack/localstack:latest",
		ExposedPorts: []string{"4566/tcp"},
		Env:          map[string]string{"SERVICES": "iam,sts"},
		WaitingFor:   wait.ForListeningPort("4566/tcp").WithStartupTimeout(120 * time.Second),
	}
	c, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Fatalf("start localstack: %v", err)
	}
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	port, err := c.MappedPort(ctx, "4566/tcp")
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	return c, fmt.Sprintf("http://%s:%s", host, port.Port())
}

func TestAwsOidc_Login_Localstack(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found")
	}
	ctx := context.Background()
	c, endpoint := startLocalstack(t)
	t.Cleanup(func() { _ = c.Terminate(ctx) })

	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, opts ...any) (aws.Endpoint, error) {
		return aws.Endpoint{URL: endpoint, SigningRegion: region}, nil
	})
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(resolver),
		config.WithCredentialsProvider(aws.StaticCredentialsProvider{Value: aws.Credentials{
			AccessKeyID:     "test",
			SecretAccessKey: "test",
			SessionToken:    "test",
			Source:          "unit",
		}}),
	)
	if err != nil {
		t.Fatalf("load cfg: %v", err)
	}
	iamClient := iam.NewFromConfig(cfg)
	_, err = iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String("TestRole"),
		AssumeRolePolicyDocument: aws.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Federated":"*"},"Action":"sts:AssumeRoleWithWebIdentity"}]}`),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	tokenPath := filepath.Join(t.TempDir(), "token.jwt")
	if err := os.WriteFile(tokenPath, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	oidc := &awsOidc{
		Common:         schema.ProviderDefaultConfig{Region: "us-east-1", Profile: "test"},
		Identity:       schema.Identity{Identity: "id"},
		RoleArn:        "arn:aws:iam::000000000000:role/TestRole",
		STSEndpoint:    endpoint,
		ForceTokenFile: tokenPath,
	}
	if err := oidc.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := oidc.Login(); err != nil {
		t.Fatalf("Login: %v", err)
	}
}
