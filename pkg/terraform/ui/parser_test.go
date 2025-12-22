package ui

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_Next_VersionMessage(t *testing.T) {
	input := `{"@level":"info","@message":"Terraform 1.9.0","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:00.000000Z","type":"version","terraform":"1.9.0","ui":"1.2"}` + "\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err)
	require.NotNil(t, result.Message)

	msg, ok := result.Message.(*VersionMessage)
	require.True(t, ok, "expected VersionMessage")
	assert.Equal(t, "1.9.0", msg.Terraform)
	assert.Equal(t, "1.2", msg.UI)
}

func TestParser_Next_PlannedChange(t *testing.T) {
	input := `{"@level":"info","@message":"Plan: 1 to add, 0 to change, 0 to destroy","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:00.000000Z","type":"planned_change","change":{"resource":{"addr":"aws_instance.example","module":"","resource":"aws_instance.example","resource_type":"aws_instance","resource_name":"example","resource_key":null},"action":"create"}}` + "\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err)
	require.NotNil(t, result.Message)

	msg, ok := result.Message.(*PlannedChangeMessage)
	require.True(t, ok, "expected PlannedChangeMessage")
	assert.Equal(t, "aws_instance.example", msg.Change.Resource.Addr)
	assert.Equal(t, "create", msg.Change.Action)
	assert.Equal(t, "aws_instance", msg.Change.Resource.ResourceType)
	assert.Equal(t, "example", msg.Change.Resource.ResourceName)
}

func TestParser_Next_ApplyStart(t *testing.T) {
	input := `{"@level":"info","@message":"aws_instance.example: Creating...","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:00.000000Z","type":"apply_start","hook":{"resource":{"addr":"aws_instance.example","module":"","resource":"aws_instance.example","resource_type":"aws_instance","resource_name":"example","resource_key":null},"action":"create"}}` + "\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err)
	require.NotNil(t, result.Message)

	msg, ok := result.Message.(*ApplyStartMessage)
	require.True(t, ok, "expected ApplyStartMessage")
	assert.Equal(t, "aws_instance.example", msg.Hook.Resource.Addr)
	assert.Equal(t, "create", msg.Hook.Action)
}

func TestParser_Next_ApplyProgress(t *testing.T) {
	input := `{"@level":"info","@message":"aws_instance.example: Still creating... [10s elapsed]","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:10.000000Z","type":"apply_progress","hook":{"resource":{"addr":"aws_instance.example","module":"","resource":"aws_instance.example","resource_type":"aws_instance","resource_name":"example","resource_key":null},"action":"create","elapsed_secs":10}}` + "\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err)
	require.NotNil(t, result.Message)

	msg, ok := result.Message.(*ApplyProgressMessage)
	require.True(t, ok, "expected ApplyProgressMessage")
	assert.Equal(t, "aws_instance.example", msg.Hook.Resource.Addr)
	assert.Equal(t, 10, msg.Hook.ElapsedSecs)
}

func TestParser_Next_ApplyComplete(t *testing.T) {
	input := `{"@level":"info","@message":"aws_instance.example: Creation complete after 15s [id=i-1234567890abcdef0]","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:15.000000Z","type":"apply_complete","hook":{"resource":{"addr":"aws_instance.example","module":"","resource":"aws_instance.example","resource_type":"aws_instance","resource_name":"example","resource_key":null},"action":"create","id_key":"id","id_value":"i-1234567890abcdef0","elapsed_secs":15}}` + "\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err)
	require.NotNil(t, result.Message)

	msg, ok := result.Message.(*ApplyCompleteMessage)
	require.True(t, ok, "expected ApplyCompleteMessage")
	assert.Equal(t, "aws_instance.example", msg.Hook.Resource.Addr)
	assert.Equal(t, "id", msg.Hook.IDKey)
	assert.Equal(t, "i-1234567890abcdef0", msg.Hook.IDValue)
	assert.Equal(t, 15, msg.Hook.ElapsedSecs)
}

func TestParser_Next_ApplyErrored(t *testing.T) {
	input := `{"@level":"error","@message":"Error: creating EC2 Instance: operation error","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:00.000000Z","type":"apply_errored","hook":{"resource":{"addr":"aws_instance.example","module":"","resource":"aws_instance.example","resource_type":"aws_instance","resource_name":"example","resource_key":null},"action":"create","elapsed_secs":5}}` + "\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err)
	require.NotNil(t, result.Message)

	msg, ok := result.Message.(*ApplyErroredMessage)
	require.True(t, ok, "expected ApplyErroredMessage")
	assert.Equal(t, "aws_instance.example", msg.Hook.Resource.Addr)
	assert.Equal(t, 5, msg.Hook.ElapsedSecs)
	assert.Contains(t, msg.Message, "Error: creating EC2 Instance")
}

func TestParser_Next_RefreshStart(t *testing.T) {
	input := `{"@level":"info","@message":"aws_instance.example: Refreshing state...","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:00.000000Z","type":"refresh_start","hook":{"resource":{"addr":"aws_instance.example","module":"","resource":"aws_instance.example","resource_type":"aws_instance","resource_name":"example","resource_key":null},"id_key":"id","id_value":"i-1234567890abcdef0"}}` + "\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err)
	require.NotNil(t, result.Message)

	msg, ok := result.Message.(*RefreshStartMessage)
	require.True(t, ok, "expected RefreshStartMessage")
	assert.Equal(t, "aws_instance.example", msg.Hook.Resource.Addr)
	assert.Equal(t, "i-1234567890abcdef0", msg.Hook.IDValue)
}

func TestParser_Next_RefreshComplete(t *testing.T) {
	input := `{"@level":"info","@message":"aws_instance.example: Refresh complete","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:00.000000Z","type":"refresh_complete","hook":{"resource":{"addr":"aws_instance.example","module":"","resource":"aws_instance.example","resource_type":"aws_instance","resource_name":"example","resource_key":null},"id_key":"id","id_value":"i-1234567890abcdef0"}}` + "\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err)
	require.NotNil(t, result.Message)

	msg, ok := result.Message.(*RefreshCompleteMessage)
	require.True(t, ok, "expected RefreshCompleteMessage")
	assert.Equal(t, "aws_instance.example", msg.Hook.Resource.Addr)
}

func TestParser_Next_Diagnostic(t *testing.T) {
	input := `{"@level":"error","@message":"Error: creating EC2 Instance","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:00.000000Z","type":"diagnostic","diagnostic":{"severity":"error","summary":"creating EC2 Instance","detail":"operation error"}}` + "\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err)
	require.NotNil(t, result.Message)

	msg, ok := result.Message.(*DiagnosticMessage)
	require.True(t, ok, "expected DiagnosticMessage")
	assert.Equal(t, "error", msg.Diagnostic.Severity)
	assert.Equal(t, "creating EC2 Instance", msg.Diagnostic.Summary)
	assert.Equal(t, "operation error", msg.Diagnostic.Detail)
}

func TestParser_Next_ChangeSummary(t *testing.T) {
	input := `{"@level":"info","@message":"Plan: 2 to add, 1 to change, 0 to destroy.","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:00.000000Z","type":"change_summary","changes":{"add":2,"change":1,"remove":0,"operation":"plan"}}` + "\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err)
	require.NotNil(t, result.Message)

	msg, ok := result.Message.(*ChangeSummaryMessage)
	require.True(t, ok, "expected ChangeSummaryMessage")
	assert.Equal(t, 2, msg.Changes.Add)
	assert.Equal(t, 1, msg.Changes.Change)
	assert.Equal(t, 0, msg.Changes.Remove)
	assert.Equal(t, "plan", msg.Changes.Operation)
}

func TestParser_Next_EOF(t *testing.T) {
	parser := NewParser(strings.NewReader(""))

	result, err := parser.Next()
	assert.Equal(t, io.EOF, err)
	assert.Nil(t, result)
}

func TestParser_Next_InvalidJSON(t *testing.T) {
	input := "not valid json\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err) // Parser returns result with error, not error itself.
	require.NotNil(t, result)
	assert.NotNil(t, result.Err)
	assert.Contains(t, result.Err.Error(), "invalid JSON")
}

func TestParser_Next_UnknownType(t *testing.T) {
	input := `{"@level":"info","@message":"Unknown","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:00.000000Z","type":"unknown_type"}` + "\n"
	parser := NewParser(strings.NewReader(input))

	result, err := parser.Next()
	require.NoError(t, err)
	// Unknown types return a raw map.
	assert.NotNil(t, result.Message)
}

func TestParser_Next_MultipleMessages(t *testing.T) {
	input := `{"@level":"info","@message":"Terraform 1.9.0","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:00.000000Z","type":"version","terraform":"1.9.0","ui":"1.2"}
{"@level":"info","@message":"aws_instance.example: Creating...","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:01.000000Z","type":"apply_start","hook":{"resource":{"addr":"aws_instance.example","module":"","resource":"aws_instance.example","resource_type":"aws_instance","resource_name":"example","resource_key":null},"action":"create"}}
{"@level":"info","@message":"aws_instance.example: Creation complete","@module":"terraform.ui","@timestamp":"2024-01-01T00:00:02.000000Z","type":"apply_complete","hook":{"resource":{"addr":"aws_instance.example","module":"","resource":"aws_instance.example","resource_type":"aws_instance","resource_name":"example","resource_key":null},"action":"create","id_key":"id","id_value":"i-123","elapsed_secs":1}}
`
	parser := NewParser(strings.NewReader(input))

	// First message: version.
	result, err := parser.Next()
	require.NoError(t, err)
	_, ok := result.Message.(*VersionMessage)
	assert.True(t, ok)

	// Second message: apply_start.
	result, err = parser.Next()
	require.NoError(t, err)
	_, ok = result.Message.(*ApplyStartMessage)
	assert.True(t, ok)

	// Third message: apply_complete.
	result, err = parser.Next()
	require.NoError(t, err)
	_, ok = result.Message.(*ApplyCompleteMessage)
	assert.True(t, ok)

	// Fourth: EOF.
	_, err = parser.Next()
	assert.Equal(t, io.EOF, err)
}
