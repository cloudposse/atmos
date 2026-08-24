package cmd

import "strings"

type helpTopic string

const (
	helpTopicDefault helpTopic = ""
	helpTopicUsage   helpTopic = "usage"
	helpTopicFlags   helpTopic = "flags"
	helpTopicAll     helpTopic = "all"
)

var supportedHelpTopics = []helpTopic{
	helpTopicUsage,
	helpTopicFlags,
	helpTopicAll,
}

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
	topic := helpTopic(value)
	request := helpTopicRequest{
		topic:    topic,
		raw:      value,
		explicit: true,
		valid:    true,
	}

	if isSupportedHelpTopic(topic) {
		return request
	}

	request.valid = false
	return request
}

func isSupportedHelpTopic(topic helpTopic) bool {
	for _, supported := range supportedHelpTopics {
		if topic == supported {
			return true
		}
	}
	return false
}

func validHelpTopics() string {
	topics := make([]string, 0, len(supportedHelpTopics))
	for _, topic := range supportedHelpTopics {
		topics = append(topics, string(topic))
	}
	return strings.Join(topics, ", ")
}
