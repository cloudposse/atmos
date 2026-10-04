package azure

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// fakeDoer is an injectable httpDoer that records requests and replays a scripted handler.
type fakeDoer struct {
	handler  func(*http.Request) (*http.Response, error)
	requests []*http.Request
	bodies   []string
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, req)
	body := ""
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		body = string(b)
	}
	f.bodies = append(f.bodies, body)
	return f.handler(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func newClientWithDoer(doer httpDoer) *armPIMClient {
	return newARMPIMClient(doer, "test-token", testScope, "https://management.azure.com")
}

func TestARMPIMClient_ActiveAssignmentExists(t *testing.T) {
	body := `{"value":[{"name":"a","properties":{"roleDefinitionId":"` + testRoleDefID + `","status":"Provisioned"}}]}`
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, body), nil
	}}
	c := newClientWithDoer(doer)

	active, err := c.ActiveAssignmentExists(context.Background(), testRoleDefID)
	require.NoError(t, err)
	assert.True(t, active)

	// Request shape: GET, scoped collection, api-version, asTarget filter, bearer auth.
	req := doer.requests[0]
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Contains(t, req.URL.Path, "/providers/Microsoft.Authorization/roleAssignmentScheduleInstances")
	assert.Equal(t, "2020-10-01", req.URL.Query().Get("api-version"))
	assert.Equal(t, "asTarget()", req.URL.Query().Get("$filter"))
	assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
}

func TestARMPIMClient_ActiveAssignmentExists_NoMatch(t *testing.T) {
	body := `{"value":[{"name":"a","properties":{"roleDefinitionId":"/providers/Microsoft.Authorization/roleDefinitions/other","status":"Provisioned"}}]}`
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, body), nil
	}}
	active, err := newClientWithDoer(doer).ActiveAssignmentExists(context.Background(), testRoleDefID)
	require.NoError(t, err)
	assert.False(t, active)
}

func TestARMPIMClient_FindEligibility(t *testing.T) {
	body := `{"value":[{"name":"e","properties":{"roleDefinitionId":"` + testRoleDefID + `","roleEligibilityScheduleId":"` + testEligID + `"}}]}`
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, body), nil
	}}
	c := newClientWithDoer(doer)

	schedID, found, err := c.FindEligibility(context.Background(), testRoleDefID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, testEligID, schedID)
	assert.Contains(t, doer.requests[0].URL.Path, "roleEligibilityScheduleInstances")
}

func TestARMPIMClient_FindEligibility_NotFound(t *testing.T) {
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"value":[]}`), nil
	}}
	_, found, err := newClientWithDoer(doer).FindEligibility(context.Background(), testRoleDefID)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestARMPIMClient_FindPendingRequest(t *testing.T) {
	// Only the SelfActivate + pending item matches; a Provisioned one and an Admin one are ignored.
	body := `{"value":[
		{"name":"done","properties":{"roleDefinitionId":"` + testRoleDefID + `","requestType":"SelfActivate","status":"Provisioned"}},
		{"name":"admin","properties":{"roleDefinitionId":"` + testRoleDefID + `","requestType":"AdminAssign","status":"PendingApproval"}},
		{"name":"pending-1","properties":{"roleDefinitionId":"` + testRoleDefID + `","requestType":"SelfActivate","status":"PendingApproval"}}
	]}`
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, body), nil
	}}
	name, found, err := newClientWithDoer(doer).FindPendingRequest(context.Background(), testRoleDefID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "pending-1", name)
}

func TestARMPIMClient_CreateActivationRequest(t *testing.T) {
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusCreated, `{"name":"req-1","properties":{"status":"Provisioned"}}`), nil
	}}
	c := newClientWithDoer(doer)

	res, err := c.CreateActivationRequest(context.Background(), &ActivationRequest{
		RequestName:           "req-1",
		PrincipalID:           testPrincipalOID,
		RoleDefinitionID:      testRoleDefID,
		EligibilityScheduleID: testEligID,
		Justification:         "because",
		Duration:              "PT8H",
	})
	require.NoError(t, err)
	assert.Equal(t, pimStatusProvisioned, res.Status)
	assert.Equal(t, "req-1", res.RequestName)

	// PUT to the request resource, with the activation body.
	req := doer.requests[0]
	assert.Equal(t, http.MethodPut, req.Method)
	assert.True(t, strings.HasSuffix(req.URL.Path, "/roleAssignmentScheduleRequests/req-1"))

	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(doer.bodies[0]), &sent))
	props := sent["properties"].(map[string]any)
	assert.Equal(t, testPrincipalOID, props["principalId"])
	assert.Equal(t, "SelfActivate", props["requestType"])
	assert.Equal(t, testEligID, props["linkedRoleEligibilityScheduleId"])
	assert.Equal(t, "because", props["justification"])
	sched := props["scheduleInfo"].(map[string]any)
	exp := sched["expiration"].(map[string]any)
	assert.Equal(t, "AfterDuration", exp["type"])
	assert.Equal(t, "PT8H", exp["duration"])
}

func TestARMPIMClient_CreateActivationRequest_OmitsEmptyDuration(t *testing.T) {
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"name":"req-1","properties":{"status":"PendingApproval"}}`), nil
	}}
	_, err := newClientWithDoer(doer).CreateActivationRequest(context.Background(), &ActivationRequest{
		RequestName: "req-1", Duration: "",
	})
	require.NoError(t, err)

	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(doer.bodies[0]), &sent))
	exp := sent["properties"].(map[string]any)["scheduleInfo"].(map[string]any)["expiration"].(map[string]any)
	_, hasDuration := exp["duration"]
	assert.False(t, hasDuration, "empty duration must be omitted so ARM applies the policy default")
}

func TestARMPIMClient_GetRequestStatus(t *testing.T) {
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"name":"req-1","properties":{"status":"PendingApproval"}}`), nil
	}}
	status, err := newClientWithDoer(doer).GetRequestStatus(context.Background(), "req-1")
	require.NoError(t, err)
	assert.Equal(t, "PendingApproval", status)
	assert.Equal(t, http.MethodGet, doer.requests[0].Method)
	assert.True(t, strings.HasSuffix(doer.requests[0].URL.Path, "/roleAssignmentScheduleRequests/req-1"))
}

func TestARMPIMClient_NonSuccessStatus(t *testing.T) {
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, `{"error":{"code":"AuthorizationFailed"}}`), nil
	}}
	_, err := newClientWithDoer(doer).ActiveAssignmentExists(context.Background(), testRoleDefID)
	require.ErrorIs(t, err, errUtils.ErrAzurePIMRequestFailed)
	assert.Contains(t, err.Error(), "403")
}

func TestARMPIMClient_TransportError(t *testing.T) {
	sentinel := errors.New("dial tcp: connection refused")
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return nil, sentinel
	}}
	_, err := newClientWithDoer(doer).GetRequestStatus(context.Background(), "req-1")
	require.ErrorIs(t, err, errUtils.ErrAzurePIMRequestFailed)
}

func TestARMPIMClient_BadJSON(t *testing.T) {
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{not json`), nil
	}}
	_, err := newClientWithDoer(doer).GetRequestStatus(context.Background(), "req-1")
	require.ErrorIs(t, err, errUtils.ErrAzurePIMRequestFailed)
}

func TestARMPIMClient_RefusesHostInjection(t *testing.T) {
	// A crafted scope that demotes the ARM host to URL user-information and points the request
	// at an attacker host. The client must fail closed BEFORE sending the bearer token.
	called := false
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{"value":[]}`), nil
	}}
	c := newARMPIMClient(doer, "test-token", "@collector.example/subscriptions/x", "https://management.azure.com")

	_, err := c.ActiveAssignmentExists(context.Background(), testRoleDefID)
	require.ErrorIs(t, err, errUtils.ErrAzurePIMInvalidScope)
	assert.False(t, called, "no request (and no bearer token) must reach a non-ARM host")
	assert.Empty(t, doer.requests, "the request must never be dispatched")
}

func TestNewARMPIMClient_Defaults(t *testing.T) {
	c := newARMPIMClient(nil, "tok", testScope+"/", "https://management.azure.com/")
	assert.NotNil(t, c.doer)
	assert.Equal(t, testScope, c.scope, "trailing slash trimmed from scope")
	assert.Equal(t, "https://management.azure.com", c.baseURL, "trailing slash trimmed from base URL")
	assert.Equal(t, pimAPIVersion, c.apiVersion)
}

func TestGoDurationToISO8601(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"8h", "PT8H"},
		{"90m", "PT1H30M"},
		{"30m", "PT30M"},
		{"45s", "PT45S"},
		{"1h30m15s", "PT1H30M15S"},
		{"2h0m0s", "PT2H"},
		{"0s", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			d, err := time.ParseDuration(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, goDurationToISO8601(d))
		})
	}
}

func TestNormalizeAndSameRoleDefinition(t *testing.T) {
	full := "/subscriptions/x/providers/Microsoft.Authorization/roleDefinitions/ABC123"
	bare := "/providers/Microsoft.Authorization/roleDefinitions/abc123"
	assert.True(t, sameRoleDefinition(full, bare), "case-insensitive, scope-prefix-insensitive match")
	assert.False(t, sameRoleDefinition(full, "/providers/Microsoft.Authorization/roleDefinitions/different"))
	assert.False(t, sameRoleDefinition("", bare))
	// A non-standard id normalizes to itself (lowercased).
	assert.Equal(t, "custom-role", normalizeRoleDefinitionID("Custom-Role"))
}

func TestSubscriptionIDFromScope(t *testing.T) {
	tests := []struct {
		scope string
		want  string
	}{
		{"/subscriptions/abc-123", "abc-123"},
		{"/subscriptions/abc-123/resourceGroups/rg1", "abc-123"},
		{"/subscriptions/abc-123/resourceGroups/rg1/providers/x/y/z", "abc-123"},
		{"/providers/Microsoft.Management/managementGroups/mg1", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.scope, func(t *testing.T) {
			assert.Equal(t, tt.want, subscriptionIDFromScope(tt.scope))
		})
	}
}

func TestIsPIMPendingStatus(t *testing.T) {
	assert.True(t, IsPIMPendingStatus("PendingApproval"))
	assert.True(t, IsPIMPendingStatus("Granted"))
	assert.False(t, IsPIMPendingStatus("Provisioned"))
	assert.False(t, IsPIMPendingStatus("Denied"))
	assert.False(t, IsPIMPendingStatus(""))
}

func TestSnippet(t *testing.T) {
	assert.Equal(t, "short", snippet([]byte("  short  ")))
	long := strings.Repeat("x", 400)
	assert.True(t, strings.HasSuffix(snippet([]byte(long)), "..."))
	assert.LessOrEqual(t, len(snippet([]byte(long))), 303)
}

func TestARMPIMClient_PolicyMaxDuration_Found(t *testing.T) {
	body := `{
		"value": [
			{
				"name": "assignment-1",
				"properties": {
					"roleDefinitionId": "` + testRoleDefID + `",
					"effectiveRules": [
						{
							"id": "Expiration_Admin_Eligibility",
							"ruleType": "RoleManagementPolicyExpirationRule",
							"maximumDuration": "P90D",
							"target": {"caller": "Admin", "level": "Eligibility"}
						},
						{
							"id": "Expiration_EndUser_Assignment",
							"ruleType": "RoleManagementPolicyExpirationRule",
							"maximumDuration": "PT4H",
							"target": {"caller": "EndUser", "level": "Assignment"}
						}
					]
				}
			}
		]
	}`
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, body), nil
	}}
	c := newClientWithDoer(doer)

	maxDuration, found, err := c.PolicyMaxDuration(context.Background(), testRoleDefID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 4*time.Hour, maxDuration)

	// Verify request attributes: GET, scoped path, no $filter, api-version, bearer auth.
	req := doer.requests[0]
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Contains(t, req.URL.Path, "/providers/Microsoft.Authorization/roleManagementPolicyAssignments")
	assert.Equal(t, "2020-10-01", req.URL.Query().Get("api-version"))
	assert.Empty(t, req.URL.Query().Get("$filter"))
	assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
}

func TestARMPIMClient_PolicyMaxDuration_NotFound(t *testing.T) {
	body := `{
		"value": [
			{
				"name": "assignment-other",
				"properties": {
					"roleDefinitionId": "/providers/Microsoft.Authorization/roleDefinitions/other",
					"effectiveRules": [
						{
							"id": "Expiration_EndUser_Assignment",
							"ruleType": "RoleManagementPolicyExpirationRule",
							"maximumDuration": "PT4H",
							"target": {"caller": "EndUser", "level": "Assignment"}
						}
					]
				}
			}
		]
	}`
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, body), nil
	}}
	c := newClientWithDoer(doer)

	maxDuration, found, err := c.PolicyMaxDuration(context.Background(), testRoleDefID)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, time.Duration(0), maxDuration)
}

func TestARMPIMClient_PolicyMaxDuration_NoExpirationRule(t *testing.T) {
	body := `{
		"value": [
			{
				"name": "assignment-1",
				"properties": {
					"roleDefinitionId": "` + testRoleDefID + `",
					"effectiveRules": [
						{
							"id": "Expiration_Admin_Eligibility",
							"ruleType": "RoleManagementPolicyExpirationRule",
							"maximumDuration": "P90D",
							"target": {"caller": "Admin", "level": "Eligibility"}
						}
					]
				}
			}
		]
	}`
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, body), nil
	}}
	c := newClientWithDoer(doer)

	maxDuration, found, err := c.PolicyMaxDuration(context.Background(), testRoleDefID)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, time.Duration(0), maxDuration)
}

func TestARMPIMClient_PolicyMaxDuration_RulesFallback(t *testing.T) {
	body := `{
		"value": [
			{
				"name": "assignment-1",
				"properties": {
					"roleDefinitionId": "` + testRoleDefID + `",
					"rules": [
						{
							"id": "Expiration_EndUser_Assignment",
							"ruleType": "RoleManagementPolicyExpirationRule",
							"maximumDuration": "PT2H",
							"target": {"caller": "EndUser", "level": "Assignment"}
						}
					]
				}
			}
		]
	}`
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, body), nil
	}}
	c := newClientWithDoer(doer)

	maxDuration, found, err := c.PolicyMaxDuration(context.Background(), testRoleDefID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 2*time.Hour, maxDuration)
}

func TestARMPIMClient_PolicyMaxDuration_HTTPError(t *testing.T) {
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusInternalServerError, `{"error":{"code":"InternalServerError"}}`), nil
	}}
	c := newClientWithDoer(doer)

	_, _, err := c.PolicyMaxDuration(context.Background(), testRoleDefID)
	require.ErrorIs(t, err, errUtils.ErrAzurePIMRequestFailed)
	assert.Contains(t, err.Error(), "500")
}

func TestARMPIMClient_PolicyMaxDuration_BadJSON(t *testing.T) {
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{not-json`), nil
	}}
	c := newClientWithDoer(doer)

	_, _, err := c.PolicyMaxDuration(context.Background(), testRoleDefID)
	require.ErrorIs(t, err, errUtils.ErrAzurePIMRequestFailed)
}

func TestARMPIMClient_PolicyMaxDuration_BadDurationFormat(t *testing.T) {
	body := `{
		"value": [
			{
				"name": "assignment-1",
				"properties": {
					"roleDefinitionId": "` + testRoleDefID + `",
					"effectiveRules": [
						{
							"id": "Expiration_EndUser_Assignment",
							"ruleType": "RoleManagementPolicyExpirationRule",
							"maximumDuration": "invalid-iso",
							"target": {"caller": "EndUser", "level": "Assignment"}
						}
					]
				}
			}
		]
	}`
	doer := &fakeDoer{handler: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, body), nil
	}}
	c := newClientWithDoer(doer)

	_, _, err := c.PolicyMaxDuration(context.Background(), testRoleDefID)
	require.ErrorIs(t, err, errUtils.ErrAzurePIMRequestFailed)
	assert.Contains(t, err.Error(), "invalid-iso")
}

func TestParseISO8601Duration(t *testing.T) {
	validTests := []struct {
		in   string
		want time.Duration
	}{
		{"PT8H", 8 * time.Hour},
		{"PT7H", 7 * time.Hour},
		{"PT1H30M", 90 * time.Minute},
		{"PT30M", 30 * time.Minute},
		{"PT45S", 45 * time.Second},
		{"PT1H30M15S", 1*time.Hour + 30*time.Minute + 15*time.Second},
		{"P1D", 24 * time.Hour},
		{"P1DT8H", 32 * time.Hour},
		{"P2W", 14 * 24 * time.Hour},
		{"P1Y", 365 * 24 * time.Hour},
		{"P2Y", 2 * 365 * 24 * time.Hour},
		{"p1y", 365 * 24 * time.Hour},
		{"P1M", 30 * 24 * time.Hour},
		{"P6M", 6 * 30 * 24 * time.Hour},
		{"p1m", 30 * 24 * time.Hour},
		{"P1Y2M3W4DT5H6M7S", time.Duration(1*365*24+2*30*24+3*7*24+4*24+5)*time.Hour + 6*time.Minute + 7*time.Second},
		{"PT0.5H", 30 * time.Minute},
		{"PT0.5M", 30 * time.Second},
		{"PT0.5S", 500 * time.Millisecond},
		{"PT0S", 0},
		{"pt8h", 8 * time.Hour},
		{"P90D", 90 * 24 * time.Hour},
	}
	for _, tt := range validTests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseISO8601Duration(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	invalidTests := []string{
		"",
		"   ",
		"8h",
		"P",
		"PT",
		"P.",
		"PT.",
		"P1X",
		"PT1",
		"invalid",
		"P1D1H",
		"P.Y",
		"P.M",
		"P.W",
		"P.D",
		"P1D2",
		"P1D!",
		"PT.H",
		"PT.M",
		"PT.S",
		"PT1H2",
		"PT1X",
		"PT!",
	}
	for _, in := range invalidTests {
		t.Run("invalid_"+in, func(t *testing.T) {
			_, err := parseISO8601Duration(in)
			assert.Error(t, err)
		})
	}
}

func TestIsActivationExpirationRule(t *testing.T) {
	tests := []struct {
		name string
		rule armPolicyAssignmentRule
		want bool
	}{
		{
			name: "valid EndUser Assignment",
			rule: armPolicyAssignmentRule{
				RuleType:        "RoleManagementPolicyExpirationRule",
				MaximumDuration: "PT8H",
				Target: armPolicyAssignmentRuleTarget{
					Caller: "EndUser",
					Level:  "Assignment",
				},
			},
			want: true,
		},
		{
			name: "valid EndUser empty level",
			rule: armPolicyAssignmentRule{
				RuleType:        "RoleManagementPolicyExpirationRule",
				MaximumDuration: "PT8H",
				Target: armPolicyAssignmentRuleTarget{
					Caller: "EndUser",
					Level:  "",
				},
			},
			want: true,
		},
		{
			name: "EndUser with non-assignment level",
			rule: armPolicyAssignmentRule{
				RuleType:        "RoleManagementPolicyExpirationRule",
				MaximumDuration: "PT8H",
				Target: armPolicyAssignmentRuleTarget{
					Caller: "EndUser",
					Level:  "Eligibility",
				},
			},
			want: false,
		},
		{
			name: "wrong rule type",
			rule: armPolicyAssignmentRule{
				RuleType:        "RoleManagementPolicyNotificationRule",
				MaximumDuration: "PT8H",
				Target: armPolicyAssignmentRuleTarget{
					Caller: "EndUser",
					Level:  "Assignment",
				},
			},
			want: false,
		},
		{
			name: "empty maximumDuration",
			rule: armPolicyAssignmentRule{
				RuleType:        "RoleManagementPolicyExpirationRule",
				MaximumDuration: "",
				Target: armPolicyAssignmentRuleTarget{
					Caller: "EndUser",
					Level:  "Assignment",
				},
			},
			want: false,
		},
		{
			name: "fallback by ID Expiration_EndUser_Assignment",
			rule: armPolicyAssignmentRule{
				ID:              "Expiration_EndUser_Assignment",
				RuleType:        "RoleManagementPolicyExpirationRule",
				MaximumDuration: "PT4H",
				Target: armPolicyAssignmentRuleTarget{
					Caller: "OtherCaller",
					Level:  "OtherLevel",
				},
			},
			want: true,
		},
		{
			name: "fallback by ID containing enduser",
			rule: armPolicyAssignmentRule{
				ID:              "custom_enduser_rule",
				RuleType:        "RoleManagementPolicyExpirationRule",
				MaximumDuration: "PT4H",
				Target: armPolicyAssignmentRuleTarget{
					Caller: "OtherCaller",
					Level:  "OtherLevel",
				},
			},
			want: true,
		},
		{
			name: "unmatched non-enduser rule",
			rule: armPolicyAssignmentRule{
				ID:              "Expiration_Admin_Eligibility",
				RuleType:        "RoleManagementPolicyExpirationRule",
				MaximumDuration: "PT4H",
				Target: armPolicyAssignmentRuleTarget{
					Caller: "Admin",
					Level:  "Eligibility",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isActivationExpirationRule(&tt.rule))
		})
	}
}

