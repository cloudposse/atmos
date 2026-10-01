package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// pimAPIVersion is the Authorization RBAC REST api-version used for PIM
// role-assignment-schedule operations (per the Microsoft docs referenced in the PRD).
const pimAPIVersion = "2020-10-01"

// secondsPerHour and secondsPerMinute convert a duration to ISO-8601 components.
const (
	secondsPerHour   = 3600
	secondsPerMinute = 60
)

// pimStatusProvisioned is the terminal success status for an activation request:
// the role is active and the assignment schedule instance exists.
const pimStatusProvisioned = "Provisioned"

// pimPendingStatuses are the non-terminal request statuses that mean activation is still
// in flight (most commonly awaiting an approver). A request in any of these states is
// resumable - a later invocation attaches to it rather than creating a duplicate.
var pimPendingStatuses = map[string]bool{
	"PendingApproval":             true,
	"PendingApprovalProvisioning": true,
	"PendingProvisioning":         true,
	"PendingScheduleCreation":     true,
	"ScheduleCreated":             true,
	"Granted":                     true,
	"PendingExternalProvisioning": true,
}

// IsPIMPendingStatus reports whether a PIM request status is non-terminal (still in flight).
func IsPIMPendingStatus(status string) bool {
	return pimPendingStatuses[status]
}

// ActivationRequest describes a single PIM self-activation request.
type ActivationRequest struct {
	// RequestName is the GUID used as the roleAssignmentScheduleRequests resource name
	// (the last path segment of the PUT). The caller generates it so the request is
	// addressable for polling.
	RequestName string
	// PrincipalID is the Azure AD object id being elevated (the parent principal).
	PrincipalID string
	// RoleDefinitionID is the full role definition id to activate.
	RoleDefinitionID string
	// EligibilityScheduleID links the activation to the eligibility it exercises
	// (roleEligibilityScheduleId from the eligibility instance).
	EligibilityScheduleID string
	// Justification is the human-supplied reason recorded with the request.
	Justification string
	// Duration is the ISO-8601 activation duration (for example "PT8H").
	Duration string
}

// ActivationResult is the outcome of creating or polling an activation request.
type ActivationResult struct {
	// RequestName is the request's resource name (GUID).
	RequestName string
	// Status is the current provisioning status (for example "Provisioned", "PendingApproval").
	Status string
}

// PIMClient abstracts the Azure Resource Manager PIM REST surface so the
// azure/pim-role identity can be unit-tested without a live tenant. Every method is
// scoped to the scope and principal the client was constructed for; implementations
// filter ARM results to the authenticated principal via `$filter=asTarget()`.
type PIMClient interface {
	// ActiveAssignmentExists reports whether an active role assignment schedule instance
	// already covers the role at the client's scope, so activation can be skipped.
	ActiveAssignmentExists(ctx context.Context, roleDefinitionID string) (bool, error)

	// FindEligibility returns the roleEligibilityScheduleId the principal can activate for
	// the role at the client's scope. found is false when the principal has no eligibility.
	FindEligibility(ctx context.Context, roleDefinitionID string) (scheduleID string, found bool, err error)

	// FindPendingRequest returns the resource name of an in-flight SelfActivate request for
	// the role at the client's scope, if one already exists (for idempotent resume).
	FindPendingRequest(ctx context.Context, roleDefinitionID string) (requestName string, found bool, err error)

	// CreateActivationRequest issues a SelfActivate roleAssignmentScheduleRequest.
	CreateActivationRequest(ctx context.Context, req *ActivationRequest) (ActivationResult, error)

	// GetRequestStatus returns the current provisioning status of a request by resource name.
	GetRequestStatus(ctx context.Context, requestName string) (string, error)
}

// httpDoer is the minimal HTTP surface the ARM client depends on, so tests can inject a
// fake transport instead of reaching the network.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// armPIMClient is the live PIMClient backed by the Azure Resource Manager REST API,
// authenticated with the management token the parent identity already acquired.
type armPIMClient struct {
	doer       httpDoer
	token      string
	scope      string // ARM scope id, e.g. /subscriptions/{id}.
	baseURL    string // ARM endpoint, e.g. https://management.azure.com.
	apiVersion string
}

// NewARMPIMClient-equivalent constructor. Builds a live PIM client for a scope, using the
// given management token and ARM base URL; doer defaults to http.DefaultClient when nil.
func newARMPIMClient(doer httpDoer, token, scope, baseURL string) *armPIMClient {
	if doer == nil {
		doer = http.DefaultClient
	}
	return &armPIMClient{
		doer:       doer,
		token:      token,
		scope:      strings.TrimSuffix(scope, "/"),
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiVersion: pimAPIVersion,
	}
}

// armListEnvelope is the common ARM list response shape.
type armListEnvelope struct {
	Value []armScheduleItem `json:"value"`
}

// armScheduleItem captures the fields we read from assignment/eligibility/request items.
type armScheduleItem struct {
	Name       string `json:"name"`
	Properties struct {
		RoleDefinitionID          string `json:"roleDefinitionId"`
		RoleEligibilityScheduleID string `json:"roleEligibilityScheduleId"`
		RequestType               string `json:"requestType"`
		Status                    string `json:"status"`
	} `json:"properties"`
}

// ActiveAssignmentExists implements PIMClient.
func (c *armPIMClient) ActiveAssignmentExists(ctx context.Context, roleDefinitionID string) (bool, error) {
	defer perf.Track(nil, "azure.armPIMClient.ActiveAssignmentExists")()

	items, err := c.list(ctx, "roleAssignmentScheduleInstances")
	if err != nil {
		return false, err
	}
	for i := range items {
		if sameRoleDefinition(items[i].Properties.RoleDefinitionID, roleDefinitionID) {
			return true, nil
		}
	}
	return false, nil
}

// FindEligibility implements PIMClient.
func (c *armPIMClient) FindEligibility(ctx context.Context, roleDefinitionID string) (string, bool, error) {
	defer perf.Track(nil, "azure.armPIMClient.FindEligibility")()

	items, err := c.list(ctx, "roleEligibilityScheduleInstances")
	if err != nil {
		return "", false, err
	}
	for i := range items {
		if sameRoleDefinition(items[i].Properties.RoleDefinitionID, roleDefinitionID) {
			return items[i].Properties.RoleEligibilityScheduleID, true, nil
		}
	}
	return "", false, nil
}

// FindPendingRequest implements PIMClient.
func (c *armPIMClient) FindPendingRequest(ctx context.Context, roleDefinitionID string) (string, bool, error) {
	defer perf.Track(nil, "azure.armPIMClient.FindPendingRequest")()

	items, err := c.list(ctx, "roleAssignmentScheduleRequests")
	if err != nil {
		return "", false, err
	}
	for i := range items {
		p := items[i].Properties
		if sameRoleDefinition(p.RoleDefinitionID, roleDefinitionID) && p.RequestType == "SelfActivate" && IsPIMPendingStatus(p.Status) {
			return items[i].Name, true, nil
		}
	}
	return "", false, nil
}

// CreateActivationRequest implements PIMClient.
func (c *armPIMClient) CreateActivationRequest(ctx context.Context, req *ActivationRequest) (ActivationResult, error) {
	defer perf.Track(nil, "azure.armPIMClient.CreateActivationRequest")()

	// expiration is AfterDuration with the configured duration. An empty duration omits it
	// so ARM applies the role's PIM activation-policy default/maximum.
	expiration := map[string]any{"type": "AfterDuration"}
	if req.Duration != "" {
		expiration["duration"] = req.Duration
	}
	body := map[string]any{
		"properties": map[string]any{
			"principalId":                     req.PrincipalID,
			"roleDefinitionId":                req.RoleDefinitionID,
			"requestType":                     "SelfActivate",
			"linkedRoleEligibilityScheduleId": req.EligibilityScheduleID,
			"justification":                   req.Justification,
			"scheduleInfo": map[string]any{
				"startDateTime": nil,
				"expiration":    expiration,
			},
		},
	}

	path := fmt.Sprintf("%s/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/%s", c.scope, req.RequestName)
	raw, err := c.do(ctx, http.MethodPut, path, "", body)
	if err != nil {
		return ActivationResult{}, err
	}

	var item armScheduleItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return ActivationResult{}, fmt.Errorf("%w: decoding activation response: %w", errUtils.ErrAzurePIMRequestFailed, err)
	}
	return ActivationResult{RequestName: req.RequestName, Status: item.Properties.Status}, nil
}

// GetRequestStatus implements PIMClient.
func (c *armPIMClient) GetRequestStatus(ctx context.Context, requestName string) (string, error) {
	defer perf.Track(nil, "azure.armPIMClient.GetRequestStatus")()

	path := fmt.Sprintf("%s/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/%s", c.scope, requestName)
	raw, err := c.do(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		return "", err
	}
	var item armScheduleItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return "", fmt.Errorf("%w: decoding request status: %w", errUtils.ErrAzurePIMRequestFailed, err)
	}
	return item.Properties.Status, nil
}

// list issues a GET against a scoped Authorization collection filtered to the caller via
// `$filter=asTarget()`, which scopes ARM's result set to the authenticated principal.
func (c *armPIMClient) list(ctx context.Context, collection string) ([]armScheduleItem, error) {
	path := fmt.Sprintf("%s/providers/Microsoft.Authorization/%s", c.scope, collection)
	raw, err := c.do(ctx, http.MethodGet, path, "asTarget()", nil)
	if err != nil {
		return nil, err
	}
	var env armListEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%w: decoding %s: %w", errUtils.ErrAzurePIMRequestFailed, collection, err)
	}
	return env.Value, nil
}

// Do performs a single ARM request and returns the raw response body on 2xx. The filter
// argument, when non-empty, is sent as the OData `$filter` query parameter.
func (c *armPIMClient) do(ctx context.Context, method, path, filter string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%w: encoding request body: %w", errUtils.ErrAzurePIMRequestFailed, err)
		}
		reader = bytes.NewReader(encoded)
	}

	query := url.Values{}
	query.Set("api-version", c.apiVersion)
	if filter != "" {
		query.Set("$filter", filter)
	}
	u := c.baseURL + path + "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, fmt.Errorf("%w: building request: %w", errUtils.ErrAzurePIMRequestFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAzurePIMRequestFailed, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: reading response: %w", errUtils.ErrAzurePIMRequestFailed, err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: %s %s returned %d: %s", errUtils.ErrAzurePIMRequestFailed, method, path, resp.StatusCode, snippet(raw))
	}
	return raw, nil
}

// snippet trims an ARM error body to a short, log-safe excerpt.
func snippet(b []byte) string {
	const max = 300
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// sameRoleDefinition compares two role definition ids case-insensitively and ignores a
// leading scope prefix, since ARM returns fully-qualified ids while config may supply the
// bare /providers/Microsoft.Authorization/roleDefinitions/{guid} form.
func sameRoleDefinition(a, b string) bool {
	na := normalizeRoleDefinitionID(a)
	nb := normalizeRoleDefinitionID(b)
	return na != "" && na == nb
}

// normalizeRoleDefinitionID reduces a role definition id to its lowercased
// /providers/Microsoft.Authorization/roleDefinitions/{guid} suffix for comparison.
func normalizeRoleDefinitionID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	const marker = "/providers/microsoft.authorization/roledefinitions/"
	if idx := strings.LastIndex(id, marker); idx >= 0 {
		return marker + id[idx+len(marker):]
	}
	return id
}

// goDurationToISO8601 converts a Go time.Duration to the ISO-8601 duration ARM expects
// for scheduleInfo.expiration (for example 8h -> "PT8H", 90m -> "PT1H30M").
func goDurationToISO8601(d time.Duration) string {
	totalSeconds := int64(d.Seconds())
	if totalSeconds <= 0 {
		return ""
	}
	hours := totalSeconds / secondsPerHour
	minutes := (totalSeconds % secondsPerHour) / secondsPerMinute
	seconds := totalSeconds % secondsPerMinute

	var sb strings.Builder
	sb.WriteString("PT")
	if hours > 0 {
		fmt.Fprintf(&sb, "%dH", hours)
	}
	if minutes > 0 {
		fmt.Fprintf(&sb, "%dM", minutes)
	}
	if seconds > 0 {
		fmt.Fprintf(&sb, "%dS", seconds)
	}
	return sb.String()
}
