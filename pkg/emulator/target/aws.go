// Package target holds the per-target connection-profile builders for emulator
// drivers: each turns a live emulator Endpoint into a Profile (SDK env vars, a
// Terraform provider fragment, or a kubeconfig placeholder). The concrete drivers
// in pkg/emulator/driver wire these builders to driver names.
package target

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Dummy credentials accepted by AWS emulators (Floci/MiniStack/LocalStack).
// We deliberately do NOT set a session token: a bogus AWS_SESSION_TOKEN makes the
// request look like temporary STS credentials, which some emulators validate and
// reject with "UnrecognizedClientException: security token invalid". Static
// access-key/secret-key pairs (like a real IAM user) carry no session token.
const (
	awsDummyAccessKeyID     = "test"
	awsDummySecretAccessKey = "test"
	awsDefaultRegion        = "us-east-1"
)

// AWSProfile builds the connection profile for an AWS-target emulator: the SDK
// env vars (endpoint + dummy creds + region), the internal-SDK resolver URL, and
// a Terraform provider fragment carrying the behavior flags env cannot set.
func AWSProfile(ep *emu.Endpoint) emu.Profile {
	defer perf.Track(nil, "emulator.target.AWSProfile")()

	url := ep.URL("http")
	region := ep.Region
	if region == "" {
		region = awsDefaultRegion
	}

	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     awsDummyAccessKeyID,
		"AWS_SECRET_ACCESS_KEY": awsDummySecretAccessKey,
		"AWS_REGION":            region,
		"AWS_DEFAULT_REGION":    region,
	}
	if url != "" {
		env["AWS_ENDPOINT_URL"] = url
		// AWS_ENDPOINT_URL_S3 (the per-service override) is what the OpenTofu/Terraform
		// S3 *state backend* reads — the backend does not honor the generic
		// AWS_ENDPOINT_URL. Setting it here lets an emulator-backed component use an S3
		// backend with no endpoint in its config.
		env["AWS_ENDPOINT_URL_S3"] = url
	}

	// Provider fragment: the behavior flags + path-style + creds the AWS provider
	// needs against an emulator. The endpoint itself is honored from AWS_ENDPOINT_URL
	// in the subprocess env; the contributor adds what env cannot set.
	provider := map[string]any{
		"access_key":                  awsDummyAccessKeyID,
		"secret_key":                  awsDummySecretAccessKey,
		"region":                      region,
		"skip_credentials_validation": true,
		"skip_metadata_api_check":     true,
		"skip_requesting_account_id":  true,
		"s3_use_path_style":           true,
	}
	if url != "" {
		provider["endpoints"] = []map[string]any{{"s3": url}}
	}

	return emu.Profile{Env: env, ResolverURL: url, Provider: provider}
}
