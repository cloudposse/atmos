package release

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
	"github.com/cloudposse/atmos/magefiles/release/milestone"
)

// Previews reconciles informational issues for stable release milestones.
// These issues never authorize a release, select its source, or create a tag.
func (Release) Previews(repo string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c, err := newMilestoneClient(ctx, repo)
	if err != nil {
		return err
	}
	return c.previews()
}

const (
	githubActionsBotID    = 41898282
	previewBodyLimit      = 60000
	previewOpen           = "open"
	previewClosed         = "closed"
	milestoneClosedMarker = "<!-- atmos-release-preview:milestone-closed -->"
)

type previewAuthor struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type previewIssue struct {
	Number      int             `json:"number"`
	NodeID      string          `json:"node_id"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	State       string          `json:"state"`
	PullRequest json.RawMessage `json:"pull_request"`
	User        previewAuthor   `json:"user"`
}

type previewUpdate struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Milestone int    `json:"milestone"`
	State     string `json:"state"`
}

func previewMarker(number int) string {
	return fmt.Sprintf("<!-- atmos-release-preview:v1 milestone=%d -->", number)
}

func (issue *previewIssue) isPullRequest() bool {
	return len(issue.PullRequest) != 0 && string(issue.PullRequest) != "null"
}

func (c *milestoneClient) previews() error {
	milestones, err := c.ListMilestones()
	if err != nil {
		return err
	}
	for _, item := range milestones {
		version, err := milestone.MilestoneVersion(item.Title)
		if err != nil {
			continue
		}
		if err = c.reconcilePreview(item, version.String()); err != nil {
			return err
		}
	}
	return nil
}

// Reconciliation keeps ownership, rendering, persistence and pinning separate.
// Neither the issue body nor its open/closed state authorizes publication.
func (c *milestoneClient) reconcilePreview(m Milestone, version string) error {
	items, err := milestonePages[previewIssue](c, fmt.Sprintf("issues?state=all&milestone=%d", m.Number))
	if err != nil {
		return err
	}
	existing, err := findOwnedPreview(items, m.Number)
	if err != nil {
		return err
	}
	if existing == nil && m.State != previewOpen {
		return nil
	}
	if m.State != previewOpen && existing.State == previewClosed && strings.Contains(existing.Body, milestoneClosedMarker) {
		// Finalized content needs no old PR lookups, but a failed unpin must
		// remain repairable on the next reconciliation.
		return c.pinPreview(existing.NodeID, false)
	}
	body, err := c.renderPreview(m, version, items)
	if err != nil {
		return err
	}
	update := previewUpdate{
		Title:     "Release v" + version,
		Body:      body,
		Milestone: m.Number,
		State:     previewState(m.State, existing),
	}
	issue, err := c.savePreview(existing, update)
	if err != nil {
		return err
	}
	return c.pinPreview(issue.NodeID, update.State == previewOpen)
}

func findOwnedPreview(items []previewIssue, milestoneNumber int) (*previewIssue, error) {
	var existing *previewIssue
	for i := range items {
		item := &items[i]
		// A copied marker is not ownership evidence. Never overwrite a user issue
		// or a bot-authored PR simply because its body contains our marker.
		if item.isPullRequest() || item.User.ID != githubActionsBotID || item.User.Login != "github-actions[bot]" {
			continue
		}
		if !strings.HasPrefix(item.Body, previewMarker(milestoneNumber)+"\n") {
			continue
		}
		if existing != nil {
			return nil, fmt.Errorf("%w: duplicate release preview for milestone %d", ErrMilestonePolicy, milestoneNumber)
		}
		existing = item
	}
	return existing, nil
}

func (c *milestoneClient) renderPreview(m Milestone, version string, items []previewIssue) (string, error) {
	var body strings.Builder
	fmt.Fprintf(&body, "%s\n\n## Upcoming release v%s\n\n[Milestone](https://github.com/%s/milestone/%d)\n\nThis issue is a preview, not a release approval. Use `atmos release cut` to publish after all milestone PRs merge.\n\n## Included pull requests\n\n", previewMarker(m.Number), version, c.Repo, m.Number)
	notes, err := loadReleaseNotesSelection()
	if err != nil {
		return "", err
	}
	// Group and order PRs exactly as the published release notes will.
	var entries []releasenotes.PREntry
	pulls := map[int]*releasePull{}
	for _, item := range items {
		if !item.isPullRequest() {
			continue
		}
		var pr releasePull
		if err := c.Get(fmt.Sprintf("pulls/%d", item.Number), &pr); err != nil {
			return "", err
		}
		pr.Number = item.Number
		pulls[item.Number] = &pr
		entries = append(entries, notes.noteEntry(&pr))
	}
	notes.sortEntries(entries)
	writePreviewEntries(&body, entries, pulls)
	if len(entries) == 0 {
		body.WriteString("No pull requests assigned yet.\n")
	}
	if m.State != previewOpen {
		body.WriteString("\n" + milestoneClosedMarker + "\nMilestone closed. See the milestone and published releases for its final status.\n")
	}
	// Keep a byte limit and reject before writing; never partially overwrite
	// an existing preview when its rendered contents exceed the API limit.
	if body.Len() > previewBodyLimit {
		return "", fmt.Errorf("%w: preview for milestone %d exceeds issue body limit", ErrMilestonePolicy, m.Number)
	}
	return body.String(), nil
}

// writePreviewEntries writes sorted entries under category headings demoted one
// level below "Included pull requests". As in the release body, uncategorized
// PRs get a heading only when other PRs are categorized.
func writePreviewEntries(body *strings.Builder, entries []releasenotes.PREntry, pulls map[int]*releasePull) {
	categorized := false
	for _, entry := range entries {
		categorized = categorized || entry.Category != ""
	}
	for i, entry := range entries {
		if i == 0 || entry.Category != entries[i-1].Category {
			writePreviewHeading(body, previewHeading(entry.Category, categorized), i > 0)
		}
		writePreviewPR(body, entry.Number, pulls[entry.Number])
	}
}

func previewHeading(category string, categorized bool) string {
	if category == "" && categorized {
		return releasenotes.UncategorizedHeading
	}
	return category
}

func writePreviewHeading(body *strings.Builder, heading string, separate bool) {
	if heading == "" {
		return
	}
	if separate {
		body.WriteString("\n")
	}
	fmt.Fprintf(body, "#%s\n\n", heading)
}

func writePreviewPR(body *strings.Builder, number int, pr *releasePull) {
	state := previewOpen
	if pr.Merged {
		state = "merged"
	} else if pr.State == previewClosed {
		state = "closed, unmerged — blocks release"
	}
	fmt.Fprintf(body, "- #%d — %s (%s)\n", number, strings.Join(strings.Fields(pr.Title), " "), state)
	if summary := releasenotes.FallbackSummary(pr.Body); summary != "" {
		fmt.Fprintf(body, "\n  %s\n\n", strings.ReplaceAll(summary, "\n", "\n  "))
	}
}

func previewState(milestoneState string, existing *previewIssue) string {
	if milestoneState != previewOpen {
		return previewClosed
	}
	// Reopening a milestone reverses an automated closure, while a person's
	// explicit closure remains respected until they reopen the preview.
	if existing != nil && existing.State == previewClosed && !strings.Contains(existing.Body, milestoneClosedMarker) {
		return previewClosed
	}
	return previewOpen
}

func (c *milestoneClient) savePreview(existing *previewIssue, update previewUpdate) (*previewIssue, error) {
	if existing == nil {
		return c.createPreview(update)
	}
	if existing.Title == update.Title && existing.Body == update.Body && existing.State == update.State {
		return existing, nil
	}
	if err := c.Write(http.MethodPatch, fmt.Sprintf("issues/%d", existing.Number), update); err != nil {
		return nil, err
	}
	return existing, nil
}

func (c *milestoneClient) createPreview(update previewUpdate) (*previewIssue, error) {
	data, _ := json.Marshal(update)
	var created previewIssue
	if err := c.API.DoWithContext(c.Context, http.MethodPost, "repos/"+c.Repo+"/issues", bytes.NewReader(data), &created); err != nil {
		return nil, err
	}
	if created.Number <= 0 || created.NodeID == "" {
		return nil, fmt.Errorf("%w: missing created preview identity", ErrMilestonePolicy)
	}
	return &created, nil
}

func (c *milestoneClient) previewGraphQL(query string, variables map[string]any, out any) error {
	data, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	var result struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err = c.API.DoWithContext(c.Context, http.MethodPost, "graphql", bytes.NewReader(data), &result); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("%w: preview GraphQL: %s", ErrMilestonePolicy, result.Errors[0].Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(result.Data, out)
}

func (c *milestoneClient) pinPreview(nodeID string, pin bool) error {
	if nodeID == "" {
		return fmt.Errorf("%w: preview node ID missing", ErrMilestonePolicy)
	}
	parts := strings.Split(c.Repo, "/")
	var data struct {
		Repository struct {
			PinnedIssues struct {
				Nodes []struct {
					Issue struct {
						ID string `json:"id"`
					} `json:"issue"`
				} `json:"nodes"`
			} `json:"pinnedIssues"`
		} `json:"repository"`
	}
	if err := c.previewGraphQL(`query($owner:String!,$name:String!){repository(owner:$owner,name:$name){pinnedIssues(first:3){nodes{issue{id}}}}}`, map[string]any{"owner": parts[0], "name": parts[1]}, &data); err != nil {
		return err
	}
	pinned := false
	for _, item := range data.Repository.PinnedIssues.Nodes {
		if item.Issue.ID == nodeID {
			pinned = true
		}
	}
	if pin == pinned {
		return nil
	}
	if pin && len(data.Repository.PinnedIssues.Nodes) >= 3 {
		return nil
	} // Never evict another pinned issue.
	operation := "unpinIssue"
	if pin {
		operation = "pinIssue"
	}
	return c.previewGraphQL("mutation($id:ID!){"+operation+"(input:{issueId:$id}){issue{id}}}", map[string]any{"id": nodeID}, nil)
}
