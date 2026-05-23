package security

import (
	"time"

	"github.com/google/uuid"

	"github.com/cloudposse/atmos/pkg/perf"
)

// buildComplianceOCSFEvents converts a ComplianceReport into OCSF 1.4.0
// Detection Finding events. Each failing control becomes one event with
// finding_info.types set to ["Compliance", <framework>] and the control ID
// surfaced via finding_info.tags + unmapped["atmos.compliance"]. Ordering
// matches the SARIF compliance renderer (severity desc, ControlID, Title,
// Stack, Component) for byte-stable output.
func buildComplianceOCSFEvents(report *ComplianceReport) []OCSFEvent {
	defer perf.Track(nil, "security.buildComplianceOCSFEvents")()

	if report == nil || len(report.FailingDetails) == 0 {
		return []OCSFEvent{}
	}

	details := sortedComplianceDetails(report.FailingDetails)
	correlationUID := complianceCorrelationUID(report)
	profiles := []string{"cloud"}

	events := make([]OCSFEvent, 0, len(details))
	for i := range details {
		events = append(events, buildComplianceOCSFEvent(&details[i], report, correlationUID, profiles))
	}
	return events
}

// buildComplianceOCSFEvent projects a failing compliance control onto an OCSF
// event. Compliance reports have no source AWS finding behind them, so most
// per-finding fields (cloud account, resources, vulnerabilities) are empty —
// we only populate what the ComplianceReport carries.
func buildComplianceOCSFEvent(ctrl *ComplianceControl, report *ComplianceReport, correlationUID string, profiles []string) OCSFEvent {
	now := report.GeneratedAt
	return OCSFEvent{
		ActivityID:   ocsfActivityCreate,
		ActivityName: ocsfActivityName(ocsfActivityCreate),
		CategoryUID:  ocsfCategoryUID,
		CategoryName: ocsfCategoryName,
		ClassUID:     ocsfClassUID,
		ClassName:    ocsfClassName,
		TypeUID:      ocsfTypeUIDCreate,
		TypeName:     ocsfClassName + ": Create",
		Severity:     string(ctrl.Severity),
		SeverityID:   ocsfSeverityID(ctrl.Severity),
		Status:       "New",
		StatusID:     ocsfStatusNew,
		Time:         ocsfMillis(now),
		Metadata:     buildOCSFMetadata(correlationUID, profiles),
		FindingInfo:  buildComplianceFindingInfo(ctrl, report),
		Cloud:        OCSFCloud{Provider: ocsfCloudProvider},
		Remediation:  buildComplianceRemediation(ctrl),
		Enrichments:  buildComplianceEnrichments(ctrl),
		Unmapped:     buildComplianceUnmapped(ctrl, report),
	}
}

// buildComplianceFindingInfo packs the control identity into finding_info.
func buildComplianceFindingInfo(ctrl *ComplianceControl, report *ComplianceReport) OCSFFindingInfo {
	id := ctrl.ControlID
	if id == "" {
		id = slugify(ctrl.Title)
	}
	info := OCSFFindingInfo{
		UID:   id,
		Title: ctrl.Title,
		Types: []string{"Compliance"},
	}
	if report.FrameworkTitle != "" {
		info.Types = append(info.Types, report.FrameworkTitle)
	} else if report.Framework != "" {
		info.Types = append(info.Types, report.Framework)
	}
	if ctrl.ControlID != "" {
		info.Tags = append(info.Tags, OCSFKeyValue{Name: "atmos.compliance_control_id", Value: ctrl.ControlID})
	}
	return info
}

// buildComplianceEnrichments mirrors the SARIF compliance result properties
// (stack, component) as OCSF enrichments so downstream consumers can pivot.
func buildComplianceEnrichments(ctrl *ComplianceControl) []OCSFEnrichment {
	var out []OCSFEnrichment
	out = appendEnrichmentString(out, "atmos.stack", ctrl.Stack)
	out = appendEnrichmentString(out, "atmos.component", ctrl.Component)
	return out
}

// buildComplianceRemediation surfaces AI remediation Description on the native
// OCSF remediation block when present (it's the only natural source of
// remediation text for compliance reports).
func buildComplianceRemediation(ctrl *ComplianceControl) *OCSFRemediation {
	if ctrl.Remediation == nil || ctrl.Remediation.Description == "" {
		return nil
	}
	rem := &OCSFRemediation{Desc: ctrl.Remediation.Description}
	if len(ctrl.Remediation.References) > 0 {
		rem.References = ctrl.Remediation.References
	}
	return rem
}

// buildComplianceUnmapped carries the framework identity and full AI
// remediation detail under the atmos.* namespace.
func buildComplianceUnmapped(ctrl *ComplianceControl, report *ComplianceReport) map[string]any {
	out := map[string]any{}
	compliance := map[string]any{}
	if ctrl.ControlID != "" {
		compliance["control"] = ctrl.ControlID
	}
	if report.Framework != "" {
		compliance["framework"] = report.Framework
	}
	if report.FrameworkTitle != "" {
		compliance["framework_title"] = report.FrameworkTitle
	}
	if len(compliance) > 0 {
		out["atmos.compliance"] = compliance
	}
	addAIRemediationFromControl(out, ctrl)
	if len(out) == 0 {
		return nil
	}
	return out
}

func addAIRemediationFromControl(out map[string]any, ctrl *ComplianceControl) {
	if ctrl.Remediation == nil {
		return
	}
	r := ctrl.Remediation
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

// complianceCorrelationUID derives a deterministic UUID per compliance report.
func complianceCorrelationUID(report *ComplianceReport) string {
	if report == nil {
		return uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://atmos.tools/ocsf/compliance/empty")).String()
	}
	seed := "atmos/ocsf/compliance/" + report.Framework + "/" + report.Stack + "/" + report.GeneratedAt.UTC().Format(time.RFC3339Nano)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed)).String()
}
