package policy

import (
	"encoding/json"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// normalizedCI preserves native datasets and adds only complete supported summaries.
// Null means unknown; an empty list means complete evidence containing no entries.
func normalizedCI(value any) any {
	ci, ok := value.(map[string]any)
	if !ok {
		return value
	}
	ci["labels"], ci["approvers"] = nil, nil
	if ci["provider"] != "github-actions" || ci["schema"] != "github-actions/v1" {
		return ci
	}
	ci["labels"] = contextLabels(ci["event"])
	if facts, ok := ci["facts"].(map[string]any); ok {
		ci["approvers"] = contextApprovers(facts["approvers"])
	}
	return ci
}

func contextLabels(value any) any {
	event, ok := availableContextDataset(value)
	if !ok {
		return nil
	}
	pr, ok := event["pull_request"].(map[string]any)
	if !ok {
		return nil
	}
	labels, ok := pr["labels"].([]any)
	if !ok {
		return nil
	}
	names := []string{}
	for _, value := range labels {
		label, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		name, ok := label["name"].(string)
		if !ok {
			return nil
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func contextApprovers(value any) any {
	dataset, ok := value.(map[string]any)
	if !ok || dataset["status"] != string(Available) || dataset["complete"] != true {
		return nil
	}
	names, ok := dataset["data"].([]any)
	if !ok {
		return nil
	}
	result := []string{}
	for _, name := range names {
		text, ok := name.(string)
		if !ok || text == "" {
			return nil
		}
		result = append(result, text)
	}
	sort.Strings(result)
	return result
}

func availableContextDataset(value any) (map[string]any, bool) {
	dataset, ok := value.(map[string]any)
	if !ok || dataset["status"] != string(Available) || dataset["complete"] != true {
		return nil, false
	}
	data, ok := dataset["data"].(map[string]any)
	return data, ok
}

// GitHubApprovers returns the latest decisive current-head approvals, excluding the author.
// The caller must verify freshness, pagination completeness and checkout binding.
func GitHubApprovers(head, author string, reviews []GitHubReview) ([]string, error) {
	defer perf.Track(nil, "policy.GitHubApprovers")()
	if head == "" || author == "" {
		return nil, errUtils.ErrPolicyInputUnavailable
	}
	latest, err := latestDecisiveGitHubReviews(reviews)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for name := range latest {
		if countGitHubApprovals([]string{name}, latest, head, author) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// ApproverDataset encodes verified reviewer names with their collection provenance.
func ApproverDataset(names []string, provenance Provenance) Dataset {
	defer perf.Track(nil, "policy.ApproverDataset")()

	data, _ := json.Marshal(names)
	return Dataset{Data: data, Status: Available, Complete: true, Provenance: provenance}
}
