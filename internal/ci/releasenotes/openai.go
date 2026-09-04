package releasenotes

//go:generate go run go.uber.org/mock/mockgen -package releasenotes -destination mock_http.go github.com/cloudposse/atmos/internal/ci/releasenotes HTTPClient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	openAIChatCompletionsURL = "https://api.openai.com/v1/chat/completions"

	// Bounds how much of one PR's body is sent to the model: a single very
	// long description should not blow up the prompt (or the bill) for the
	// whole batch. Sized to fit a full "what / why / references" description
	// (typically 2-4k) so the model condenses the real content, not a cut.
	maxEntryBodyChars = 6000

	summarizerSystemPrompt = `You condense GitHub pull request descriptions into release notes ` +
		`for end users of the Atmos CLI. For each pull request write a short Markdown summary: ` +
		`3 to 6 bullet points, about 60 to 150 words in total, that keep what changed and why ` +
		`it matters, using the concrete names of commands, flags, configuration keys, and ` +
		`behaviors from the description. Do not repeat the pull request title, do not mention ` +
		`tests, CI, coverage, or internal refactoring unless that is the whole change, and do ` +
		`not invent details that are not in the description. Return ONLY a JSON array of ` +
		`objects: [{"number": <PR number>, "summary": "<markdown>"}, ...], one per pull ` +
		`request, in the same order as given, with no other text before or after the array.`
)

// errSummaryMismatch means the model's response didn't cover every entry
// sent to it, in the same order - either the count is off or a PR number at
// some position doesn't match the entry sent at that position. Callers
// should not render a partial or misattributed summary, since a silently
// dropped or swapped entry is worse than no summarization at all.
var errSummaryMismatch = errors.New("releasenotes: summary does not match entry")

// errOpenAIRequestFailed wraps a non-200 response from the Chat Completions
// API, carrying the status and body as %w-wrapped context rather than a
// dynamic error string.
var errOpenAIRequestFailed = errors.New("releasenotes: OpenAI API request failed")

// HTTPClient is the one method this package needs from *http.Client, so
// tests can inject a fake instead of making live requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// chatCompletionRequest is the subset of OpenAI's Chat Completions request
// body this package uses. Deliberately no `temperature`: the gpt-5 family
// rejects any value but the default with a 400 (verified live), and the
// "faithful condensation, not a rewrite" steer lives in the system prompt
// instead, which every model honors.
type chatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []chatMsg `json:"messages"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMsg `json:"message"`
	} `json:"choices"`
}

type entrySummary struct {
	Number  int    `json:"number"`
	Summary string `json:"summary"`
}

// Summarize sends every entry's title and body to the given OpenAI chat
// model in a single request and returns one condensed summary per entry, in
// the same order as entries. It returns an error - never a partial result -
// when the request fails, the response isn't valid JSON, or the model
// returned a different number of summaries than entries given.
func Summarize(ctx context.Context, client HTTPClient, apiKey, model string, entries []PREntry) ([]string, error) {
	defer perf.Track(nil, "releasenotes.Summarize")()

	reqBody := chatCompletionRequest{
		Model: model,
		Messages: []chatMsg{
			{Role: "system", Content: summarizerSystemPrompt},
			{Role: "user", Content: buildPrompt(entries)},
		},
	}
	content, err := doChatCompletion(ctx, client, apiKey, reqBody)
	if err != nil {
		return nil, err
	}

	var summaries []entrySummary
	if err := json.Unmarshal([]byte(extractJSONArray(content)), &summaries); err != nil {
		return nil, fmt.Errorf("releasenotes: decode model response: %w", err)
	}
	if len(summaries) != len(entries) {
		return nil, fmt.Errorf("%w: got %d, want %d", errSummaryMismatch, len(summaries), len(entries))
	}

	out := make([]string, len(summaries))
	for i, s := range summaries {
		if s.Number != entries[i].Number {
			return nil, fmt.Errorf(
				"%w: got PR #%d at position %d, want PR #%d",
				errSummaryMismatch, s.Number, i, entries[i].Number,
			)
		}
		out[i] = strings.TrimSpace(s.Summary)
	}
	return out, nil
}

func doChatCompletion(ctx context.Context, client HTTPClient, apiKey string, reqBody chatCompletionRequest) (string, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("releasenotes: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIChatCompletionsURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("releasenotes: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("releasenotes: call OpenAI: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("releasenotes: read OpenAI response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: OpenAI returned %s: %s", errOpenAIRequestFailed, resp.Status, string(body))
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("releasenotes: decode OpenAI response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("releasenotes: %w", errNoEntries)
	}
	return parsed.Choices[0].Message.Content, nil
}

func buildPrompt(entries []PREntry) string {
	var b strings.Builder
	for _, e := range entries {
		body := e.Body
		if len(body) > maxEntryBodyChars {
			body = body[:maxEntryBodyChars] + "…"
		}
		fmt.Fprintf(&b, "PR #%d: %s\n%s\n\n", e.Number, e.Summary, body)
	}
	return b.String()
}

// extractJSONArray trims any leading/trailing prose a model added despite
// being told not to, keeping only the outermost `[...]` array so json.Unmarshal
// doesn't choke on it.
func extractJSONArray(s string) string {
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start == -1 || end == -1 || end < start {
		return s
	}
	return s[start : end+1]
}
