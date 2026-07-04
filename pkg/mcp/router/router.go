// Package router provides smart MCP server selection using a fast AI model
// to determine which servers are relevant to a user's question.
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// DefaultTimeout is the maximum time allowed for the routing call.
	DefaultTimeout = 10 * time.Second
	// Max tokens for routing responses (server name list only).
	maxRoutingTokens = 256
)

// DefaultMaxTokens returns the max tokens for routing responses.
func DefaultMaxTokens() int {
	return maxRoutingTokens
}

// ServerInfo holds the name and description of an MCP server for routing decisions.
type ServerInfo struct {
	Name        string
	Description string
}

// MessageSender is the minimal interface needed for routing — just SendMessage.
type MessageSender interface {
	SendMessage(ctx context.Context, message string) (string, error)
}

// Route uses a fast AI model to select which MCP servers are relevant to the question.
// Returns server names. On any error, falls back to returning all servers.
func Route(ctx context.Context, client MessageSender, question string, servers []ServerInfo) []string {
	defer perf.Track(nil, "mcp.router.Route")()

	// No routing needed for 0 or 1 servers.
	if len(servers) <= 1 {
		return allNames(servers)
	}

	prompt := buildPrompt(question, servers)

	routeCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	response, err := client.SendMessage(routeCtx, prompt)
	if err != nil {
		ui.Warning(fmt.Sprintf("MCP routing failed, starting all servers: %v", err))
		return allNames(servers)
	}

	selected := parseResponse(response, servers)
	if len(selected) == 0 {
		return allNames(servers)
	}

	return selected
}

// buildPrompt creates the routing prompt for the fast model.
func buildPrompt(question string, servers []ServerInfo) string {
	var sb strings.Builder
	sb.WriteString("You are a tool routing assistant. Given a user question and a list of available MCP servers, ")
	sb.WriteString("select ONLY the servers needed to answer the question.\n\n")
	sb.WriteString("Available servers:\n")
	for _, s := range servers {
		fmt.Fprintf(&sb, "- %s: %s\n", s.Name, s.Description)
	}
	sb.WriteString("\nUser question: ")
	sb.WriteString(question)
	sb.WriteString("\n\nReturn ONLY a JSON array of relevant server names. Example: [\"aws-iam\", \"aws-cloudtrail\"]\n")
	sb.WriteString("If unsure, include more servers rather than fewer. Return ONLY the JSON array, no other text.")
	return sb.String()
}

// parseResponse extracts server names from the AI response.
func parseResponse(response string, servers []ServerInfo) []string {
	response = strings.TrimSpace(response)

	// Strip markdown code fences if present.
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		var inner []string
		for _, line := range lines {
			if !strings.HasPrefix(strings.TrimSpace(line), "```") {
				inner = append(inner, line)
			}
		}
		response = strings.Join(inner, "\n")
		response = strings.TrimSpace(response)
	}

	var names []string
	if err := json.Unmarshal([]byte(response), &names); err != nil {
		return nil
	}
	return filterValid(names, servers)
}

// filterValid returns only names that exist in the server list.
func filterValid(names []string, servers []ServerInfo) []string {
	valid := make(map[string]bool, len(servers))
	for _, s := range servers {
		valid[s.Name] = true
	}
	var result []string
	for _, name := range names {
		if valid[name] {
			result = append(result, name)
		}
	}
	return result
}

// allNames returns all server names.
func allNames(servers []ServerInfo) []string {
	names := make([]string, len(servers))
	for i, s := range servers {
		names[i] = s.Name
	}
	return names
}
