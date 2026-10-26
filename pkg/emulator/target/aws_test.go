package target

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

func TestAWSProfile_Branches(t *testing.T) {
	cases := map[string]struct {
		ep             *emu.Endpoint
		wantRegion     string
		wantURL        string
		wantEndpointKV bool
	}{
		"bound port and explicit region": {
			ep:             &emu.Endpoint{Target: emu.TargetAWS, Host: "localhost", Ports: map[int]int{4566: 54321}, Region: "eu-west-1"},
			wantRegion:     "eu-west-1",
			wantURL:        "http://127.0.0.1:54321",
			wantEndpointKV: true,
		},
		"bound port and default region fallback": {
			ep:             &emu.Endpoint{Target: emu.TargetAWS, Host: "localhost", Ports: map[int]int{4566: 4566}},
			wantRegion:     awsDefaultRegion,
			wantURL:        "http://127.0.0.1:4566",
			wantEndpointKV: true,
		},
		"no bound port omits endpoint keys": {
			ep:             &emu.Endpoint{Target: emu.TargetAWS, Host: "localhost", Ports: map[int]int{}, Region: "us-west-2"},
			wantRegion:     "us-west-2",
			wantURL:        "",
			wantEndpointKV: false,
		},
		"nil ports omits endpoint keys with default region": {
			ep:             &emu.Endpoint{Target: emu.TargetAWS, Host: "localhost"},
			wantRegion:     awsDefaultRegion,
			wantURL:        "",
			wantEndpointKV: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := AWSProfile(tc.ep)

			// Dummy static creds (no session token) are always present.
			assert.Equal(t, awsDummyAccessKeyID, p.Env["AWS_ACCESS_KEY_ID"])
			assert.Equal(t, awsDummySecretAccessKey, p.Env["AWS_SECRET_ACCESS_KEY"])
			assert.NotContains(t, p.Env, "AWS_SESSION_TOKEN", "no session token: emulators reject bogus STS tokens")

			// Region appears in both AWS_REGION and AWS_DEFAULT_REGION.
			assert.Equal(t, tc.wantRegion, p.Env["AWS_REGION"])
			assert.Equal(t, tc.wantRegion, p.Env["AWS_DEFAULT_REGION"])

			// ResolverURL mirrors the primary URL in every case.
			assert.Equal(t, tc.wantURL, p.ResolverURL)

			if tc.wantEndpointKV {
				assert.Equal(t, tc.wantURL, p.Env["AWS_ENDPOINT_URL"])
				assert.Equal(t, tc.wantURL, p.Env["AWS_ENDPOINT_URL_S3"], "per-service S3 endpoint for the state backend")
			} else {
				assert.NotContains(t, p.Env, "AWS_ENDPOINT_URL")
				assert.NotContains(t, p.Env, "AWS_ENDPOINT_URL_S3")
			}

			// Provider fragment is identical regardless of port binding.
			require.NotNil(t, p.Provider)
			assert.Equal(t, awsDummyAccessKeyID, p.Provider["access_key"])
			assert.Equal(t, awsDummySecretAccessKey, p.Provider["secret_key"])
			assert.Equal(t, tc.wantRegion, p.Provider["region"])
			assert.Equal(t, true, p.Provider["skip_credentials_validation"])
			assert.Equal(t, true, p.Provider["skip_metadata_api_check"])
			assert.Equal(t, true, p.Provider["skip_requesting_account_id"])
			assert.Equal(t, true, p.Provider["s3_use_path_style"])
			if tc.wantEndpointKV {
				assert.Equal(t, []map[string]any{{"s3": tc.wantURL}}, p.Provider["endpoints"])
			} else {
				assert.NotContains(t, p.Provider, "endpoints")
			}
		})
	}
}
