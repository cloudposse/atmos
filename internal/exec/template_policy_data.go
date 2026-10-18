package exec

import (
	"context"
	"fmt"

	"github.com/hairyhenderson/gomplate/v3/conv"
	"github.com/hairyhenderson/gomplate/v3/data"
	"github.com/hairyhenderson/gomplate/v3/funcs" //nolint:staticcheck // Gomplate v3 namespace API.
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

type policyDataFuncs struct {
	native *funcs.DataFuncs
	file   string
}

// guardedTemplateJSON prevents implicit EJSON key-file reads under source policies.
func guardedTemplateJSON(input any) (map[string]any, error) {
	enabled, err := guard.Enabled("source")
	if err != nil {
		return nil, err
	}
	value := conv.ToString(input)
	if !enabled {
		return data.JSON(value)
	}
	// Decode with YAML (a JSON superset) so whole numbers stay ints, matching native gomplate data.JSON.
	var result map[string]any
	if err := yaml.Unmarshal([]byte(value), &result); err != nil {
		return nil, err
	}
	if _, encrypted := result["_public_key"]; encrypted {
		return nil, fmt.Errorf("%w: EJSON key reads have no guarded collector", errUtils.ErrPolicyInputUnavailable)
	}
	return result, nil
}

func (f *policyDataFuncs) JSON(in any) (map[string]any, error) {
	defer perf.Track(nil, "exec.JSON")()
	if err := guard.Function(context.Background(), "data.JSON", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return guardedTemplateJSON(in)
}

func (f *policyDataFuncs) JSONArray(in any) ([]any, error) {
	defer perf.Track(nil, "exec.JSONArray")()
	if err := guard.Function(context.Background(), "data.JSONArray", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.JSONArray(in)
}

func (f *policyDataFuncs) YAML(in any) (map[string]any, error) {
	defer perf.Track(nil, "exec.YAML")()
	if err := guard.Function(context.Background(), "data.YAML", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.YAML(in)
}

func (f *policyDataFuncs) YAMLArray(in any) ([]any, error) {
	defer perf.Track(nil, "exec.YAMLArray")()
	if err := guard.Function(context.Background(), "data.YAMLArray", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.YAMLArray(in)
}

func (f *policyDataFuncs) TOML(in any) (any, error) {
	defer perf.Track(nil, "exec.TOML")()
	if err := guard.Function(context.Background(), "data.TOML", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.TOML(in)
}

func (f *policyDataFuncs) CSV(args ...string) ([][]string, error) {
	defer perf.Track(nil, "exec.CSV")()
	if err := guard.Function(context.Background(), "data.CSV", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.CSV(args...)
}

func (f *policyDataFuncs) CSVByRow(args ...string) ([]map[string]string, error) {
	defer perf.Track(nil, "exec.CSVByRow")()
	if err := guard.Function(context.Background(), "data.CSVByRow", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.CSVByRow(args...)
}

func (f *policyDataFuncs) CSVByColumn(args ...string) (map[string][]string, error) {
	defer perf.Track(nil, "exec.CSVByColumn")()
	if err := guard.Function(context.Background(), "data.CSVByColumn", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.CSVByColumn(args...)
}

func (f *policyDataFuncs) ToCSV(args ...any) (string, error) {
	defer perf.Track(nil, "exec.ToCSV")()
	if err := guard.Function(context.Background(), "data.ToCSV", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ToCSV(args...)
}

func (f *policyDataFuncs) ToJSON(in any) (string, error) {
	defer perf.Track(nil, "exec.ToJSON")()
	if err := guard.Function(context.Background(), "data.ToJSON", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ToJSON(in)
}

func (f *policyDataFuncs) ToJSONPretty(indent string, in any) (string, error) {
	defer perf.Track(nil, "exec.ToJSONPretty")()
	if err := guard.Function(context.Background(), "data.ToJSONPretty", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ToJSONPretty(indent, in)
}

func (f *policyDataFuncs) ToYAML(in any) (string, error) {
	defer perf.Track(nil, "exec.ToYAML")()
	if err := guard.Function(context.Background(), "data.ToYAML", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ToYAML(in)
}

func (f *policyDataFuncs) ToTOML(in any) (string, error) {
	defer perf.Track(nil, "exec.ToTOML")()
	if err := guard.Function(context.Background(), "data.ToTOML", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ToTOML(in)
}
