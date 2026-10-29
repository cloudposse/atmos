package component

import (
	"errors"
	"testing"

	"github.com/zclconf/go-cty/cty"
	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func TestValuesRoundTripThroughProtobuf(t *testing.T) {
	t.Parallel()
	large, err := cty.ParseNumberVal("9007199254740993123456789.123456789")
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]cty.Value{
		"exact number":      large,
		"bool":              cty.False,
		"null":              cty.NullVal(cty.String),
		"unknown":           cty.UnknownVal(cty.Number),
		"dynamic unknown":   cty.DynamicVal,
		"sensitive unknown": cty.UnknownVal(cty.String).Mark(Sensitive),
		"sensitive null":    cty.NullVal(cty.String).Mark(Sensitive),
		"empty list":        cty.ListValEmpty(cty.Number),
		"empty object":      cty.EmptyObjectVal,
		"set":               cty.SetVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}).Mark(Sensitive),
		"list":              cty.ListVal([]cty.Value{cty.StringVal("public"), cty.StringVal("private").Mark(Sensitive)}),
		"map":               cty.MapVal(map[string]cty.Value{"key": cty.StringVal("private").Mark(Sensitive)}),
		"tuple":             cty.TupleVal([]cty.Value{cty.True, large, cty.NullVal(cty.Number)}),
		"nested": cty.ObjectVal(map[string]cty.Value{
			"known":   large,
			"unknown": cty.UnknownVal(cty.List(cty.String)),
			"nested":  cty.ObjectVal(map[string]cty.Value{"token": cty.StringVal("secret").Mark(Sensitive)}),
		}),
	}
	for name, want := range values {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			encoded, err := Encode(want)
			if err != nil {
				t.Fatal(err)
			}
			wire, err := proto.Marshal(encoded)
			if err != nil {
				t.Fatal(err)
			}
			decoded := &pluginv1.ComponentValue{}
			if err := proto.Unmarshal(wire, decoded); err != nil {
				t.Fatal(err)
			}
			got, err := Decode(decoded)
			if err != nil {
				t.Fatal(err)
			}
			if !got.RawEquals(want) {
				t.Fatal("value, type, or sensitivity changed in transport")
			}
		})
	}
}

func TestEncodeRejectsUnsupportedValuesAndMarks(t *testing.T) {
	t.Parallel()
	for _, value := range []cty.Value{
		cty.NilVal,
		cty.StringVal("secret").Mark("unsupported"),
		cty.StringVal("secret").WithMarks(cty.NewValueMarks(Sensitive, "unsupported")),
	} {
		_, err := Encode(value)
		if !errors.Is(err, ErrInvalidValue) || err.Error() != ErrInvalidValue.Error() {
			t.Fatalf("expected a safe error: %v", err)
		}
	}
}

func TestDecodeRejectsMalformedPayloadsAndSensitivityPaths(t *testing.T) {
	t.Parallel()
	valid, err := Encode(cty.ObjectVal(map[string]cty.Value{"token": cty.StringVal("secret")}))
	if err != nil {
		t.Fatal(err)
	}
	index, err := Encode(cty.StringVal("token"))
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*pluginv1.ComponentValue){
		"invalid type":    func(v *pluginv1.ComponentValue) { v.TypeJson = []byte(`"secret"`) },
		"invalid payload": func(v *pluginv1.ComponentValue) { v.Msgpack = []byte{0xc1} },
		"empty payload":   func(v *pluginv1.ComponentValue) { v.Msgpack = nil },
		"nil path":        func(v *pluginv1.ComponentValue) { v.SensitivePaths = []*pluginv1.ComponentValuePath{nil} },
		"nil step": func(v *pluginv1.ComponentValue) {
			v.SensitivePaths = []*pluginv1.ComponentValuePath{{Steps: []*pluginv1.ComponentValueStep{nil}}}
		},
		"missing attribute": func(v *pluginv1.ComponentValue) {
			v.SensitivePaths = []*pluginv1.ComponentValuePath{{Steps: []*pluginv1.ComponentValueStep{{Selector: &pluginv1.ComponentValueStep_Attribute{Attribute: "absent"}}}}}
		},
		"index on object": func(v *pluginv1.ComponentValue) {
			v.SensitivePaths = []*pluginv1.ComponentValuePath{{Steps: []*pluginv1.ComponentValueStep{{Selector: &pluginv1.ComponentValueStep_Index{Index: index}}}}}
		},
		"nil index": func(v *pluginv1.ComponentValue) {
			v.SensitivePaths = []*pluginv1.ComponentValuePath{{Steps: []*pluginv1.ComponentValueStep{{Selector: &pluginv1.ComponentValueStep_Index{}}}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v := proto.Clone(valid).(*pluginv1.ComponentValue)
			change(v)
			_, err := Decode(v)
			if !errors.Is(err, ErrInvalidValue) || err.Error() != ErrInvalidValue.Error() {
				t.Fatalf("expected a safe error: %v", err)
			}
		})
	}
	if _, err := Decode(nil); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("nil payload accepted: %v", err)
	}
}

func TestDecodeRejectsInvalidIndexValues(t *testing.T) {
	t.Parallel()
	for _, value := range []cty.Value{cty.True, cty.NullVal(cty.String), cty.UnknownVal(cty.String), cty.StringVal("secret").Mark(Sensitive)} {
		encoded, err := Encode(value)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeIndex(encoded); !errors.Is(err, ErrInvalidValue) {
			t.Fatalf("invalid index accepted: %v", err)
		}
	}
}

func TestDecodeCannotDropSensitivityBelowAnUnknownObject(t *testing.T) {
	t.Parallel()
	encoded, err := Encode(cty.UnknownVal(cty.Object(map[string]cty.Type{"token": cty.String})))
	if err != nil {
		t.Fatal(err)
	}
	encoded.SensitivePaths = []*pluginv1.ComponentValuePath{{Steps: []*pluginv1.ComponentValueStep{{Selector: &pluginv1.ComponentValueStep_Attribute{Attribute: "token"}}}}}
	if _, err := Decode(encoded); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("unapplied sensitivity mark accepted: %v", err)
	}
}
