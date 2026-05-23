//nolint:revive // OCSF model and renderer types stay together to mirror the 1.4 schema.
package security

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// OCSF schema constants. We emit Detection Finding (class 2004) from the OCSF
// 1.4.0 schema with the cloud + vulnerability profiles active. Detection
// Finding does not expose a top-level compliance object on 2004, so compliance
// standards are flattened into finding_info.types[] and full id/name/version
// detail is preserved via the unmapped extension.
//
// Spec: https://schema.ocsf.io/1.4.0/classes/detection_finding
const (
	ocsfVersion       = "1.4.0"
	ocsfClassUID      = 2004
	ocsfClassName     = "Detection Finding"
	ocsfCategoryUID   = 2
	ocsfCategoryName  = "Findings"
	ocsfTypeUIDCreate = 200401
	ocsfTypeUIDUpdate = 200402
	ocsfTypeUIDClose  = 200403

	ocsfActivityCreate = 1
	ocsfActivityUpdate = 2
	ocsfActivityClose  = 3

	// Severity ID mapping (OCSF dictionary: 0=Unknown, 1=Informational, 2=Low,
	// 3=Medium, 4=High, 5=Critical, 6=Fatal, 99=Other).
	ocsfSeverityInformational = 1
	ocsfSeverityLow           = 2
	ocsfSeverityMedium        = 3
	ocsfSeverityHigh          = 4
	ocsfSeverityCritical      = 5
	ocsfSeverityOther         = 99

	// Status ID mapping (OCSF dictionary: 0=Unknown, 1=New, 2=In Progress,
	// 3=Suppressed, 4=Resolved, 99=Other).
	ocsfStatusNew        = 1
	ocsfStatusInProgress = 2
	ocsfStatusSuppressed = 3
	ocsfStatusResolved   = 4
	ocsfStatusOther      = 99

	ocsfCloudProvider = "AWS"
	ocsfProductName   = "atmos"
	ocsfProductVendor = "Cloud Posse"
	ocsfProductURL    = "https://atmos.tools"
	ocsfFloatBitSize  = 64
)

// OCSFEvent is one top-level OCSF Detection Finding event.
type OCSFEvent struct {
	ActivityID      int              `json:"activity_id"`
	ActivityName    string           `json:"activity_name,omitempty"`
	CategoryUID     int              `json:"category_uid"`
	CategoryName    string           `json:"category_name,omitempty"`
	ClassUID        int              `json:"class_uid"`
	ClassName       string           `json:"class_name,omitempty"`
	TypeUID         int              `json:"type_uid"`
	TypeName        string           `json:"type_name,omitempty"`
	Severity        string           `json:"severity,omitempty"`
	SeverityID      int              `json:"severity_id"`
	Status          string           `json:"status,omitempty"`
	StatusID        int              `json:"status_id,omitempty"`
	StatusCode      string           `json:"status_code,omitempty"`
	StatusDetail    string           `json:"status_detail,omitempty"`
	Time            int64            `json:"time"`
	StartTime       int64            `json:"start_time,omitempty"`
	EndTime         int64            `json:"end_time,omitempty"`
	ModifiedTime    int64            `json:"modified_time,omitempty"`
	Metadata        OCSFMetadata     `json:"metadata"`
	FindingInfo     OCSFFindingInfo  `json:"finding_info"`
	Cloud           OCSFCloud        `json:"cloud"`
	Resources       []OCSFResource   `json:"resources,omitempty"`
	Vulnerabilities []OCSFVuln       `json:"vulnerabilities,omitempty"`
	Remediation     *OCSFRemediation `json:"remediation,omitempty"`
	Enrichments     []OCSFEnrichment `json:"enrichments,omitempty"`
	Unmapped        map[string]any   `json:"unmapped,omitempty"`
}

// OCSFMetadata identifies the producer and the batch.
type OCSFMetadata struct {
	Version        string      `json:"version"`
	Product        OCSFProduct `json:"product"`
	CorrelationUID string      `json:"correlation_uid,omitempty"`
	Profiles       []string    `json:"profiles,omitempty"`
}

// OCSFProduct identifies the tool that produced the finding.
type OCSFProduct struct {
	Name       string `json:"name,omitempty"`
	VendorName string `json:"vendor_name,omitempty"`
	Version    string `json:"version,omitempty"`
	URLString  string `json:"url_string,omitempty"`
}

// OCSFFindingInfo describes the finding itself. ProductUID identifies the
// upstream AWS service that detected the finding (security-hub, config,
// inspector, guardduty, macie, access-analyzer) — orthogonal to
// metadata.product which identifies atmos as the event producer.
type OCSFFindingInfo struct {
	UID        string         `json:"uid"`
	Title      string         `json:"title,omitempty"`
	Desc       string         `json:"desc,omitempty"`
	SrcURL     string         `json:"src_url,omitempty"`
	ProductUID string         `json:"product_uid,omitempty"`
	Types      []string       `json:"types,omitempty"`
	Tags       []OCSFKeyValue `json:"tags,omitempty"`
}

// OCSFCloud describes the cloud environment the finding originated in.
type OCSFCloud struct {
	Provider string       `json:"provider"`
	Region   string       `json:"region,omitempty"`
	Account  *OCSFAccount `json:"account,omitempty"`
}

// OCSFAccount identifies a cloud account.
type OCSFAccount struct {
	UID  string `json:"uid,omitempty"`
	Name string `json:"name,omitempty"`
}

// OCSFResource describes an affected resource.
type OCSFResource struct {
	UID    string         `json:"uid,omitempty"`
	Type   string         `json:"type,omitempty"`
	Region string         `json:"region,omitempty"`
	Tags   []OCSFKeyValue `json:"tags,omitempty"`
	Labels []string       `json:"labels,omitempty"`
}

// OCSFKeyValue mirrors the OCSF key_value_object pattern used by tags.
type OCSFKeyValue struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

// OCSFVuln models the OCSF Vulnerability object. The OCSF schema requires
// exactly one of cve/cwe/advisory — we prefer cve when available, otherwise
// cwe, otherwise we omit the whole vulnerabilities[] entry.
type OCSFVuln struct {
	Desc             string                `json:"desc,omitempty"`
	Title            string                `json:"title,omitempty"`
	Severity         string                `json:"severity,omitempty"`
	IsFixAvailable   *bool                 `json:"is_fix_available,omitempty"`
	FixAvailable     string                `json:"fix_available,omitempty"`
	CVE              *OCSFCVE              `json:"cve,omitempty"`
	CWE              *OCSFCWE              `json:"cwe,omitempty"`
	AffectedPackages []OCSFAffectedPackage `json:"affected_packages,omitempty"`
	References       []string              `json:"references,omitempty"`
}

// OCSFCVE captures CVE detail including CVSS scores and EPSS probability.
type OCSFCVE struct {
	UID        string     `json:"uid"`
	Desc       string     `json:"desc,omitempty"`
	Title      string     `json:"title,omitempty"`
	CWEUID     string     `json:"cwe_uid,omitempty"`
	CVSS       []OCSFCVSS `json:"cvss,omitempty"`
	EPSS       *OCSFEPSS  `json:"epss,omitempty"`
	References []string   `json:"references,omitempty"`
}

// OCSFCWE captures a CWE classification.
type OCSFCWE struct {
	UID     string `json:"uid"`
	Caption string `json:"caption,omitempty"`
	SrcURL  string `json:"src_url,omitempty"`
}

// OCSFCVSS is a single CVSS score.
type OCSFCVSS struct {
	BaseScore    float64 `json:"base_score"`
	Version      string  `json:"version"`
	VectorString string  `json:"vector_string,omitempty"`
}

// OCSFEPSS is the EPSS probability for a CVE. OCSF 1.4.0 schemas the score as
// a string (https://schema.ocsf.io/1.4.0/objects/epss); we honor the spec
// even though the value is a float, so we format on the way out.
type OCSFEPSS struct {
	Score string `json:"score"`
}

// OCSFAffectedPackage describes a vulnerable software package.
type OCSFAffectedPackage struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	FixedInVersion string `json:"fixed_in_version,omitempty"`
	PackageManager string `json:"package_manager,omitempty"`
	Path           string `json:"path,omitempty"`
	Remediation    string `json:"remediation,omitempty"`
}

// OCSFRemediation carries native remediation guidance.
type OCSFRemediation struct {
	Desc       string   `json:"desc"`
	References []string `json:"references,omitempty"`
}

// OCSFEnrichment is an Atmos extension carried under the OCSF enrichments[]
// array. The schema requires name, value, and data — we set data to the same
// (string-coerced) value, which keeps consumers that ignore data correct and
// gives structured consumers a useful payload.
type OCSFEnrichment struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Data     any    `json:"data"`
	Type     string `json:"type,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// ocsfRenderer renders security/compliance reports as an OCSF 1.4.0 JSON array.
type ocsfRenderer struct{}

// RenderSecurityReport implements ReportRenderer.
func (r *ocsfRenderer) RenderSecurityReport(w io.Writer, report *Report) error {
	defer perf.Track(nil, "security.ocsfRenderer.RenderSecurityReport")()

	events := BuildOCSFEvents(report)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(events)
}

// RenderComplianceReport implements ReportRenderer.
func (r *ocsfRenderer) RenderComplianceReport(w io.Writer, report *ComplianceReport) error {
	defer perf.Track(nil, "security.ocsfRenderer.RenderComplianceReport")()

	events := buildComplianceOCSFEvents(report)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(events)
}

// BuildOCSFEvents converts a security Report into OCSF 1.4.0 Detection Finding
// events. Findings are sorted with the same deterministic ordering SARIF uses
// so output is byte-stable for the same input report.
func BuildOCSFEvents(report *Report) []OCSFEvent {
	defer perf.Track(nil, "security.BuildOCSFEvents")()

	if report == nil || len(report.Findings) == 0 {
		return []OCSFEvent{}
	}

	findings := sortedFindings(report.Findings)
	correlationUID := stableCorrelationUID(report)
	profiles := ocsfProfilesForFindings(findings)

	events := make([]OCSFEvent, 0, len(findings))
	for i := range findings {
		events = append(events, buildOCSFEvent(&findings[i], correlationUID, profiles))
	}
	return events
}

// buildOCSFEvent projects a single Finding onto an OCSF event.
func buildOCSFEvent(f *Finding, correlationUID string, profiles []string) OCSFEvent {
	activityID, typeUID := ocsfActivityAndType(f)
	return OCSFEvent{
		ActivityID:      activityID,
		ActivityName:    ocsfActivityName(activityID),
		CategoryUID:     ocsfCategoryUID,
		CategoryName:    ocsfCategoryName,
		ClassUID:        ocsfClassUID,
		ClassName:       ocsfClassName,
		TypeUID:         typeUID,
		TypeName:        ocsfClassName + ": " + ocsfActivityName(activityID),
		Severity:        string(f.Severity),
		SeverityID:      ocsfSeverityID(f.Severity),
		Status:          ocsfStatusName(f),
		StatusID:        ocsfStatusID(f),
		StatusCode:      ocsfStatusCode(f),
		StatusDetail:    ocsfStatusDetail(f),
		Time:            ocsfMillis(f.CreatedAt),
		StartTime:       ocsfMillisFromPtr(timestampsFirstObserved(f)),
		EndTime:         ocsfMillisFromPtr(timestampsLastObserved(f)),
		ModifiedTime:    ocsfMillis(f.UpdatedAt),
		Metadata:        buildOCSFMetadata(correlationUID, profiles),
		FindingInfo:     buildOCSFFindingInfo(f),
		Cloud:           buildOCSFCloud(f),
		Resources:       buildOCSFResources(f),
		Vulnerabilities: buildOCSFVulnerabilities(f),
		Remediation:     buildOCSFRemediation(f),
		Enrichments:     buildOCSFEnrichments(f),
		Unmapped:        buildOCSFUnmapped(f),
	}
}

// buildOCSFMetadata stamps every event in a batch with shared identity so
// SIEMs can group the run.
func buildOCSFMetadata(correlationUID string, profiles []string) OCSFMetadata {
	return OCSFMetadata{
		Version: ocsfVersion,
		Product: OCSFProduct{
			Name:       ocsfProductName,
			VendorName: ocsfProductVendor,
			Version:    version.Version,
			URLString:  ocsfProductURL,
		},
		CorrelationUID: correlationUID,
		Profiles:       profiles,
	}
}

// buildOCSFFindingInfo emits finding_info. Compliance standards are flattened
// into types[] (one per standard's display name) because OCSF 2004 has no
// top-level compliance object; full id/name/version is preserved via unmapped.
func buildOCSFFindingInfo(f *Finding) OCSFFindingInfo {
	info := OCSFFindingInfo{
		UID:        f.ID,
		Title:      f.Title,
		Desc:       f.Description,
		SrcURL:     f.SourceURL,
		ProductUID: string(f.Source),
	}
	for _, std := range findingComplianceStandards(f) {
		caption := complianceTaxonomyName(std)
		if caption != "" {
			info.Types = append(info.Types, caption)
		}
	}
	if f.SecurityControlID != "" {
		info.Tags = append(info.Tags, OCSFKeyValue{Name: "atmos.security_control_id", Value: f.SecurityControlID})
	}
	return info
}

// buildOCSFCloud emits the cloud block. The OCSF schema requires provider, so
// even unmapped findings carry provider="AWS".
func buildOCSFCloud(f *Finding) OCSFCloud {
	cloud := OCSFCloud{Provider: ocsfCloudProvider, Region: f.Region}
	if f.AccountID != "" {
		cloud.Account = &OCSFAccount{UID: f.AccountID}
	}
	return cloud
}

// buildOCSFResources emits the affected resource (one per finding in v1).
func buildOCSFResources(f *Finding) []OCSFResource {
	if f.ResourceARN == "" && f.ResourceType == "" && len(f.ResourceTags) == 0 {
		return nil
	}
	res := OCSFResource{
		UID:    f.ResourceARN,
		Type:   f.ResourceType,
		Region: f.Region,
	}
	for k, v := range f.ResourceTags {
		res.Tags = append(res.Tags, OCSFKeyValue{Name: k, Value: v})
	}
	// Stable tag ordering (maps iterate randomly) so output is byte-stable.
	sortKVByName(res.Tags)
	return []OCSFResource{res}
}

// buildOCSFRemediation maps AWS-supplied SourceRemediation onto the native
// OCSF remediation block. AI-generated Remediation goes into unmapped.
func buildOCSFRemediation(f *Finding) *OCSFRemediation {
	if f.SourceRemediation == nil || f.SourceRemediation.Text == "" {
		return nil
	}
	rem := &OCSFRemediation{Desc: f.SourceRemediation.Text}
	if f.SourceRemediation.URL != "" {
		rem.References = []string{f.SourceRemediation.URL}
	}
	return rem
}

// buildOCSFVulnerabilities maps the Atmos VulnerabilityDetails onto the OCSF
// vulnerabilities[] array. The schema requires oneOf(cve, cwe, advisory) per
// entry — we prefer cve, fall back to cwe; entries without either are dropped.
func buildOCSFVulnerabilities(f *Finding) []OCSFVuln {
	if f.Vulnerability == nil {
		return nil
	}
	v := f.Vulnerability
	cve := buildOCSFCVE(v)
	cwe := buildOCSFCWE(v)
	if cve == nil && cwe == nil {
		return nil
	}
	vuln := OCSFVuln{
		CVE:              cve,
		CWE:              cwe,
		AffectedPackages: buildOCSFAffectedPackages(v),
		References:       v.ReferenceURLs,
	}
	if v.FixedInVersion != "" {
		vuln.FixAvailable = v.FixedInVersion
		t := true
		vuln.IsFixAvailable = &t
	}
	return []OCSFVuln{vuln}
}

func buildOCSFCVE(v *VulnerabilityDetails) *OCSFCVE {
	if v.CVEID == "" {
		return nil
	}
	cve := &OCSFCVE{UID: v.CVEID}
	if v.EPSSScore > 0 {
		cve.EPSS = &OCSFEPSS{Score: formatEPSSScore(v.EPSSScore)}
	}
	if len(v.CWEIDs) > 0 {
		cve.CWEUID = v.CWEIDs[0]
	}
	for _, c := range v.CVSS {
		if c.BaseScore == 0 || c.Version == "" {
			continue
		}
		cve.CVSS = append(cve.CVSS, OCSFCVSS{
			BaseScore:    c.BaseScore,
			Version:      c.Version,
			VectorString: c.Vector,
		})
	}
	cve.References = v.ReferenceURLs
	return cve
}

func buildOCSFCWE(v *VulnerabilityDetails) *OCSFCWE {
	if v.CVEID != "" || len(v.CWEIDs) == 0 {
		return nil
	}
	return &OCSFCWE{UID: v.CWEIDs[0]}
}

func buildOCSFAffectedPackages(v *VulnerabilityDetails) []OCSFAffectedPackage {
	if len(v.Packages) > 0 {
		out := make([]OCSFAffectedPackage, 0, len(v.Packages))
		for _, p := range v.Packages {
			if p.Name == "" || p.Version == "" {
				continue
			}
			out = append(out, OCSFAffectedPackage{
				Name:           p.Name,
				Version:        p.Version,
				FixedInVersion: p.FixedInVersion,
				PackageManager: p.PackageManager,
				Path:           p.FilePath,
				Remediation:    p.Remediation,
			})
		}
		return out
	}
	if v.PackageName == "" || v.PackageVersion == "" {
		return nil
	}
	return []OCSFAffectedPackage{{
		Name:           v.PackageName,
		Version:        v.PackageVersion,
		FixedInVersion: v.FixedInVersion,
	}}
}

// buildOCSFEnrichments surfaces Atmos's component/stack mapping as first-class
// OCSF enrichments so SIEMs that understand them can pivot by stack/component.
func buildOCSFEnrichments(f *Finding) []OCSFEnrichment {
	if f.Mapping == nil {
		return nil
	}
	m := f.Mapping
	var out []OCSFEnrichment
	out = appendEnrichmentString(out, "atmos.stack", m.Stack)
	out = appendEnrichmentString(out, "atmos.component", m.Component)
	out = appendEnrichmentString(out, "atmos.component_path", m.ComponentPath)
	out = appendEnrichmentString(out, "atmos.workspace", m.Workspace)
	out = appendEnrichmentString(out, "atmos.mapping.confidence", string(m.Confidence))
	out = appendEnrichmentString(out, "atmos.mapping.method", m.Method)
	out = append(out, OCSFEnrichment{
		Name:     "atmos.mapping.mapped",
		Value:    boolString(m.Mapped),
		Data:     m.Mapped,
		Type:     "boolean",
		Provider: "atmos",
	})
	return out
}

// buildOCSFUnmapped carries Atmos extension data and source metadata that has
// no native OCSF 1.4.0 home on the Detection Finding class.
func buildOCSFUnmapped(f *Finding) map[string]any {
	out := make(map[string]any)
	addCompliance(out, f)
	addSourceSeverity(out, f)
	addSourceLifecycle(out, f)
	addAIRemediation(out, f)
	addVulnerabilityOverflow(out, f)
	if len(out) == 0 {
		return nil
	}
	return out
}

func addCompliance(out map[string]any, f *Finding) {
	standards := findingComplianceStandards(f)
	if len(standards) == 0 && f.SecurityControlID == "" {
		return
	}
	compliance := map[string]any{}
	if f.SecurityControlID != "" {
		compliance["control"] = f.SecurityControlID
	}
	if len(standards) > 0 {
		entries := make([]map[string]string, 0, len(standards))
		for _, s := range standards {
			entry := compactStringMap(map[string]string{
				"id":      s.ID,
				"name":    s.Name,
				"version": s.Version,
			})
			if entry != nil {
				entries = append(entries, entry)
			}
		}
		if len(entries) > 0 {
			compliance["standards"] = entries
		}
	}
	if len(compliance) > 0 {
		out["atmos.compliance"] = compliance
	}
}

func addSourceSeverity(out map[string]any, f *Finding) {
	if f.SourceSeverity == nil {
		return
	}
	sev := compactStringMap(map[string]string{
		"label": f.SourceSeverity.Label,
	})
	if sev == nil {
		sev = map[string]string{}
	}
	wrapped := map[string]any{}
	for k, v := range sev {
		wrapped[k] = v
	}
	if f.SourceSeverity.Score != nil {
		wrapped["score"] = *f.SourceSeverity.Score
	}
	if len(wrapped) > 0 {
		out["atmos.source_severity"] = wrapped
	}
}

func addSourceLifecycle(out map[string]any, f *Finding) {
	if f.SourceLifecycle == nil {
		return
	}
	lc := compactStringMap(map[string]string{
		"workflow_status":   f.SourceLifecycle.WorkflowStatus,
		"record_state":      f.SourceLifecycle.RecordState,
		"compliance_status": f.SourceLifecycle.ComplianceStatus,
		"inspector_status":  f.SourceLifecycle.InspectorStatus,
	})
	if lc != nil {
		out["atmos.source_lifecycle"] = lc
	}
}

func addAIRemediation(out map[string]any, f *Finding) {
	if f.Remediation == nil {
		return
	}
	r := f.Remediation
	rem := map[string]any{}
	addNonEmpty(rem, "description", r.Description)
	addNonEmpty(rem, "root_cause", r.RootCause)
	addNonEmpty(rem, "stack_changes", r.StackChanges)
	addNonEmpty(rem, "deploy_command", r.DeployCommand)
	addNonEmpty(rem, "risk_level", r.RiskLevel)
	if len(r.Steps) > 0 {
		rem["steps"] = r.Steps
	}
	if len(r.CodeChanges) > 0 {
		rem["code_changes"] = r.CodeChanges
	}
	if len(r.References) > 0 {
		rem["references"] = r.References
	}
	if len(rem) > 0 {
		out["atmos.remediation"] = rem
	}
}

func addVulnerabilityOverflow(out map[string]any, f *Finding) {
	if f.Vulnerability == nil || len(f.Vulnerability.CWEIDs) <= 1 {
		return
	}
	out["atmos.vulnerability.cwe_ids"] = f.Vulnerability.CWEIDs
}

// ocsfActivityAndType decides activity_id and type_uid for a finding.
func ocsfActivityAndType(f *Finding) (int, int) {
	if isArchived(f) {
		return ocsfActivityClose, ocsfTypeUIDClose
	}
	if !f.UpdatedAt.IsZero() && f.UpdatedAt.After(f.CreatedAt) {
		return ocsfActivityUpdate, ocsfTypeUIDUpdate
	}
	return ocsfActivityCreate, ocsfTypeUIDCreate
}

func ocsfActivityName(id int) string {
	switch id {
	case ocsfActivityCreate:
		return "Create"
	case ocsfActivityUpdate:
		return "Update"
	case ocsfActivityClose:
		return "Close"
	}
	return "Unknown"
}

// ocsfSeverityID maps Atmos severity strings to the OCSF dictionary.
func ocsfSeverityID(s Severity) int {
	switch s {
	case SeverityCritical:
		return ocsfSeverityCritical
	case SeverityHigh:
		return ocsfSeverityHigh
	case SeverityMedium:
		return ocsfSeverityMedium
	case SeverityLow:
		return ocsfSeverityLow
	case SeverityInformational:
		return ocsfSeverityInformational
	}
	return ocsfSeverityOther
}

// ocsfStatusID translates SourceLifecycle workflow state into the OCSF status
// dictionary. Defaults to New when no source state is available.
func ocsfStatusID(f *Finding) int {
	if f.SourceLifecycle == nil {
		return ocsfStatusNew
	}
	switch strings.ToUpper(f.SourceLifecycle.WorkflowStatus) {
	case "", "NEW":
		return ocsfStatusNew
	case "NOTIFIED", "IN_PROGRESS":
		return ocsfStatusInProgress
	case "SUPPRESSED":
		return ocsfStatusSuppressed
	case "RESOLVED":
		return ocsfStatusResolved
	}
	return ocsfStatusOther
}

func ocsfStatusName(f *Finding) string {
	switch ocsfStatusID(f) {
	case ocsfStatusNew:
		return "New"
	case ocsfStatusInProgress:
		return "In Progress"
	case ocsfStatusSuppressed:
		return "Suppressed"
	case ocsfStatusResolved:
		return "Resolved"
	}
	return "Other"
}

func ocsfStatusCode(f *Finding) string {
	if f.SourceLifecycle == nil {
		return ""
	}
	return f.SourceLifecycle.WorkflowStatus
}

func ocsfStatusDetail(f *Finding) string {
	if f.SourceLifecycle == nil {
		return ""
	}
	return f.SourceLifecycle.RecordState
}

func isArchived(f *Finding) bool {
	return f.SourceLifecycle != nil && strings.EqualFold(f.SourceLifecycle.RecordState, "ARCHIVED")
}

// ocsfProfilesForFindings returns the OCSF profiles active for a batch. The
// cloud profile is always on (we always emit cloud{}); vulnerability is added
// only if at least one finding carries Vulnerability data.
func ocsfProfilesForFindings(findings []Finding) []string {
	profiles := []string{"cloud"}
	for i := range findings {
		if findings[i].Vulnerability != nil {
			profiles = append(profiles, "vulnerability")
			break
		}
	}
	return profiles
}

// stableCorrelationUID derives a deterministic UUID for a batch so re-runs on
// the same report produce byte-identical output. SARIF uses a similar pattern
// for taxonomy GUIDs.
func stableCorrelationUID(report *Report) string {
	if report == nil {
		return uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://atmos.tools/ocsf/empty")).String()
	}
	seed := "atmos/ocsf/" + report.Stack + "/" + report.Component + "/" + report.GeneratedAt.UTC().Format(time.RFC3339Nano)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed)).String()
}

// ocsfMillis converts a time to OCSF epoch-milliseconds. Zero times become 0.
func ocsfMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// ocsfMillisFromPtr is the pointer variant for SourceTimestamps fields.
func ocsfMillisFromPtr(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return ocsfMillis(*t)
}

func timestampsFirstObserved(f *Finding) *time.Time {
	if f.SourceTimestamps == nil {
		return nil
	}
	return f.SourceTimestamps.FirstObservedAt
}

func timestampsLastObserved(f *Finding) *time.Time {
	if f.SourceTimestamps == nil {
		return nil
	}
	return f.SourceTimestamps.LastObservedAt
}

// appendEnrichmentString appends a string-valued enrichment only when non-empty.
func appendEnrichmentString(out []OCSFEnrichment, name, value string) []OCSFEnrichment {
	if value == "" {
		return out
	}
	return append(out, OCSFEnrichment{
		Name:     name,
		Value:    value,
		Data:     value,
		Type:     "string",
		Provider: "atmos",
	})
}

// formatEPSSScore renders the EPSS probability as a compact string. OCSF
// types EPSS.score as a string per the 1.4.0 spec.
func formatEPSSScore(score float64) string {
	return strconv.FormatFloat(score, 'f', -1, ocsfFloatBitSize)
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// sortKVByName sorts OCSF key-value pairs by name for deterministic output.
func sortKVByName(kvs []OCSFKeyValue) {
	for i := 1; i < len(kvs); i++ {
		for j := i; j > 0 && kvs[j-1].Name > kvs[j].Name; j-- {
			kvs[j-1], kvs[j] = kvs[j], kvs[j-1]
		}
	}
}
