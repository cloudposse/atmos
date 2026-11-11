package policy

import (
	"context"
	"fmt"

	"github.com/hairyhenderson/gomplate/v5/conv"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/template/render"
)

// nativeDataFuncs describes the Gomplate namespace capabilities guarded below.
type nativeDataFuncs interface {
	JSONArray(in any) ([]any, error)
	YAML(in any) (map[string]any, error)
	YAMLArray(in any) ([]any, error)
	TOML(in any) (any, error)
	CSV(args ...string) ([][]string, error)
	CSVByRow(args ...string) ([]map[string]string, error)
	CSVByColumn(args ...string) (map[string][]string, error)
	ToCSV(args ...any) (string, error)
	ToJSON(in any) (string, error)
	ToJSONPretty(indent string, in any) (string, error)
	ToYAML(in any) (string, error)
	ToTOML(in any) (string, error)
}

type policyDataFuncs struct {
	native nativeDataFuncs
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
		return render.JSON(value)
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
	defer perf.Track(nil, "policy.JSON")()
	if err := guard.Function(context.Background(), "data.JSON", Engine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return guardedTemplateJSON(in)
}

func (f *policyDataFuncs) JSONArray(in any) ([]any, error) {
	defer perf.Track(nil, "policy.JSONArray")()
	if err := guard.Function(context.Background(), "data.JSONArray", Engine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.JSONArray(in)
}

func (f *policyDataFuncs) YAML(in any) (map[string]any, error) {
	defer perf.Track(nil, "policy.YAML")()
	if err := guard.Function(context.Background(), "data.YAML", Engine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.YAML(in)
}

func (f *policyDataFuncs) YAMLArray(in any) ([]any, error) {
	defer perf.Track(nil, "policy.YAMLArray")()
	if err := guard.Function(context.Background(), "data.YAMLArray", Engine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.YAMLArray(in)
}

func (f *policyDataFuncs) TOML(in any) (any, error) {
	defer perf.Track(nil, "policy.TOML")()
	if err := guard.Function(context.Background(), "data.TOML", Engine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.TOML(in)
}

func (f *policyDataFuncs) CSV(args ...string) ([][]string, error) {
	defer perf.Track(nil, "policy.CSV")()
	if err := guard.Function(context.Background(), "data.CSV", Engine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.CSV(args...)
}

func (f *policyDataFuncs) CSVByRow(args ...string) ([]map[string]string, error) {
	defer perf.Track(nil, "policy.CSVByRow")()
	if err := guard.Function(context.Background(), "data.CSVByRow", Engine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.CSVByRow(args...)
}

func (f *policyDataFuncs) CSVByColumn(args ...string) (map[string][]string, error) {
	defer perf.Track(nil, "policy.CSVByColumn")()
	if err := guard.Function(context.Background(), "data.CSVByColumn", Engine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.CSVByColumn(args...)
}

func (f *policyDataFuncs) ToCSV(args ...any) (string, error) {
	defer perf.Track(nil, "policy.ToCSV")()
	if err := guard.Function(context.Background(), "data.ToCSV", Engine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ToCSV(args...)
}

func (f *policyDataFuncs) ToJSON(in any) (string, error) {
	defer perf.Track(nil, "policy.ToJSON")()
	if err := guard.Function(context.Background(), "data.ToJSON", Engine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ToJSON(in)
}

func (f *policyDataFuncs) ToJSONPretty(indent string, in any) (string, error) {
	defer perf.Track(nil, "policy.ToJSONPretty")()
	if err := guard.Function(context.Background(), "data.ToJSONPretty", Engine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ToJSONPretty(indent, in)
}

func (f *policyDataFuncs) ToYAML(in any) (string, error) {
	defer perf.Track(nil, "policy.ToYAML")()
	if err := guard.Function(context.Background(), "data.ToYAML", Engine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ToYAML(in)
}

func (f *policyDataFuncs) ToTOML(in any) (string, error) {
	defer perf.Track(nil, "policy.ToTOML")()
	if err := guard.Function(context.Background(), "data.ToTOML", Engine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ToTOML(in)
}
