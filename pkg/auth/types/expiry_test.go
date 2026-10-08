package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExpiresWithin(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		name   string
		creds  ICredentials
		within time.Duration
		want   bool
	}{
		{name: "nil credentials are treated as expiring", creds: nil, within: time.Hour, want: true},
		{name: "no expiration never expires", creds: &AWSCredentials{}, within: time.Hour, want: false},
		{name: "unparseable expiration always expires", creds: &AWSCredentials{Expiration: "soon"}, within: 0, want: true},
		{name: "expired", creds: &AWSCredentials{Expiration: now.Add(-time.Minute).Format(time.RFC3339)}, within: 0, want: true},
		{name: "valid beyond the window", creds: &AWSCredentials{Expiration: now.Add(20 * time.Minute).Format(time.RFC3339)}, within: 15 * time.Minute, want: false},
		{name: "valid but inside the window", creds: &AWSCredentials{Expiration: now.Add(20 * time.Minute).Format(time.RFC3339)}, within: 30 * time.Minute, want: true},
		{name: "zero window on a valid credential", creds: &AWSCredentials{Expiration: now.Add(time.Minute).Format(time.RFC3339)}, within: 0, want: false},
		{name: "works for non-AWS credentials inside the window", creds: &AzureCredentials{Expiration: now.Add(5 * time.Minute).Format(time.RFC3339)}, within: 10 * time.Minute, want: true},
		{name: "works for non-AWS credentials beyond the window", creds: &AzureCredentials{Expiration: now.Add(time.Hour).Format(time.RFC3339)}, within: 10 * time.Minute, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ExpiresWithin(tc.creds, tc.within))
			if tc.creds != nil && tc.within == 0 {
				assert.Equal(t, tc.creds.IsExpired(), ExpiresWithin(tc.creds, 0))
			}
		})
	}
}
