package component

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func objectWireValue(t *testing.T, value cty.Value) *pluginv1.ComponentValue {
	t.Helper()
	typ, err := ctyjson.MarshalType(value.Type())
	require.NoError(t, err)
	payload, err := msgpack.Marshal(value, value.Type())
	require.NoError(t, err)
	return &pluginv1.ComponentValue{TypeJson: typ, Msgpack: payload}
}

func TestObjectRoundTrip(t *testing.T) {
	value := map[string]any{"large": json.Number("9007199254740993"), "ratio": json.Number("0.5"), "precise": json.Number("0.1000000000000000000001"), "nested": []any{nil, true, "literal", json.Number("3")}}
	encoded, err := EncodeObject(value)
	require.NoError(t, err)
	decoded, err := DecodeObject(encoded)
	require.NoError(t, err)
	// Exact numbers become native so YAML renders them unquoted; others keep full precision.
	want := map[string]any{"large": int64(9007199254740993), "ratio": 0.5, "precise": json.Number("0.1000000000000000000001"), "nested": []any{nil, true, "literal", int64(3)}}
	require.Equal(t, want, decoded)
	decoded, err = DecodeObject(nil)
	require.NoError(t, err)
	require.Empty(t, decoded)
	_, err = EncodeObject(make(chan int))
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestObjectRejectsMalformedAndUnknown(t *testing.T) {
	for _, encoded := range []*pluginv1.ComponentValue{{}, {TypeJson: []byte(`"string"`), Msgpack: []byte{0xc1}}, objectWireValue(t, cty.NullVal(cty.EmptyObject)), objectWireValue(t, cty.StringVal("scalar"))} {
		_, err := DecodeObject(encoded)
		require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	}
	_, err := DecodeObject(objectWireValue(t, cty.ObjectVal(map[string]cty.Value{"pending": cty.UnknownVal(cty.String)})))
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
}

func TestObjectSensitivePaths(t *testing.T) {
	atmosio.Reset()
	t.Cleanup(atmosio.Reset)
	require.NoError(t, atmosio.Initialize())
	attr := &pluginv1.ComponentValueStep{Selector: &pluginv1.ComponentValueStep_Attribute{Attribute: "items"}}
	index := &pluginv1.ComponentValueStep{Selector: &pluginv1.ComponentValueStep_Index{Index: objectWireValue(t, cty.NumberIntVal(0))}}
	paths := []*pluginv1.ComponentValuePath{{}, {Steps: []*pluginv1.ComponentValueStep{attr}}, {Steps: []*pluginv1.ComponentValueStep{attr, index}}}
	for i, path := range paths {
		secret := fmt.Sprintf("unique-masked-value-%d", i)
		value := cty.ObjectVal(map[string]cty.Value{"items": cty.TupleVal([]cty.Value{cty.StringVal(secret), cty.NumberIntVal(int64(12345 + i))})})
		encoded := objectWireValue(t, value)
		encoded.SensitivePaths = []*pluginv1.ComponentValuePath{path}
		require.Equal(t, secret, atmosio.MaskString(secret))
		decoded, err := DecodeObject(encoded)
		require.NoError(t, err)
		require.Equal(t, []any{secret, int64(12345 + i)}, decoded["items"])
		require.Equal(t, "<MASKED>", atmosio.MaskString(secret))
	}
	numeric := "987654321"
	require.Equal(t, numeric, atmosio.MaskString(numeric))
	numericValue := objectWireValue(t, cty.ObjectVal(map[string]cty.Value{"items": cty.TupleVal([]cty.Value{cty.StringVal("other"), cty.NumberIntVal(987654321)})}))
	numericIndex := &pluginv1.ComponentValueStep{Selector: &pluginv1.ComponentValueStep_Index{Index: objectWireValue(t, cty.NumberIntVal(1))}}
	numericValue.SensitivePaths = []*pluginv1.ComponentValuePath{{Steps: []*pluginv1.ComponentValueStep{attr, numericIndex}}}
	_, err := DecodeObject(numericValue)
	require.NoError(t, err)
	require.Equal(t, "<MASKED>", atmosio.MaskString(numeric))
	encoded := objectWireValue(t, cty.ObjectVal(map[string]cty.Value{"items": cty.TupleVal([]cty.Value{cty.StringVal("x"), cty.NumberIntVal(1)})}))

	for _, path := range []*pluginv1.ComponentValuePath{nil, {Steps: []*pluginv1.ComponentValueStep{nil}}, {Steps: []*pluginv1.ComponentValueStep{index}}, {Steps: []*pluginv1.ComponentValueStep{{Selector: &pluginv1.ComponentValueStep_Index{Index: objectWireValue(t, cty.True)}}}}, {Steps: []*pluginv1.ComponentValueStep{{Selector: &pluginv1.ComponentValueStep_Index{}}}}} {
		encoded.SensitivePaths = []*pluginv1.ComponentValuePath{path}
		_, err := DecodeObject(encoded)
		require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	}
}
