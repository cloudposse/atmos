package exec

import (
	"github.com/cloudposse/atmos/pkg/naming"
	"github.com/cloudposse/atmos/pkg/perf"
)

// NamingResolver supplies the existing template engine to shared naming rules.
func NamingResolver() naming.Resolver {
	defer perf.Track(nil, "exec.NamingResolver")()

	return naming.Resolver{Template: ProcessTmpl, StackTemplate: processStackNameTemplate}
}
