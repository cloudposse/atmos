package container

import (
	"encoding/json"
	"regexp"
)

var pushDigestPattern = regexp.MustCompile(`digest:\s*(sha256:[a-fA-F0-9]+)`)

func parsePushDigest(output string) string {
	matches := pushDigestPattern.FindStringSubmatch(output)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func parseImageInspectOutput(output []byte) (*ImageInfo, error) {
	var data map[string]any
	if err := json.Unmarshal(output, &data); err != nil {
		return nil, err
	}

	info := &ImageInfo{
		ID:           getString(data, "Id"),
		RepoTags:     getStringSlice(data, "RepoTags"),
		RepoDigests:  getStringSlice(data, "RepoDigests"),
		Size:         getInt64(data, "Size"),
		Created:      getString(data, "Created"),
		Architecture: getString(data, "Architecture"),
		Os:           getString(data, "Os"),
		Author:       getString(data, "Author"),
	}
	if config, ok := data["Config"].(map[string]any); ok {
		info.Labels = getStringMap(config, "Labels")
	}
	if rootFS, ok := data["RootFS"].(map[string]any); ok {
		info.Layers = len(getStringSlice(rootFS, "Layers"))
	}
	return info, nil
}

// getInt64 extracts a numeric field, tolerating both float64 (encoding/json's
// default) and json.Number representations.
func getInt64(data map[string]any, key string) int64 {
	switch v := data[key].(type) {
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	}
	return 0
}

// getStringMap extracts a map of string values (e.g. image labels).
func getStringMap(data map[string]any, key string) map[string]string {
	raw, ok := data[key].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func getStringSlice(data map[string]any, key string) []string {
	raw, ok := data[key].([]any)
	if !ok {
		return nil
	}

	values := make([]string, 0, len(raw))
	for _, item := range raw {
		if value, ok := item.(string); ok {
			values = append(values, value)
		}
	}
	return values
}
