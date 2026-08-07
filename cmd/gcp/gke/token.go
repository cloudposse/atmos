package gke

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	gcpCloud "github.com/cloudposse/atmos/pkg/auth/cloud/gcp"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const execCredentialAPIVersion = "client.authentication.k8s.io/v1beta1"

var (
	initCliConfigFn        = cfg.InitCliConfig
	authenticateForTokenFn = authenticateForToken
	getGKETokenFn          = gcpCloud.GetToken
	newAuthManagerFn       = auth.NewAuthManager
)

var tokenCmd = &cobra.Command{
	Use:          "token",
	Short:        "Generate a GKE bearer token for kubectl",
	Long:         "Generate a Kubernetes ExecCredential from an Atmos-managed GCP identity. This command is normally invoked by kubectl from an Atmos-generated kubeconfig.",
	Args:         cobra.NoArgs,
	RunE:         executeTokenCommand,
	SilenceUsage: true,
}

type execCredential struct {
	APIVersion string               `json:"apiVersion"`
	Kind       string               `json:"kind"`
	Status     execCredentialStatus `json:"status"`
}

type execCredentialStatus struct {
	ExpirationTimestamp string `json:"expirationTimestamp,omitempty"`
	Token               string `json:"token"`
}

func executeTokenCommand(cmd *cobra.Command, _ []string) error {
	atmosConfig, err := initCliConfigFn(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}
	defer perf.Track(&atmosConfig, "gke.executeTokenCommand")()

	identityName := resolveIdentity(cmd)
	ctx := auth.ContextWithSkipIntegrations(context.Background())
	creds, err := authenticateForTokenFn(ctx, &atmosConfig.Auth, atmosConfig.CliConfigPath, identityName)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrGKETokenGeneration, err)
	}
	token, expiresAt, err := getGKETokenFn(creds)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrGKETokenGeneration, err)
	}
	return writeExecCredential(token, expiresAt)
}

func writeExecCredential(token string, expiresAt time.Time) error {
	status := execCredentialStatus{Token: token}
	if !expiresAt.IsZero() {
		status.ExpirationTimestamp = expiresAt.UTC().Format(time.RFC3339)
	}
	payload, err := json.Marshal(execCredential{
		APIVersion: execCredentialAPIVersion,
		Kind:       "ExecCredential",
		Status:     status,
	})
	if err != nil {
		return fmt.Errorf("%w: failed to marshal ExecCredential: %w", errUtils.ErrGKETokenGeneration, err)
	}
	return data.Write(string(payload))
}

func resolveIdentity(cmd *cobra.Command) string {
	identityName, _ := cmd.Flags().GetString("identity")
	if identityName != "" {
		return identityName
	}
	return os.Getenv("ATMOS_IDENTITY") //nolint:forbidigo // Exec plugins inherit this explicit identity selector.
}

func authenticateForToken(ctx context.Context, authConfig *schema.AuthConfig, cliConfigPath, identityName string) (types.ICredentials, error) {
	authStackInfo := &schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{}}
	mgr, err := newAuthManagerFn(
		authConfig,
		credentials.NewCredentialStoreWithConfig(authConfig),
		validation.NewValidator(),
		authStackInfo,
		cliConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}
	if identityName == "" {
		identityName = resolveDefaultIdentity(authConfig)
		if identityName == "" {
			return nil, fmt.Errorf("%w: no identity specified and no default identity found", errUtils.ErrGKETokenGeneration)
		}
	}
	whoami, err := mgr.Authenticate(ctx, identityName)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, err)
	}
	if whoami.Credentials == nil {
		return nil, fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, errUtils.ErrIdentityCredentialsNone)
	}
	if _, ok := whoami.Credentials.(*types.GCPCredentials); !ok {
		return nil, fmt.Errorf("%w: identity %q returned non-GCP credentials", errUtils.ErrGKETokenGeneration, identityName)
	}
	return whoami.Credentials, nil
}

func resolveDefaultIdentity(authConfig *schema.AuthConfig) string {
	if authConfig == nil || len(authConfig.Identities) != 1 {
		return ""
	}
	for name := range authConfig.Identities {
		return name
	}
	return ""
}

func init() {
	tokenCmd.Flags().StringP("identity", "i", "", "Atmos GCP identity to authenticate with")
	GkeCmd.AddCommand(tokenCmd)
}
