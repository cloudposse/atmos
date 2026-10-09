package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func parse(t *testing.T, manifest string) map[string]any {
	t.Helper()
	result, err := u.UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{}, manifest, "stack.yaml")
	require.NoError(t, err)
	return result
}

func TestShortFormIntrinsicLongForm(t *testing.T) {
	for tag, want := range map[string]string{"!Ref": "Ref", "!Condition": "Condition", "!Sub": "Fn::Sub", "!GetAtt": "Fn::GetAtt"} {
		long, ok := ShortFormIntrinsicLongForm(tag)
		require.True(t, ok)
		assert.Equal(t, want, long)
	}
	_, ok := ShortFormIntrinsicLongForm("!ref")
	assert.False(t, ok, "case differs")
}

func TestIntrinsicRewriters_EveryShortFormBecomesLongForm(t *testing.T) {
	cases := []struct {
		tag  string
		yaml string
		want any
	}{
		{"!Ref", "v: !Ref Bucket", map[string]any{"Ref": "Bucket"}},
		{"!Condition", "v: !Condition IsProd", map[string]any{"Condition": "IsProd"}},
		{"!Sub scalar", `v: !Sub "${AppName}-${Stage}"`, map[string]any{"Fn::Sub": "${AppName}-${Stage}"}},
		{"!Sub list", "v: !Sub\n  - \"${a}\"\n  - a: !Ref X", map[string]any{"Fn::Sub": []any{"${a}", map[string]any{"a": map[string]any{"Ref": "X"}}}}},
		{"!GetAtt dotted", "v: !GetAtt Role.Arn", map[string]any{"Fn::GetAtt": []any{"Role", "Arn"}}},
		{"!GetAtt nested attr", "v: !GetAtt Table.StreamSpecification.StreamViewType", map[string]any{"Fn::GetAtt": []any{"Table", "StreamSpecification.StreamViewType"}}},
		{"!GetAtt list", "v: !GetAtt [Role, Arn]", map[string]any{"Fn::GetAtt": []any{"Role", "Arn"}}},
		{"!Join", "v: !Join [\"-\", [a, !Ref B]]", map[string]any{"Fn::Join": []any{"-", []any{"a", map[string]any{"Ref": "B"}}}}},
		{"!Select", "v: !Select [0, !GetAZs \"\"]", map[string]any{"Fn::Select": []any{0, map[string]any{"Fn::GetAZs": ""}}}},
		{"!Split", "v: !Split [\",\", \"a,b\"]", map[string]any{"Fn::Split": []any{",", "a,b"}}},
		{"!If", "v: !If [IsProd, a, b]", map[string]any{"Fn::If": []any{"IsProd", "a", "b"}}},
		{"!Equals", "v: !Equals [!Ref Env, prod]", map[string]any{"Fn::Equals": []any{map[string]any{"Ref": "Env"}, "prod"}}},
		{"!And", "v: !And [!Condition A, !Condition B]", map[string]any{"Fn::And": []any{map[string]any{"Condition": "A"}, map[string]any{"Condition": "B"}}}},
		{"!Or", "v: !Or [!Condition A, !Condition B]", map[string]any{"Fn::Or": []any{map[string]any{"Condition": "A"}, map[string]any{"Condition": "B"}}}},
		{"!Not", "v: !Not [!Condition A]", map[string]any{"Fn::Not": []any{map[string]any{"Condition": "A"}}}},
		{"!FindInMap", "v: !FindInMap [Map, !Ref R, Key]", map[string]any{"Fn::FindInMap": []any{"Map", map[string]any{"Ref": "R"}, "Key"}}},
		{"!Base64", "v: !Base64 hello", map[string]any{"Fn::Base64": "hello"}},
		{"!Cidr", "v: !Cidr [\"10.0.0.0/16\", 6, 8]", map[string]any{"Fn::Cidr": []any{"10.0.0.0/16", 6, 8}}},
		{"!GetAZs", "v: !GetAZs us-east-1", map[string]any{"Fn::GetAZs": "us-east-1"}},
		{"!ImportValue", "v: !ImportValue SharedVpc", map[string]any{"Fn::ImportValue": "SharedVpc"}},
		{"!Transform", "v: !Transform {Name: AWS::Include, Parameters: {Location: s3://b/k}}", map[string]any{"Fn::Transform": map[string]any{"Name": "AWS::Include", "Parameters": map[string]any{"Location": "s3://b/k"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.tag, func(t *testing.T) {
			assert.Equal(t, tc.want, parse(t, tc.yaml)["v"])
		})
	}
}

func TestIntrinsicRewriters_ScalarValuesStayStrings(t *testing.T) {
	// A logical ID that looks numeric must not be decoded as a number.
	assert.Equal(t, map[string]any{"Ref": "123"}, parse(t, "v: !Ref 123")["v"])
}

func TestIntrinsicRewriters_AtmosFunctionsAlongsideIntrinsics(t *testing.T) {
	manifest := `
template:
  Resources:
    Marker:
      Properties:
        Name: !Sub "/acme/${Stage}/owner"
        Value: !env DEPLOY_OWNER
`
	props := parse(t, manifest)["template"].(map[string]any)["Resources"].(map[string]any)["Marker"].(map[string]any)["Properties"].(map[string]any)
	assert.Equal(t, map[string]any{"Fn::Sub": "/acme/${Stage}/owner"}, props["Name"])
	assert.Equal(t, "!env DEPLOY_OWNER", props["Value"], "the Atmos function is deferred for the evaluation phase")
}

func TestIntrinsicRewriters_IncludedTemplateFileKeepsShortForms(t *testing.T) {
	dir := t.TempDir()
	template := `AWSTemplateFormatVersion: '2010-09-09'
Resources:
  Function:
    Type: AWS::Lambda::Function
    Properties:
      FunctionName: !Sub "${AppName}-${Stage}"
      Role: !GetAtt Role.Arn
      Code:
        ZipFile: !include ./handler.py
Outputs:
  Name:
    Value: !Ref Function
`
	sub := filepath.Join(dir, "components", "cloudformation", "app")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "template.yaml"), []byte(template), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "handler.py"), []byte("print(1)\n"), 0o644))
	manifestPath := filepath.Join(dir, "stack.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte("x: 1\n"), 0o644))

	result, err := u.UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir}, "template: !include ./components/cloudformation/app/template.yaml | eval\n", manifestPath)
	require.NoError(t, err)

	tpl := result["template"].(map[string]any)
	props := tpl["Resources"].(map[string]any)["Function"].(map[string]any)["Properties"].(map[string]any)
	assert.Equal(t, map[string]any{"Fn::Sub": "${AppName}-${Stage}"}, props["FunctionName"])
	assert.Equal(t, map[string]any{"Fn::GetAtt": []any{"Role", "Arn"}}, props["Role"])
	assert.Equal(t, "print(1)\n", props["Code"].(map[string]any)["ZipFile"], "Rain's !Rain::Embed parity: a nested ./ include resolves relative to the template file")
	assert.Equal(t, map[string]any{"Ref": "Function"}, tpl["Outputs"].(map[string]any)["Name"].(map[string]any)["Value"])
}
