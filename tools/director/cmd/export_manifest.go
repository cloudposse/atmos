package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/tools/director/internal/scene"
	vhsCache "github.com/cloudposse/atmos/tools/director/internal/vhs"
)

// ManifestScene represents a scene in the exported manifest.
type ManifestScene struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Tags        []string                  `json:"tags,omitempty"`     // Scene tags (e.g., "featured")
	Category    string                    `json:"category,omitempty"` // Gallery category ID
	Title       string                    `json:"title,omitempty"`    // Display title
	Order       int                       `json:"order,omitempty"`    // Sort order within category
	Formats     map[string]ManifestFormat `json:"formats"`
}

// ManifestFormat represents format-specific metadata for a scene output.
type ManifestFormat struct {
	URL  string `json:"url"`
	Type string `json:"type"` // "stream" or "r2"

	// Stream-specific fields (only populated for MP4 videos).
	UID       string   `json:"uid,omitempty"`
	Subdomain string   `json:"subdomain,omitempty"`
	AllUIDs   []string `json:"all_uids,omitempty"` // All UIDs for graceful transitions.

	// Video metadata.
	Duration  float64 `json:"duration,omitempty"`  // Duration in seconds.
	Thumbnail string  `json:"thumbnail,omitempty"` // Thumbnail URL.
}

// Manifest represents the exported gallery manifest.
type Manifest struct {
	Subdomain string          `json:"subdomain,omitempty"` // Common subdomain for all Stream videos.
	Scenes    []ManifestScene `json:"scenes"`
}

// runExportManifest is the shared export logic that can be called from both the export command
// and the render command (when --export-manifest flag is used).
func runExportManifest(demosDir string) error {
	return runExportManifestWithOptions(demosDir, "", false)
}

// runExportManifestWithOptions exports the manifest with configurable options.
func runExportManifestWithOptions(demosDir string, output string, pretty bool) error {
	// Load cache metadata.
	cacheDir := filepath.Join(demosDir, ".cache")
	cache, err := vhsCache.LoadCache(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	// Load scenes.
	scenesFile := filepath.Join(demosDir, "scenes.yaml")
	scenesList, err := scene.LoadScenes(scenesFile)
	if err != nil {
		return fmt.Errorf("failed to load scenes: %w", err)
	}

	// Build manifest.
	manifest := Manifest{
		Scenes: make([]ManifestScene, 0),
	}

	// Determine common subdomain from any scene.
	for _, sceneHash := range cache.Scenes {
		if sceneHash.StreamSubdomain != "" {
			manifest.Subdomain = sceneHash.StreamSubdomain
			break
		}
	}

	for _, sc := range scenesList.Scenes {
		sceneHash, exists := cache.Scenes[sc.Name]
		isPublished := exists && sceneHash.PublicURLs != nil

		// Skip scenes without gallery config that aren't published.
		if !isPublished && sc.Gallery == nil {
			continue
		}

		manifestScene := ManifestScene{
			Name:        sc.Name,
			Description: sc.Description,
			Tags:        sc.Tags,
			Formats:     make(map[string]ManifestFormat),
		}

		// Add gallery metadata if present.
		if sc.Gallery != nil {
			manifestScene.Category = sc.Gallery.Category
			manifestScene.Title = sc.Gallery.Title
			manifestScene.Order = sc.Gallery.Order
		}

		// If scene isn't published yet, add it as a placeholder and continue.
		if !isPublished {
			manifest.Scenes = append(manifest.Scenes, manifestScene)
			continue
		}

		// Add each published format.
		for format, url := range sceneHash.PublicURLs {
			manifestFormat := ManifestFormat{
				URL: url,
			}

			// Determine backend type based on Stream metadata presence.
			if sceneHash.StreamUID != "" && format == "mp4" {
				// This is a Stream video.
				manifestFormat.Type = "stream"
				manifestFormat.UID = sceneHash.GetLatestStreamUID()
				manifestFormat.Subdomain = sceneHash.StreamSubdomain
				manifestFormat.AllUIDs = sceneHash.GetAllStreamUIDs()

				// Include video metadata.
				if sceneHash.Duration > 0 {
					manifestFormat.Duration = sceneHash.Duration
				}

				// Build thumbnail URL from Stream subdomain and UID.
				// Include time parameter if thumbnail time is set.
				if sceneHash.StreamSubdomain != "" && manifestFormat.UID != "" {
					if sceneHash.ThumbnailTime > 0 {
						manifestFormat.Thumbnail = fmt.Sprintf("https://%s/%s/thumbnails/thumbnail.jpg?time=%.1fs",
							sceneHash.StreamSubdomain, manifestFormat.UID, sceneHash.ThumbnailTime)
					} else {
						manifestFormat.Thumbnail = fmt.Sprintf("https://%s/%s/thumbnails/thumbnail.jpg",
							sceneHash.StreamSubdomain, manifestFormat.UID)
					}
				}
			} else {
				// This is an R2 file.
				manifestFormat.Type = "r2"
			}

			manifestScene.Formats[format] = manifestFormat
		}

		manifest.Scenes = append(manifest.Scenes, manifestScene)
	}

	// Marshal to JSON (always pretty-print for readability).
	var jsonData []byte
	if pretty || output != "-" {
		// Pretty-print when writing to file (default) or when --pretty is set.
		jsonData, err = json.MarshalIndent(manifest, "", "  ")
	} else {
		jsonData, err = json.Marshal(manifest)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Determine output path.
	outputPath := output
	if outputPath == "" {
		// Default to website/src/data/manifest.json relative to demos dir.
		// demos dir is at the repo root under "demos/", so we need to go up
		// and into website/src/data/.
		repoRoot := filepath.Dir(demosDir)
		outputPath = filepath.Join(repoRoot, "website", "src", "data", "manifest.json")
	}

	// Write to stdout or file.
	if outputPath == "-" {
		fmt.Println(string(jsonData))
	} else {
		if err := os.WriteFile(outputPath, append(jsonData, '\n'), 0o644); err != nil {
			return fmt.Errorf("failed to write manifest to %s: %w", outputPath, err)
		}
		fmt.Printf("Manifest written to %s\n", outputPath)
	}

	return nil
}

func exportManifestCmd() *cobra.Command {
	var (
		pretty bool
		output string
	)

	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Export gallery manifest JSON",
		Long: `Generate manifest.json with Stream and R2 metadata for gallery components.

Reads cache metadata and scenes configuration to build a JSON manifest
that includes public URLs, video UIDs, and backend information for each
rendered demo scene.

By default, writes to website/src/data/manifest.json. Use --output to
specify a different path, or --output=- to write to stdout.`,
		Example: `
# Export manifest to default location (website/src/data/manifest.json)
director export manifest

# Export manifest to stdout
director export manifest --output=-

# Export manifest to custom file
director export manifest --output=custom.json

# Export with pretty-printing
director export manifest --pretty
`,
		RunE: func(c *cobra.Command, args []string) error {
			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			return runExportManifestWithOptions(demosDir, output, pretty)
		},
	}

	cmd.Flags().BoolVar(&pretty, "pretty", false, "Pretty-print JSON output")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: website/src/data/manifest.json, use - for stdout)")

	return cmd
}
