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

	return &ImageInfo{
		ID:          getString(data, "Id"),
		RepoTags:    getStringSlice(data, "RepoTags"),
		RepoDigests: getStringSlice(data, "RepoDigests"),
	}, nil
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
