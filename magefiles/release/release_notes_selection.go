package release

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
	"github.com/cloudposse/atmos/magefiles/internal/mageutil"
)

var errReleaseNotesConfig = errors.New("invalid release notes configuration")

// releaseNotesSelection retains the repository's release-drafter presentation
// configuration. Version resolution remains exclusively milestone policy.
type releaseNotesSelection struct {
	FilterByCommitish bool `yaml:"filter-by-commitish"`
	Categories        []struct {
		Title  string   `yaml:"title"`
		Labels []string `yaml:"labels"`
	} `yaml:"categories"`
	ChangeTemplate string `yaml:"change-template"`
	Template       string `yaml:"template"`
}

func loadReleaseNotesSelection() (*releaseNotesSelection, error) {
	root, err := mageutil.RepoRoot()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(filepath.Join(root, ".github", "release-notes.yml"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return parseReleaseNotesSelection(file)
}

func parseReleaseNotesSelection(reader io.Reader) (*releaseNotesSelection, error) {
	var config releaseNotesSelection
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("%w: %w", errReleaseNotesConfig, err)
	}
	if err := config.validateTemplates(); err != nil {
		return nil, err
	}
	if err := config.validateCategories(); err != nil {
		return nil, err
	}
	return &config, nil
}

var releaseNoteToken = regexp.MustCompile(`\$[A-Za-z_]+`)

func (config *releaseNotesSelection) validateTemplates() error {
	if !config.FilterByCommitish {
		return fmt.Errorf("%w: filter-by-commitish must remain enabled", errReleaseNotesConfig)
	}
	if strings.TrimSpace(config.Template) != "$CHANGES" {
		return fmt.Errorf("%w: template must contain only $CHANGES", errReleaseNotesConfig)
	}
	if !strings.HasPrefix(config.ChangeTemplate, "- ") || strings.ContainsAny(config.ChangeTemplate, "\r\n") {
		return fmt.Errorf("%w: change-template must be a single bullet", errReleaseNotesConfig)
	}
	for _, token := range releaseNoteToken.FindAllString(config.ChangeTemplate, -1) {
		switch token {
		case "$TITLE", "$AUTHOR", "$NUMBER":
		default:
			return fmt.Errorf("%w: unsupported change-template token %s", errReleaseNotesConfig, token)
		}
	}
	return nil
}

func (config *releaseNotesSelection) validateCategories() error {
	seen := map[string]bool{}
	for _, category := range config.Categories {
		if strings.TrimSpace(category.Title) == "" || strings.ContainsAny(category.Title, "\r\n") || seen[category.Title] {
			return fmt.Errorf("%w: category headings must be unique nonempty lines", errReleaseNotesConfig)
		}
		seen[category.Title] = true
		if len(category.Labels) == 0 {
			return fmt.Errorf("%w: category %s needs labels", errReleaseNotesConfig, category.Title)
		}
		for _, label := range category.Labels {
			if strings.TrimSpace(label) == "" {
				return fmt.Errorf("%w: category %s has an empty label", errReleaseNotesConfig, category.Title)
			}
		}
	}
	return nil
}

func (config *releaseNotesSelection) noteEntry(pr *releasePull) releasenotes.PREntry {
	// Replace in one pass: tokens in a PR's title remain literal data.
	title := strings.Join(strings.Fields(pr.Title), " ")
	replacer := strings.NewReplacer("$TITLE", title, "$AUTHOR", pr.User.Login, "$NUMBER", strconv.Itoa(pr.Number))
	return releasenotes.PREntry{
		Category: config.noteCategory(pr),
		Summary:  strings.TrimPrefix(replacer.Replace(config.ChangeTemplate), "- "),
		Number:   pr.Number,
		Body:     pr.Body,
	}
}

func (config *releaseNotesSelection) noteCategory(pr *releasePull) string {
	labels := make(map[string]bool, len(pr.Labels))
	for _, label := range pr.Labels {
		labels[label.Name] = true
	}
	for _, category := range config.Categories {
		for _, label := range category.Labels {
			if labels[label] {
				return "## " + category.Title
			}
		}
	}
	return ""
}

func (config *releaseNotesSelection) sortEntries(entries []releasenotes.PREntry) {
	ranks := make(map[string]int, len(config.Categories))
	for i, category := range config.Categories {
		ranks["## "+category.Title] = i + 1
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := ranks[entries[i].Category], ranks[entries[j].Category]
		if left != right {
			return left < right
		}
		return entries[i].Number < entries[j].Number
	})
}
