package helm

import (
	"fmt"
	"path/filepath"

	chartv2 "helm.sh/helm/v4/pkg/chart/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ValidateAdoptionInputs loads a local chart and its overlays using the same
// readers as a named render. No dependencies are fetched during registration.
func ValidateAdoptionInputs(chartDir string, valuesFiles []string) error {
	defer perf.Track(nil, "helm.ValidateAdoptionInputs")()

	loaded, err := loadChart(chartDir)
	if err != nil {
		return err
	}
	chart, ok := loaded.(*chartv2.Chart)
	if !ok {
		return fmt.Errorf("%w: stack add supports application charts with apiVersion v1 or v2", errUtils.ErrInvalidComponent)
	}
	if chart.Metadata.Type == "library" {
		return fmt.Errorf("%w: select an application chart; %s is a library", errUtils.ErrInvalidComponent, chartDir)
	}
	if crds := chart.CRDObjects(); len(crds) > 0 {
		return fmt.Errorf("%w: %s supplies %s, which Atmos renders and helm template omits by default", errUtils.ErrInvalidComponent, chartDir, crds[0].Filename)
	}
	for _, value := range valuesFiles {
		if _, err := loadValuesFile(filepath.Join(chartDir, filepath.FromSlash(value))); err != nil {
			return err
		}
	}
	return nil
}
