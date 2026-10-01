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
