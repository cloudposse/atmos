package azure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateSubscriptionsInProfile_EmptySubscriptionKeepsDefault verifies that an
// update without a subscription ID (for example a provider configured with only a
// tenant) leaves the user's existing default az subscription untouched instead of
// clearing isDefault on every entry.
func TestUpdateSubscriptionsInProfile_EmptySubscriptionKeepsDefault(t *testing.T) {
	profile := map[string]interface{}{
		"subscriptions": []interface{}{
			map[string]interface{}{"id": "user-default-sub", "isDefault": true, "tenantId": "user-tenant"},
			map[string]interface{}{"id": "other-sub", "isDefault": false, "tenantId": "user-tenant"},
		},
	}

	subs := UpdateSubscriptionsInProfile(profile, ProfileUpdateParams{
		Username: "user@example.com",
		TenantID: "atmos-tenant",
	})

	require.Len(t, subs, 2)
	first, ok := subs[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "user-default-sub", first["id"])
	assert.Equal(t, true, first["isDefault"], "the user's default subscription must stay the default")
	assert.Equal(t, "user-tenant", first["tenantId"], "the user's entry must not be rewritten")
	last, ok := subs[1].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "other-sub", last["id"])
	assert.Equal(t, false, last["isDefault"])
}

// TestUpdateSubscriptionsInProfile_EmptySubscriptionDoesNotClaimBlankEntry verifies an
// entry with a blank id is not treated as the Atmos subscription when none is set.
func TestUpdateSubscriptionsInProfile_EmptySubscriptionDoesNotClaimBlankEntry(t *testing.T) {
	profile := map[string]interface{}{
		"subscriptions": []interface{}{
			map[string]interface{}{"id": "", "isDefault": false, "tenantId": "user-tenant"},
		},
	}

	subs := UpdateSubscriptionsInProfile(profile, ProfileUpdateParams{TenantID: "atmos-tenant"})

	require.Len(t, subs, 1)
	entry, ok := subs[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, entry["isDefault"])
	assert.Equal(t, "user-tenant", entry["tenantId"])
}

// TestUpdateSubscriptionsInProfile_SetSubscriptionBecomesDefault is the negative-path
// counterpart: with a subscription ID, that entry becomes the default and others do not.
func TestUpdateSubscriptionsInProfile_SetSubscriptionBecomesDefault(t *testing.T) {
	profile := map[string]interface{}{
		"subscriptions": []interface{}{
			map[string]interface{}{"id": "user-default-sub", "isDefault": true},
		},
	}

	subs := UpdateSubscriptionsInProfile(profile, ProfileUpdateParams{
		Username:       "user@example.com",
		TenantID:       "atmos-tenant",
		SubscriptionID: "atmos-sub",
	})

	require.Len(t, subs, 2)
	first, ok := subs[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, first["isDefault"])
	last, ok := subs[1].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "atmos-sub", last["id"])
	assert.Equal(t, true, last["isDefault"])
}
