package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ErrStreamUploadFailed is returned when uploading to Stream fails.
var ErrStreamUploadFailed = fmt.Errorf("failed to upload to Stream")

// StreamBackend implements the Backend interface for Cloudflare Stream.
type StreamBackend struct {
	config       *StreamConfig
	client       *http.Client
	lastMetadata *StreamMetadata // Metadata from the last upload
}

// StreamMetadata holds the metadata returned from Stream API after upload.
type StreamMetadata struct {
	UID               string
	Preview           string
	CustomerSubdomain string
	PlaybackHLS       string
	PlaybackDASH      string
}

// streamAPIResponse represents the response from Cloudflare Stream API.
type streamAPIResponse struct {
	Success bool `json:"success"`
	Result  struct {
		UID      string `json:"uid"`
		Preview  string `json:"preview"`
		Playback struct {
			HLS  string `json:"hls"`
			DASH string `json:"dash"`
		} `json:"playback"`
	} `json:"result"`
	Errors []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// NewStreamBackend creates a new Stream backend with the given configuration.
func NewStreamBackend(config *StreamConfig) (*StreamBackend, error) {
	return &StreamBackend{
		config: config,
		client: &http.Client{},
	}, nil
}

// Validate checks that the backend credentials and configuration are valid.
func (s *StreamBackend) Validate(ctx context.Context) error {
	// For Stream, we can validate by making a lightweight API call.
	// We'll use the list videos endpoint with a limit of 1.
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/stream?per_page=1", s.config.AccountID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("%w: failed to create request: %v", ErrStreamValidation, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.config.APIToken))

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v\n\nVerify your credentials are correct", ErrStreamValidation, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: API returned status %d: %s\n\nVerify your API token has Stream:Edit permissions",
			ErrStreamValidation, resp.StatusCode, string(body))
	}

	return nil
}

// Upload uploads a video file to Cloudflare Stream.
// The title parameter sets a human-readable display name in the Stream UI.
func (s *StreamBackend) Upload(ctx context.Context, localPath, remotePath, title string) error {
	// Read the file.
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("%w: failed to open file %s: %v", ErrStreamUploadFailed, localPath, err)
	}
	defer file.Close()

	// Create multipart form data.
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the file field.
	part, err := writer.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return fmt.Errorf("%w: failed to create form file: %v", ErrStreamUploadFailed, err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("%w: failed to copy file data: %v", ErrStreamUploadFailed, err)
	}

	// Add metadata fields for video identification.
	// - name: hierarchical path for organizing videos in Stream UI
	// - title: human-readable display name shown in Stream dashboard
	meta := map[string]string{
		"name": remotePath,
	}
	if title != "" {
		meta["title"] = title
	}
	metaJSON, _ := json.Marshal(meta)
	if err := writer.WriteField("meta", string(metaJSON)); err != nil {
		return fmt.Errorf("%w: failed to write metadata: %v", ErrStreamUploadFailed, err)
	}

	writer.Close()

	// Build the upload URL.
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/stream", s.config.AccountID)

	// Create the request.
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return fmt.Errorf("%w: failed to create request: %v", ErrStreamUploadFailed, err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.config.APIToken))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send the request.
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: failed to upload to Stream: %v", ErrStreamUploadFailed, err)
	}
	defer resp.Body.Close()

	// Parse the response.
	var apiResp streamAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("%w: failed to parse response: %v", ErrStreamUploadFailed, err)
	}

	if !apiResp.Success {
		errMsg := "unknown error"
		if len(apiResp.Errors) > 0 {
			errMsg = apiResp.Errors[0].Message
		}
		return fmt.Errorf("%w: Stream API error: %s", ErrStreamUploadFailed, errMsg)
	}

	// Extract customer subdomain from preview URL if not already configured.
	customerSubdomain := s.config.CustomerSubdomain
	if customerSubdomain == "" && apiResp.Result.Preview != "" {
		customerSubdomain = extractSubdomain(apiResp.Result.Preview)
		s.config.CustomerSubdomain = customerSubdomain
	}

	// Store metadata from this upload.
	s.lastMetadata = &StreamMetadata{
		UID:               apiResp.Result.UID,
		Preview:           apiResp.Result.Preview,
		CustomerSubdomain: customerSubdomain,
		PlaybackHLS:       apiResp.Result.Playback.HLS,
		PlaybackDASH:      apiResp.Result.Playback.DASH,
	}

	return nil
}

// GetPublicURL returns the public URL for a video at remotePath.
// For Stream, remotePath should be the video UID.
func (s *StreamBackend) GetPublicURL(remotePath string) string {
	// For Stream, we return the preview/watch URL.
	if s.config.CustomerSubdomain != "" {
		return fmt.Sprintf("https://%s/%s/watch", s.config.CustomerSubdomain, remotePath)
	}

	// Fallback if subdomain not yet known (shouldn't happen after first upload).
	return fmt.Sprintf("https://customer-unknown.cloudflarestream.com/%s/watch", remotePath)
}

// SupportsFormat checks if Stream backend supports a given file format.
func (s *StreamBackend) SupportsFormat(ext string) bool {
	// Stream supports video formats only.
	videoFormats := map[string]bool{
		"mp4":  true,
		"webm": true,
		"avi":  true,
		"mkv":  true,
		"mov":  true,
		"flv":  true,
		"mpg":  true,
		"mpeg": true,
		"3gp":  true,
		"m4v":  true,
	}

	// Normalize extension (remove leading dot if present).
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))

	return videoFormats[ext]
}

// GetLastMetadata returns the metadata from the last upload.
// Returns nil if no uploads have been performed yet.
func (s *StreamBackend) GetLastMetadata() *StreamMetadata {
	return s.lastMetadata
}

// SetThumbnailTime sets the thumbnail timestamp for a video.
// The thumbnailTimestampPct is a value between 0 and 1 representing the position in the video.
func (s *StreamBackend) SetThumbnailTime(ctx context.Context, videoUID string, thumbnailTimestampPct float64) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/stream/%s",
		s.config.AccountID, videoUID)

	// Build the request body.
	body := map[string]interface{}{
		"thumbnailTimestampPct": thumbnailTimestampPct,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.config.APIToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to set thumbnail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to set thumbnail (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// extractSubdomain extracts the customer subdomain from a Stream preview URL.
// Example: https://customer-xxx.cloudflarestream.com/uid/watch -> customer-xxx.cloudflarestream.com
func extractSubdomain(previewURL string) string {
	// Remove https:// prefix.
	url := strings.TrimPrefix(previewURL, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Split by / and take first part (the hostname).
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[0]
	}

	return ""
}
