package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

// defaultARMRequestTimeout bounds a single ARM request when the caller injects no HTTP
// client, so a stalled endpoint cannot hang authentication indefinitely.
const defaultARMRequestTimeout = 30 * time.Second

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

	// PolicyMaxDuration returns the maximum activation duration allowed by the role's
	// PIM policy at the client's scope. found is false when no policy assignment or
	// activation expiration rule is configured for the role.
	PolicyMaxDuration(ctx context.Context, roleDefinitionID string) (maxDuration time.Duration, found bool, err error)
}

// httpDoer is the minimal HTTP surface the ARM client depends on, so tests can inject a
// fake transport instead of reaching the network.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// armPIMClient is the live PIMClient backed by the Azure Resource Manager REST API,
// authenticated with the management token the parent identity already acquired.
type armPIMClient struct {
	doer           httpDoer
	token          string
	scope          string // ARM scope id, e.g. /subscriptions/{id}.
	baseURL        string // ARM endpoint, e.g. https://management.azure.com.
	apiVersion     string
	expectedScheme string // Expected request scheme (from baseURL); enforced before sending the bearer.
	expectedHost   string // Expected request host (from baseURL); enforced before sending the bearer.
}

// NewARMPIMClient-equivalent constructor. Builds a live PIM client for a scope, using the
// given management token and ARM base URL. When doer is nil it defaults to an HTTP client
// with a finite timeout (not http.DefaultClient, which has none) so a stalled ARM endpoint
// or proxy cannot hang `auth login`/`auth exec` when the caller's context has no deadline.
func newARMPIMClient(doer httpDoer, token, scope, baseURL string) *armPIMClient {
	if doer == nil {
		doer = &http.Client{Timeout: defaultARMRequestTimeout}
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	// Record the intended origin so do() can fail closed if a crafted scope redirects the
	// credential-bearing request to a different host (URL user-information injection).
	scheme, host := "", ""
	if parsed, err := url.Parse(baseURL); err == nil {
		scheme, host = parsed.Scheme, parsed.Host
	}
	return &armPIMClient{
		doer:           doer,
		token:          token,
		scope:          strings.TrimSuffix(scope, "/"),
		baseURL:        baseURL,
		apiVersion:     pimAPIVersion,
		expectedScheme: scheme,
		expectedHost:   host,
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
	req, err := c.newRequest(ctx, method, path, filter, body)
	if err != nil {
		return nil, err
	}
	return c.doRequest(req, path)
}

// doRequest performs a prepared HTTP request and returns the response body on 2xx.
func (c *armPIMClient) doRequest(req *http.Request, displayPath string) ([]byte, error) {
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
		return nil, fmt.Errorf("%w: %s %s returned %d: %s", errUtils.ErrAzurePIMRequestFailed, req.Method, displayPath, resp.StatusCode, snippet(raw))
	}
	return raw, nil
}

// authorizeRequest enforces that the request targets the expected ARM origin and attaches credentials.
func (c *armPIMClient) authorizeRequest(req *http.Request) error {
	// Fail closed before attaching the bearer token: the scope or continuation URL can
	// redirect the request to an attacker-controlled host (URL user-information injection).
	// Refuse to send credentials anywhere other than the intended ARM origin.
	if req.URL.Scheme != c.expectedScheme || req.URL.Host != c.expectedHost || req.URL.User != nil {
		return fmt.Errorf("%w: refusing to send credentials to %q://%q (expected %q://%q)",
			errUtils.ErrAzurePIMInvalidScope, req.URL.Scheme, req.URL.Host, c.expectedScheme, c.expectedHost)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

// newRequest builds the ARM request, enforces the credential-bearing request stays on the
// intended ARM origin, and attaches the bearer token and content headers.
func (c *armPIMClient) newRequest(ctx context.Context, method, path, filter string, body any) (*http.Request, error) {
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

	if err := c.authorizeRequest(req); err != nil {
		return nil, err
	}
	return req, nil
}

// newContinuationRequest builds an ARM request for a pagination continuation link (nextLink).
// It preserves continuation query parameters, resolves relative URLs against baseURL,
// ensures api-version is present, and enforces the same-origin check before attaching credentials.
func (c *armPIMClient) newContinuationRequest(ctx context.Context, continuationURL string) (*http.Request, error) {
	parsedURL, err := url.Parse(continuationURL)
	if err != nil {
		return nil, fmt.Errorf("%w: parsing continuation URL %q: %w", errUtils.ErrAzurePIMRequestFailed, continuationURL, err)
	}

	if !parsedURL.IsAbs() {
		baseURL, err := url.Parse(c.baseURL)
		if err != nil {
			return nil, fmt.Errorf("%w: parsing base URL %q: %w", errUtils.ErrAzurePIMRequestFailed, c.baseURL, err)
		}
		parsedURL = baseURL.ResolveReference(parsedURL)
	}

	if parsedURL.Query().Get("api-version") == "" && c.apiVersion != "" {
		if parsedURL.RawQuery == "" {
			parsedURL.RawQuery = "api-version=" + url.QueryEscape(c.apiVersion)
		} else {
			parsedURL.RawQuery += "&api-version=" + url.QueryEscape(c.apiVersion)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: building continuation request: %w", errUtils.ErrAzurePIMRequestFailed, err)
	}

	if err := c.authorizeRequest(req); err != nil {
		return nil, err
	}
	return req, nil
}

// doContinuation performs a GET request against an ARM pagination continuation link.
func (c *armPIMClient) doContinuation(ctx context.Context, continuationURL string) ([]byte, error) {
	req, err := c.newContinuationRequest(ctx, continuationURL)
	if err != nil {
		return nil, err
	}
	return c.doRequest(req, continuationURL)
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

// armPolicyAssignmentEnvelope is the ARM list response shape for roleManagementPolicyAssignments.
type armPolicyAssignmentEnvelope struct {
	Value    []armPolicyAssignmentItem `json:"value"`
	NextLink string                    `json:"nextLink"`
}

// armPolicyAssignmentItem captures a roleManagementPolicyAssignment resource.
type armPolicyAssignmentItem struct {
	Name       string                        `json:"name"`
	Properties armPolicyAssignmentProperties `json:"properties"`
}

// armPolicyAssignmentProperties captures the assignment's role definition and rules.
type armPolicyAssignmentProperties struct {
	RoleDefinitionID string                    `json:"roleDefinitionId"`
	EffectiveRules   []armPolicyAssignmentRule `json:"effectiveRules"`
	Rules            []armPolicyAssignmentRule `json:"rules"`
}

// armPolicyAssignmentRule captures rule properties across policy rule types.
type armPolicyAssignmentRule struct {
	ID              string                        `json:"id"`
	RuleType        string                        `json:"ruleType"`
	MaximumDuration string                        `json:"maximumDuration"`
	Target          armPolicyAssignmentRuleTarget `json:"target"`
}

// armPolicyAssignmentRuleTarget captures target metadata (e.g. caller: EndUser, level: Assignment).
type armPolicyAssignmentRuleTarget struct {
	Caller string `json:"caller"`
	Level  string `json:"level"`
}

// isActivationExpirationRule reports whether a policy rule governs the maximum duration
// of an EndUser self-activation assignment.
func isActivationExpirationRule(r *armPolicyAssignmentRule) bool {
	if !strings.EqualFold(r.RuleType, "RoleManagementPolicyExpirationRule") {
		return false
	}
	if r.MaximumDuration == "" {
		return false
	}

	caller := strings.TrimSpace(r.Target.Caller)
	level := strings.TrimSpace(r.Target.Level)
	hasCaller := caller != ""
	hasLevel := level != ""

	if hasCaller && hasLevel {
		return strings.EqualFold(caller, "EndUser") && strings.EqualFold(level, "Assignment")
	}
	if hasCaller {
		return strings.EqualFold(caller, "EndUser")
	}
	if hasLevel {
		return strings.EqualFold(level, "Assignment")
	}

	// Target is absent; fallback to ID matching.
	if strings.EqualFold(r.ID, "Expiration_EndUser_Assignment") || strings.Contains(strings.ToLower(r.ID), "enduser") {
		return true
	}
	return false
}

// PolicyMaxDuration implements PIMClient.
func (c *armPIMClient) PolicyMaxDuration(ctx context.Context, roleDefinitionID string) (time.Duration, bool, error) {
	defer perf.Track(nil, "azure.armPIMClient.PolicyMaxDuration")()

	assignments, err := c.listPolicyAssignments(ctx)
	if err != nil {
		return 0, false, err
	}
	for i := range assignments {
		if !sameRoleDefinition(assignments[i].Properties.RoleDefinitionID, roleDefinitionID) {
			continue
		}
		rules := assignments[i].Properties.EffectiveRules
		if len(rules) == 0 {
			rules = assignments[i].Properties.Rules
		}
		for j := range rules {
			if isActivationExpirationRule(&rules[j]) {
				d, err := parseISO8601Duration(rules[j].MaximumDuration)
				if err != nil {
					return 0, false, fmt.Errorf("%w: parsing policy maximumDuration %q: %w", errUtils.ErrAzurePIMRequestFailed, rules[j].MaximumDuration, err)
				}
				return d, true, nil
			}
		}
	}
	return 0, false, nil
}

// listPolicyAssignments retrieves role management policy assignments for the client's scope,
// following ARM continuation links to combine assignment values from every page before returning.
func (c *armPIMClient) listPolicyAssignments(ctx context.Context) ([]armPolicyAssignmentItem, error) {
	path := fmt.Sprintf("%s/providers/Microsoft.Authorization/roleManagementPolicyAssignments", c.scope)
	raw, err := c.do(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		return nil, err
	}
	var env armPolicyAssignmentEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%w: decoding roleManagementPolicyAssignments: %w", errUtils.ErrAzurePIMRequestFailed, err)
	}

	items := env.Value
	nextLink := strings.TrimSpace(env.NextLink)
	visited := map[string]bool{}

	for nextLink != "" {
		if visited[nextLink] {
			return nil, fmt.Errorf("%w: cyclic continuation link %q in roleManagementPolicyAssignments", errUtils.ErrAzurePIMRequestFailed, nextLink)
		}
		visited[nextLink] = true

		rawPage, err := c.doContinuation(ctx, nextLink)
		if err != nil {
			return nil, err
		}
		var pageEnv armPolicyAssignmentEnvelope
		if err := json.Unmarshal(rawPage, &pageEnv); err != nil {
			return nil, fmt.Errorf("%w: decoding roleManagementPolicyAssignments page: %w", errUtils.ErrAzurePIMRequestFailed, err)
		}
		items = append(items, pageEnv.Value...)
		nextLink = strings.TrimSpace(pageEnv.NextLink)
	}

	return items, nil
}

// parseISO8601Duration parses an ISO-8601 duration string into a time.Duration.
// It supports day (D), hour (H), minute (M), and second (S) designators, as well as
// week (W), month (M), and year (Y).
func parseISO8601Duration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty ISO-8601 duration")
	}
	if !strings.HasPrefix(s, "P") && !strings.HasPrefix(s, "p") {
		return 0, fmt.Errorf("invalid ISO-8601 duration %q: missing 'P' prefix", s)
	}
	s = s[1:]
	if s == "" {
		return 0, errors.New("invalid ISO-8601 duration: empty after 'P'")
	}

	var datePart, timePart string
	tIdx := strings.IndexAny(s, "Tt")
	if tIdx >= 0 {
		datePart = s[:tIdx]
		timePart = s[tIdx+1:]
		if timePart == "" && datePart == "" {
			return 0, errors.New("invalid ISO-8601 duration: no designators found")
		}
	} else {
		datePart = s
	}

	var total time.Duration
	foundDesignator := false

	if datePart != "" {
		cur := ""
		for _, r := range datePart {
			switch {
			case (r >= '0' && r <= '9') || r == '.':
				cur += string(r)
			case r == 'Y' || r == 'y':
				val, err := strconv.ParseFloat(cur, 64)
				if err != nil || cur == "" {
					return 0, fmt.Errorf("invalid ISO-8601 duration: bad year in %q", datePart)
				}
				total += time.Duration(val * 365 * 24 * float64(time.Hour))
				cur = ""
				foundDesignator = true
			case r == 'M' || r == 'm':
				val, err := strconv.ParseFloat(cur, 64)
				if err != nil || cur == "" {
					return 0, fmt.Errorf("invalid ISO-8601 duration: bad month in %q", datePart)
				}
				total += time.Duration(val * 30 * 24 * float64(time.Hour))
				cur = ""
				foundDesignator = true
			case r == 'W' || r == 'w':
				val, err := strconv.ParseFloat(cur, 64)
				if err != nil || cur == "" {
					return 0, fmt.Errorf("invalid ISO-8601 duration: bad week in %q", datePart)
				}
				total += time.Duration(val * 7 * 24 * float64(time.Hour))
				cur = ""
				foundDesignator = true
			case r == 'D' || r == 'd':
				val, err := strconv.ParseFloat(cur, 64)
				if err != nil || cur == "" {
					return 0, fmt.Errorf("invalid ISO-8601 duration: bad day in %q", datePart)
				}
				total += time.Duration(val * 24 * float64(time.Hour))
				cur = ""
				foundDesignator = true
			default:
				return 0, fmt.Errorf("invalid ISO-8601 duration character %q in date part", r)
			}
		}
		if cur != "" {
			return 0, fmt.Errorf("invalid ISO-8601 duration: unparsed number %q in date part", cur)
		}
	}

	if timePart != "" {
		cur := ""
		for _, r := range timePart {
			switch {
			case (r >= '0' && r <= '9') || r == '.':
				cur += string(r)
			case r == 'H' || r == 'h':
				val, err := strconv.ParseFloat(cur, 64)
				if err != nil || cur == "" {
					return 0, fmt.Errorf("invalid ISO-8601 duration: bad hour in %q", timePart)
				}
				total += time.Duration(val * float64(time.Hour))
				cur = ""
				foundDesignator = true
			case r == 'M' || r == 'm':
				val, err := strconv.ParseFloat(cur, 64)
				if err != nil || cur == "" {
					return 0, fmt.Errorf("invalid ISO-8601 duration: bad minute in %q", timePart)
				}
				total += time.Duration(val * float64(time.Minute))
				cur = ""
				foundDesignator = true
			case r == 'S' || r == 's':
				val, err := strconv.ParseFloat(cur, 64)
				if err != nil || cur == "" {
					return 0, fmt.Errorf("invalid ISO-8601 duration: bad second in %q", timePart)
				}
				total += time.Duration(val * float64(time.Second))
				cur = ""
				foundDesignator = true
			default:
				return 0, fmt.Errorf("invalid ISO-8601 duration character %q in time part", r)
			}
		}
		if cur != "" {
			return 0, fmt.Errorf("invalid ISO-8601 duration: unparsed number %q in time part", cur)
		}
	}

	if !foundDesignator {
		return 0, errors.New("invalid ISO-8601 duration: no valid designators found")
	}

	return total, nil
}
