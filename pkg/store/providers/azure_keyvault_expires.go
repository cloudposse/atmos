package providers

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/store"
)

const (
	// AzureExpiresDateLayout is the date-only form accepted by the expires option (midnight UTC).
	azureExpiresDateLayout = "2006-01-02"
	// AzureExpiresDaySuffix marks a duration expressed in days (for example "90d").
	azureExpiresDaySuffix = "d"
	// AzureExpiresAcceptedForms lists the accepted expires forms for error messages.
	azureExpiresAcceptedForms = "an RFC 3339 timestamp (2027-01-01T00:00:00Z), a date (2027-01-01), or a positive duration (90d, 2160h, 720h30m)"
	hoursPerDay               = 24
	decimalBase               = 10
	int64Bits                 = 64
)

// resolveAzureExpires parses the optional expires option and warns when a fixed expiry is already in the past.
// A nil option means no expiry is applied.
func resolveAzureExpires(expires *string) (*time.Time, time.Duration, error) {
	if expires == nil {
		return nil, 0, nil
	}

	expiresAt, expiresIn, err := parseAzureExpires(*expires)
	if err != nil {
		return nil, 0, err
	}
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		log.Warn("Azure Key Vault store expires option is in the past; secret writes may be rejected", "expires", *expires)
	}

	return expiresAt, expiresIn, nil
}

// parseAzureExpires parses the value of the expires option. It returns either a fixed expiry time
// (RFC 3339 timestamp or YYYY-MM-DD date at midnight UTC) or a positive duration that is applied
// relative to the time of each write. Durations use Go syntax plus a "d" day suffix (for example "90d").
func parseAzureExpires(value string) (*time.Time, time.Duration, error) {
	value = strings.TrimSpace(value)

	if t, err := time.Parse(time.RFC3339, value); err == nil {
		t = t.UTC()
		return &t, 0, nil
	}
	if t, err := time.Parse(azureExpiresDateLayout, value); err == nil {
		return &t, 0, nil
	}

	d, ok := parseAzureDuration(value)
	if !ok {
		return nil, 0, fmt.Errorf("%w: %q is not %s", store.ErrInvalidExpires, value, azureExpiresAcceptedForms)
	}
	if d <= 0 {
		return nil, 0, fmt.Errorf("%w: duration %q must be positive", store.ErrInvalidExpires, value)
	}

	return nil, d, nil
}

// parseAzureDuration parses a Go duration, additionally accepting a whole number of days with a "d" suffix.
func parseAzureDuration(value string) (time.Duration, bool) {
	if days, found := strings.CutSuffix(value, azureExpiresDaySuffix); found {
		n, err := strconv.ParseUint(days, decimalBase, int64Bits)
		if err != nil || n > math.MaxInt64/uint64(hoursPerDay*time.Hour) {
			return 0, false
		}
		return time.Duration(n) * hoursPerDay * time.Hour, true
	}

	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, false
	}

	return d, true
}

// azureTagsToPointers converts configured tags to the pointer map the Azure SDK expects.
// It returns nil when no tags are configured, and never aliases the caller's map.
func azureTagsToPointers(tags map[string]string) map[string]*string {
	if len(tags) == 0 {
		return nil
	}

	out := make(map[string]*string, len(tags))
	for k, v := range tags {
		out[k] = &v
	}

	return out
}

// secretParameters builds the SetSecret parameters for value, applying the configured tags and expiry.
// Tags are copied on every call so the SDK can never mutate store state, and a relative expiry is
// recomputed from the current time so it does not go stale.
func (s *AzureKeyVaultStore) secretParameters(value *string) azsecrets.SetSecretParameters {
	params := azsecrets.SetSecretParameters{Value: value}

	if len(s.tags) > 0 {
		params.Tags = make(map[string]*string, len(s.tags))
		for k, v := range s.tags {
			tag := *v
			params.Tags[k] = &tag
		}
	}

	switch {
	case s.expiresAt != nil:
		expires := *s.expiresAt
		params.SecretAttributes = &azsecrets.SecretAttributes{Expires: &expires}
	case s.expiresIn > 0:
		expires := time.Now().UTC().Add(s.expiresIn)
		params.SecretAttributes = &azsecrets.SecretAttributes{Expires: &expires}
	}

	return params
}
