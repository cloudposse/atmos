package scene

import (
	"os"

	"gopkg.in/yaml.v3"
)

// ScenesList represents the scenes.yaml file.
type ScenesList struct {
	Version string   `yaml:"version"`
	Scenes  []*Scene `yaml:"scenes"`
}

// Scene represents a single demo scene.
type Scene struct {
	Name        string         `yaml:"name"`
	Enabled     bool           `yaml:"enabled"`
	Description string         `yaml:"description"`
	Tape        string         `yaml:"tape"`
	Workdir     string         `yaml:"workdir,omitempty"` // Working directory for VHS (relative to repo root)
	Requires    []string       `yaml:"requires"`
	Outputs     []string       `yaml:"outputs"`
	Audio       *AudioConfig   `yaml:"audio,omitempty"`
	Tags        []string       `yaml:"tags,omitempty"`    // Tags for filtering (e.g., "featured")
	Gallery     *GalleryConfig `yaml:"gallery,omitempty"` // Gallery display configuration
	Prep        []string       `yaml:"prep,omitempty"`    // Shell commands to run before VHS (in workdir)
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
