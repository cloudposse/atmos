package auth

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/charmbracelet/log"
	"time"
)

func SsoSync(startUrl, region string) *ssooidc.CreateTokenOutput {
	tokenOut, err := SsoSyncE(startUrl, region)
	if err != nil {
		log.Error(err)
	}
	return tokenOut
}

func SsoSyncE(startUrl, region string) (*ssooidc.CreateTokenOutput, error) {
	log.Debug("Syncing with SSO", "startUrl", startUrl, "region", region)
	// 1. Load config for SSO region
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	oidc := ssooidc.NewFromConfig(cfg)

	// 2. Register client
	regOut, err := oidc.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String("atmos-sso"),
		ClientType: aws.String("public"),
	})
	if err != nil {
		err = fmt.Errorf("failed to register client: %w", err)
		log.Error(err)
		return nil, err
	}

	// 3. Start device authorization
	authOut, err := oidc.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     regOut.ClientId,
		ClientSecret: regOut.ClientSecret,
		StartUrl:     aws.String(startUrl),
	})
	if err != nil {
		err = fmt.Errorf("start device auth failed: %w", err)
		log.Error(err)
		return nil, err
	}

	fmt.Printf("🔐 Please visit %s and enter code: %s\n", *authOut.VerificationUriComplete, *authOut.UserCode)

	// 4. Poll for token
	var tokenOut *ssooidc.CreateTokenOutput
	for {
		tokenOut, err = oidc.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     regOut.ClientId,
			ClientSecret: regOut.ClientSecret,
			DeviceCode:   authOut.DeviceCode,
			GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
		})

		if err == nil {
			break // success
		}

		var authPending *ssooidctypes.AuthorizationPendingException
		var slowDown *ssooidctypes.SlowDownException

		switch {
		case errors.As(err, &authPending):
			// Keep polling — user hasn't logged in yet
			time.Sleep(time.Duration(authOut.Interval) * time.Second)
		case errors.As(err, &slowDown):
			// AWS asked us to slow down
			time.Sleep(time.Duration(authOut.Interval+2) * time.Second)
		default:
			return nil, err
		}
	}

	return tokenOut, nil
}
