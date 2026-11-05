package doctor

import (
	"os"

	"github.com/cloudposse/atmos/pkg/validation"
)

// missingSource reports a source that does not exist and returns nil, so discovery continues; any
// other read error is returned.
func (p *project) missingSource(o *Options, r *Report, kind, rel string, err error) error {
	if !os.IsNotExist(err) {
		return err
	}
	p.limitKind(kind, "", 0, "Local source does not exist: "+rel)
	addFinding(o, r, &Finding{RuleID: ruleImports, Category: categoryValidation, Severity: validation.SeverityError, File: rel, Message: "Local source does not exist", Status: statusManual})
	return nil
}

// recordParseFailure reports a document that does not parse. The first configuration file that does
// not parse is remembered, because its base_path is then unknown.
func (p *project) recordParseFailure(o *Options, r *Report, d *document, rel string, err error) {
	if d.kind == documentConfig && p.unparsedConfig == "" {
		p.unparsedConfig = d.path
	}
	p.limitKind(d.kind, d.path, 0, "Configuration could not be parsed")
	addFinding(o, r, parseFinding(rel, err))
	d.node = nil
}
