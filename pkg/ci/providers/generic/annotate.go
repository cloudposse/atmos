package generic

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Ensure Provider implements provider.Annotator.
var _ provider.Annotator = (*Provider)(nil)

// Annotate renders each annotation as one line, locating it as path:line when known.
func (p *Provider) Annotate(annotations []provider.Annotation) error {
	defer perf.Track(nil, "generic.Provider.Annotate")()

	out := p.out()
	for i := range annotations {
		a := &annotations[i]
		line := formatAnnotation(a)
		switch a.Level {
		case provider.AnnotationError:
			out.Errorf("%s", line)
		case provider.AnnotationWarning:
			out.Warningf("%s", line)
		default:
			out.Infof("%s", line)
		}
	}
	return nil
}

// formatAnnotation builds "path:line: title: message", omitting empty parts.
func formatAnnotation(a *provider.Annotation) string {
	var location string
	switch {
	case a.Path != "" && a.StartLine > 0:
		location = fmt.Sprintf("%s:%d", a.Path, a.StartLine)
	case a.Path != "":
		location = a.Path
	}

	parts := make([]string, 0, 3)
	for _, part := range []string{location, a.Title, a.Message} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ": ")
}
