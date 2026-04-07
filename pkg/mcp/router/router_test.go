package router

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSender implements MessageSender for testing.
type mockSender struct {
	response string
	err      error
}

func (m *mockSender) SendMessage(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

var testServers = []ServerInfo{
	{Name: "aws-iam", Description: "IAM role and policy analysis"},
	{Name: "aws-billing", Description: "Billing summaries and cost analysis"},
	{Name: "aws-security", Description: "Security posture assessment"},
	{Name: "aws-docs", Description: "AWS documentation search"},
}

func TestRoute_SingleServer(t *testing.T) {
	servers := []ServerInfo{{Name: "aws-iam", Description: "IAM"}}
	result := Route(context.Background(), nil, "anything", servers)
	assert.Equal(t, []string{"aws-iam"}, result)
}

func TestRoute_EmptyServers(t *testing.T) {
	result := Route(context.Background(), nil, "anything", nil)
	assert.Empty(t, result)
}

func TestRoute_ValidJSONResponse(t *testing.T) {
	sender := &mockSender{response: `["aws-iam", "aws-security"]`}
	result := Route(context.Background(), sender, "List IAM roles", testServers)
	assert.Equal(t, []string{"aws-iam", "aws-security"}, result)
}

func TestRoute_FallbackOnError(t *testing.T) {
	sender := &mockSender{err: errors.New("API error")}
	result := Route(context.Background(), sender, "anything", testServers)
	assert.ElementsMatch(t, allNames(testServers), result, "should return all servers on error")
}

func TestRoute_FallbackOnEmptyResponse(t *testing.T) {
	sender := &mockSender{response: ""}
	result := Route(context.Background(), sender, "anything", testServers)
	assert.ElementsMatch(t, allNames(testServers), result, "should return all servers on empty response")
}

func TestRoute_FallbackOnInvalidJSON(t *testing.T) {
	sender := &mockSender{response: "not json"}
	result := Route(context.Background(), sender, "anything", testServers)
	assert.ElementsMatch(t, allNames(testServers), result, "should return all servers on invalid JSON")
}

func TestParseResponse_ValidJSON(t *testing.T) {
	result := parseResponse(`["aws-iam", "aws-billing"]`, testServers)
	assert.Equal(t, []string{"aws-iam", "aws-billing"}, result)
}

func TestParseResponse_WithCodeFence(t *testing.T) {
	response := "```json\n[\"aws-iam\"]\n```"
	result := parseResponse(response, testServers)
	assert.Equal(t, []string{"aws-iam"}, result)
}

func TestParseResponse_FiltersInvalidNames(t *testing.T) {
	result := parseResponse(`["aws-iam", "nonexistent"]`, testServers)
	assert.Equal(t, []string{"aws-iam"}, result)
}

func TestParseResponse_EmptyArray(t *testing.T) {
	result := parseResponse(`[]`, testServers)
	assert.Empty(t, result)
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	result := parseResponse(`not json`, testServers)
	assert.Nil(t, result)
}

func TestBuildPrompt(t *testing.T) {
	prompt := buildPrompt("List IAM roles", testServers)
	assert.Contains(t, prompt, "aws-iam")
	assert.Contains(t, prompt, "aws-billing")
	assert.Contains(t, prompt, "List IAM roles")
	assert.Contains(t, prompt, "JSON array")
}

func TestFilterValid(t *testing.T) {
	tests := []struct {
		name     string
		names    []string
		expected []string
	}{
		{"all valid", []string{"aws-iam", "aws-billing"}, []string{"aws-iam", "aws-billing"}},
		{"some invalid", []string{"aws-iam", "fake"}, []string{"aws-iam"}},
		{"all invalid", []string{"fake1", "fake2"}, nil},
		{"empty input", []string{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterValid(tt.names, testServers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAllNames(t *testing.T) {
	result := allNames(testServers)
	require.Len(t, result, 4)
	assert.Contains(t, result, "aws-iam")
	assert.Contains(t, result, "aws-billing")
	assert.Contains(t, result, "aws-security")
	assert.Contains(t, result, "aws-docs")
}

func TestDefaultMaxTokens(t *testing.T) {
	assert.Equal(t, maxRoutingTokens, DefaultMaxTokens())
}
