package vhs

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// CacheMetadata stores rendering metadata for scenes.
type CacheMetadata struct {
	Version string               `json:"version"`
	Scenes  map[string]SceneHash `json:"scenes"`
}

// StreamVersion tracks a single version of a video on Cloudflare Stream.
type StreamVersion struct {
	UID         string    `json:"uid"`
	PublishedAt time.Time `json:"published_at"`
	PublishHash string    `json:"publish_hash"` // SHA256 of uploaded file.
	Latest      bool      `json:"latest"`
}

// SceneHash stores the SHA256 hash and render time for a scene.
type SceneHash struct {
	TapeHash   string    `json:"tape_hash"`
	AudioHash  string    `json:"audio_hash,omitempty"`
	RenderedAt time.Time `json:"rendered_at"`
	Outputs    []string  `json:"outputs"`

	// Publish tracking fields.
	PublishedAt *time.Time        `json:"published_at,omitempty"`
	PublishHash string            `json:"publish_hash,omitempty"` // SHA256 of uploaded file.
	PublicURLs  map[string]string `json:"public_urls,omitempty"`  // format -> URL.

	// Video metadata (for MP4 files).
	Duration      float64 `json:"duration,omitempty"`       // Duration in seconds.
	ThumbnailTime float64 `json:"thumbnail_time,omitempty"` // Thumbnail timestamp in seconds.

	// Stream-specific metadata for gallery components.
	StreamUID       string `json:"stream_uid,omitempty"`       // Video UID from Cloudflare Stream (latest).
	StreamSubdomain string `json:"stream_subdomain,omitempty"` // customer-xxx.cloudflarestream.com.

	// Stream version history - tracks all UIDs for graceful transitions.
	StreamVersions []StreamVersion `json:"stream_versions,omitempty"`
}

// LoadCache loads the cache metadata from disk.
func LoadCache(cacheDir string) (*CacheMetadata, error) {
	metadataFile := filepath.Join(cacheDir, "metadata.json")

	data, err := os.ReadFile(metadataFile)
	if os.IsNotExist(err) {
		// Cache doesn't exist yet - return empty cache.
		return &CacheMetadata{
			Version: "1.0",
			Scenes:  make(map[string]SceneHash),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read cache metadata: %w", err)
	}

	var cache CacheMetadata
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse cache metadata: %w", err)
	}

	if cache.Scenes == nil {
		cache.Scenes = make(map[string]SceneHash)
	}

	return &cache, nil
}

// SaveCache saves the cache metadata to disk.
func (c *CacheMetadata) SaveCache(cacheDir string) error {
	metadataFile := filepath.Join(cacheDir, "metadata.json")

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache metadata: %w", err)
	}

	if err := os.WriteFile(metadataFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write cache metadata: %w", err)
	}

	return nil
}

// CalculateSHA256 calculates the SHA256 hash of a file.
func CalculateSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// NeedsRender checks if a scene needs to be rendered.
// Returns true if the tape file has changed, audio file has changed, or outputs are missing.
func (c *CacheMetadata) NeedsRender(sceneName, tapeFile, audioFile string, outputs []string, force bool) (bool, error) {
	// Force flag always triggers render.
	if force {
		return true, nil
	}

	// Calculate current tape file hash.
	currentHash, err := CalculateSHA256(tapeFile)
	if err != nil {
		return false, fmt.Errorf("failed to calculate tape hash: %w", err)
	}

	// Calculate audio file hash if provided.
	var currentAudioHash string
	if audioFile != "" {
		currentAudioHash, err = CalculateSHA256(audioFile)
		if err != nil {
			return false, fmt.Errorf("failed to calculate audio hash: %w", err)
		}
	}

	// Check if scene exists in cache.
	sceneHash, exists := c.Scenes[sceneName]
	if !exists {
		// Never rendered before.
		return true, nil
	}

	// Check if tape hash changed.
	if sceneHash.TapeHash != currentHash {
		return true, nil
	}

	// Check if audio hash changed.
	if currentAudioHash != sceneHash.AudioHash {
		return true, nil
	}

	// Check if all outputs exist.
	for _, output := range outputs {
		if _, err := os.Stat(output); os.IsNotExist(err) {
			return true, nil
		}
	}

	// Cache hit - no render needed.
	return false, nil
}

// UpdateScene updates the cache for a rendered scene.
func (c *CacheMetadata) UpdateScene(sceneName, tapeFile, audioFile string, outputs []string) error {
	hash, err := CalculateSHA256(tapeFile)
	if err != nil {
		return fmt.Errorf("failed to calculate tape hash: %w", err)
	}

	var audioHash string
	if audioFile != "" {
		audioHash, err = CalculateSHA256(audioFile)
		if err != nil {
			return fmt.Errorf("failed to calculate audio hash: %w", err)
		}
	}

	c.Scenes[sceneName] = SceneHash{
		TapeHash:   hash,
		AudioHash:  audioHash,
		RenderedAt: time.Now(),
		Outputs:    outputs,
	}

	return nil
}

// NeedsPublish checks if a scene output file needs to be published.
// Returns true if the file has changed since last publish or has never been published.
func (c *CacheMetadata) NeedsPublish(sceneName, outputFile string, force bool) (bool, error) {
	// Force flag always triggers publish.
	if force {
		return true, nil
	}

	// Check if scene exists in cache.
	sceneHash, exists := c.Scenes[sceneName]
	if !exists {
		// Never published before.
		return true, nil
	}

	// If never published, need to publish.
	if sceneHash.PublishedAt == nil || sceneHash.PublishHash == "" {
		return true, nil
	}

	// Calculate current file hash.
	currentHash, err := CalculateSHA256(outputFile)
	if err != nil {
		return false, fmt.Errorf("failed to calculate file hash: %w", err)
	}

	// Check if file hash changed.
	if sceneHash.PublishHash != currentHash {
		return true, nil
	}

	// Cache hit - no publish needed.
	return false, nil
}

// StreamMetadata contains metadata from Cloudflare Stream upload response.
type StreamMetadata struct {
	UID               string
	CustomerSubdomain string
	Duration          float64 // Video duration in seconds.
}

// UpdatePublish updates the cache for a published scene.
func (c *CacheMetadata) UpdatePublish(sceneName, outputFile, publicURL string, streamMetadata *StreamMetadata) error {
	// Get existing scene hash or create new one.
	sceneHash, exists := c.Scenes[sceneName]
	if !exists {
		sceneHash = SceneHash{
			PublicURLs: make(map[string]string),
		}
	}

	// Ensure PublicURLs map is initialized.
	if sceneHash.PublicURLs == nil {
		sceneHash.PublicURLs = make(map[string]string)
	}

	// Calculate file hash.
	hash, err := CalculateSHA256(outputFile)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}

	// Update publish metadata.
	now := time.Now()
	sceneHash.PublishedAt = &now
	sceneHash.PublishHash = hash

	// Store Stream metadata if provided (for MP4 uploads).
	if streamMetadata != nil {
		// Update the current/latest UID fields.
		sceneHash.StreamUID = streamMetadata.UID
		sceneHash.StreamSubdomain = streamMetadata.CustomerSubdomain

		// Store video duration.
		if streamMetadata.Duration > 0 {
			sceneHash.Duration = streamMetadata.Duration
		}

		// Add to version history.
		sceneHash.addStreamVersion(streamMetadata.UID, now, hash)
	}

	// Store public URL by file extension (format).
	ext := filepath.Ext(outputFile)
	if ext != "" {
		ext = ext[1:] // Remove leading dot
		sceneHash.PublicURLs[ext] = publicURL
	}

	c.Scenes[sceneName] = sceneHash

	return nil
}

// addStreamVersion adds a new Stream version to the history, marking it as latest.
func (s *SceneHash) addStreamVersion(uid string, publishedAt time.Time, publishHash string) {
	// Check if this UID already exists.
	for i := range s.StreamVersions {
		if s.StreamVersions[i].UID == uid {
			// UID exists - update it and mark as latest.
			s.StreamVersions[i].PublishedAt = publishedAt
			s.StreamVersions[i].PublishHash = publishHash
			s.StreamVersions[i].Latest = true
			// Mark others as not latest.
			for j := range s.StreamVersions {
				if j != i {
					s.StreamVersions[j].Latest = false
				}
			}
			return
		}
	}

	// New UID - mark all existing as not latest.
	for i := range s.StreamVersions {
		s.StreamVersions[i].Latest = false
	}

	// Add new version as latest.
	s.StreamVersions = append(s.StreamVersions, StreamVersion{
		UID:         uid,
		PublishedAt: publishedAt,
		PublishHash: publishHash,
		Latest:      true,
	})
}

// MigrateStreamUIDs migrates existing stream_uid fields to stream_versions array.
// This preserves backward compatibility while enabling UID history tracking.
func (c *CacheMetadata) MigrateStreamUIDs() int {
	migrated := 0
	for name, scene := range c.Scenes {
		// Skip if no StreamUID or already has versions.
		if scene.StreamUID == "" || len(scene.StreamVersions) > 0 {
			continue
		}

		// Migrate existing UID to versions array.
		publishedAt := time.Now()
		if scene.PublishedAt != nil {
			publishedAt = *scene.PublishedAt
		}

		scene.StreamVersions = []StreamVersion{
			{
				UID:         scene.StreamUID,
				PublishedAt: publishedAt,
				PublishHash: scene.PublishHash,
				Latest:      true,
			},
		}
		c.Scenes[name] = scene
		migrated++
	}
	return migrated
}

// GetLatestStreamUID returns the latest Stream UID for a scene, or empty string if none.
func (s *SceneHash) GetLatestStreamUID() string {
	for _, v := range s.StreamVersions {
		if v.Latest {
			return v.UID
		}
	}
	// Fallback to legacy field.
	return s.StreamUID
}

// GetAllStreamUIDs returns all Stream UIDs for a scene (latest first).
func (s *SceneHash) GetAllStreamUIDs() []string {
	uids := make([]string, 0, len(s.StreamVersions))
	// Add latest first.
	for _, v := range s.StreamVersions {
		if v.Latest {
			uids = append(uids, v.UID)
			break
		}
	}
	// Add rest.
	for _, v := range s.StreamVersions {
		if !v.Latest {
			uids = append(uids, v.UID)
		}
	}
	return uids
}
