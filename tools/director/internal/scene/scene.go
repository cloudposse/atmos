package scene

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ScenesList represents the scenes.yaml file.
type ScenesList struct {
	Version string   `yaml:"version"`
	Scenes  []*Scene `yaml:"scenes"`
}

// Scene represents a single demo scene.
type Scene struct {
	Name        string            `yaml:"name"`
	Enabled     bool              `yaml:"enabled"`
	Description string            `yaml:"description"`
	Tape        string            `yaml:"tape"`
	Workdir     string            `yaml:"workdir,omitempty"` // Working directory for VHS (relative to repo root)
	Requires    []string          `yaml:"requires"`
	Outputs     []string          `yaml:"outputs"`
	Audio       *AudioConfig      `yaml:"audio,omitempty"`
	Tags        []string          `yaml:"tags,omitempty"`     // Tags for filtering (e.g., "featured")
	Gallery     *GalleryConfig    `yaml:"gallery,omitempty"`  // Gallery display configuration
	Prep        []string          `yaml:"prep,omitempty"`     // Shell commands to run before VHS (in workdir)
	Validate    *ValidationConfig `yaml:"validate,omitempty"` // Post-render validation rules
}

// ValidationConfig contains patterns for validating rendered SVG output.
type ValidationConfig struct {
	MustNotMatch []string `yaml:"must_not_match,omitempty"` // Patterns that must NOT appear (errors)
	MustMatch    []string `yaml:"must_match,omitempty"`     // Patterns that MUST appear (expected output)
}

// GalleryConfig contains settings for how the scene appears in the website gallery.
type GalleryConfig struct {
	Category string `yaml:"category"`        // Category ID (e.g., "terraform", "list", "dx")
	Title    string `yaml:"title,omitempty"` // Display title (defaults to scene name)
	Order    int    `yaml:"order,omitempty"` // Sort order within category (lower = first)
}

// AudioConfig contains background audio configuration for MP4 outputs.
type AudioConfig struct {
	Source  string  `yaml:"source"`             // Path to MP3 file relative to demos/
	Volume  float64 `yaml:"volume,omitempty"`   // Volume level 0.0-1.0, default 0.3
	FadeOut float64 `yaml:"fade_out,omitempty"` // Fade-out duration in seconds, default 2.0
	Loop    bool    `yaml:"loop,omitempty"`     // Loop audio if shorter than video, default true
}

// GetCategory returns the gallery category for the scene.
// Returns empty string if no gallery config is set.
func (s *Scene) GetCategory() string {
	if s.Gallery != nil {
		return s.Gallery.Category
	}
	return ""
}

// HasTag returns true if the scene has the specified tag.
func (s *Scene) HasTag(tag string) bool {
	for _, t := range s.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// LoadScenes loads scenes from scenes.yaml file.
func LoadScenes(path string) (*ScenesList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var scenes ScenesList
	if err := yaml.Unmarshal(data, &scenes); err != nil {
		return nil, err
	}

	return &scenes, nil
}

// CalculateTapeDuration parses a VHS tape file and calculates the expected duration
// by summing all Sleep commands. This is useful for estimating SVG animation length
// when no MP4 duration is available.
// Returns duration in seconds.
func CalculateTapeDuration(tapePath string) (float64, error) {
	file, err := os.Open(tapePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	// Regex to match Sleep commands: "Sleep 2s", "Sleep 500ms", "Sleep 1.5s"
	sleepRegex := regexp.MustCompile(`^\s*Sleep\s+([0-9.]+)(ms|s)\s*$`)

	var totalSeconds float64
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines.
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		matches := sleepRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			value, err := strconv.ParseFloat(matches[1], 64)
			if err != nil {
				continue
			}

			unit := matches[2]
			if unit == "ms" {
				totalSeconds += value / 1000.0
			} else {
				totalSeconds += value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return totalSeconds, nil
}
