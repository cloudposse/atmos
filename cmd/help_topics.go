package cmd

import "strings"

type helpTopic string

const (
	helpTopicDefault helpTopic = ""
	helpTopicUsage   helpTopic = "usage"
	helpTopicFlags   helpTopic = "flags"
	helpTopicAll     helpTopic = "all"
)

type helpTopicRequest struct {
	topic    helpTopic
	raw      string
	explicit bool
	valid    bool
}

var currentHelpTopic = helpTopicRequest{valid: true}

func normalizeHelpTopicArgs(args []string) ([]string, helpTopicRequest, bool) {
	request := helpTopicRequest{valid: true}
	normalized := append([]string(nil), args...)
	changed := false

	for i, arg := range normalized {
		if !strings.HasPrefix(arg, "--help=") {
			continue
		}

		value := strings.TrimPrefix(arg, "--help=")
		if isBoolHelpValue(value) {
			continue
		}

		request = parseHelpTopic(value)
		normalized[i] = "--help"
		changed = true
	}

	return normalized, request, changed
}

func isBoolHelpValue(value string) bool {
	switch strings.ToLower(value) {
	case "true", "false", "1", "0", "t", "f":
		return true
	default:
		return false
	}
}

func parseHelpTopic(value string) helpTopicRequest {
	value = strings.ToLower(value)
	request := helpTopicRequest{
		topic:    helpTopic(value),
		raw:      value,
		explicit: true,
		valid:    true,
	}

	switch helpTopic(value) {
	case helpTopicUsage, helpTopicFlags, helpTopicAll:
		return request
	default:
		request.valid = false
		return request
	}
}

func validHelpTopics() string {
	return "usage, flags, all"
}
