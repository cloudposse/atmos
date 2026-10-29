package component

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"

	errUtils "github.com/cloudposse/atmos/errors"
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
	value := map[string]any{"large": json.Number("9007199254740993"), "nested": []any{nil, true, "literal"}}
	encoded, err := EncodeObject(value)
	require.NoError(t, err)
	decoded, err := DecodeObject(encoded)
	require.NoError(t, err)
	require.Equal(t, value, decoded)
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
	value := cty.ObjectVal(map[string]cty.Value{"items": cty.TupleVal([]cty.Value{cty.StringVal("masked-value"), cty.NumberIntVal(12345)})})
	encoded := objectWireValue(t, value)
	attr := &pluginv1.ComponentValueStep{Selector: &pluginv1.ComponentValueStep_Attribute{Attribute: "items"}}
	index := &pluginv1.ComponentValueStep{Selector: &pluginv1.ComponentValueStep_Index{Index: objectWireValue(t, cty.NumberIntVal(0))}}
	for _, path := range []*pluginv1.ComponentValuePath{{}, {Steps: []*pluginv1.ComponentValueStep{attr}}, {Steps: []*pluginv1.ComponentValueStep{attr, index}}} {
		encoded.SensitivePaths = []*pluginv1.ComponentValuePath{path}
		decoded, err := DecodeObject(encoded)
		require.NoError(t, err)
		require.Equal(t, []any{"masked-value", json.Number("12345")}, decoded["items"])
	}
	for _, path := range []*pluginv1.ComponentValuePath{nil, {Steps: []*pluginv1.ComponentValueStep{nil}}, {Steps: []*pluginv1.ComponentValueStep{index}}, {Steps: []*pluginv1.ComponentValueStep{{Selector: &pluginv1.ComponentValueStep_Index{Index: objectWireValue(t, cty.True)}}}}, {Steps: []*pluginv1.ComponentValueStep{{Selector: &pluginv1.ComponentValueStep_Index{}}}}} {
		encoded.SensitivePaths = []*pluginv1.ComponentValuePath{path}
		_, err := DecodeObject(encoded)
		require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	}
}
